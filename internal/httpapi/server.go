package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/zeebo/blake3"

	"hexsonic/internal/auth"
	"hexsonic/internal/config"
	"hexsonic/internal/media"
	"hexsonic/internal/security"
	"hexsonic/internal/storage"
)

type Server struct {
	cfg               config.Config
	db                *pgxpool.Pool
	store             *storage.Store
	signer            *security.Signer
	verifier          *auth.Verifier
	debugLogCleanupMu sync.Mutex
	lastDebugLogSweep time.Time
}

var registerMetricsOnce sync.Once

type Track struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Artist       string  `json:"artist"`
	Album        string  `json:"album"`
	Genre        string  `json:"genre"`
	TrackNo      int     `json:"track_number"`
	Rating       float64 `json:"rating"`
	Duration     float64 `json:"duration_seconds"`
	Visibility   string  `json:"visibility"`
	OwnerSub     string  `json:"owner_sub"`
	Uploader     string  `json:"uploader_name"`
	HasLyricsSRT bool    `json:"has_lyrics_srt"`
	HasLyricsTXT bool    `json:"has_lyrics_txt"`
	Created      string  `json:"created_at"`
}

func New(cfg config.Config, db *pgxpool.Pool, store *storage.Store, signer *security.Signer, verifier *auth.Verifier) *Server {
	registerMetricsOnce.Do(func() {
		prometheus.MustRegister(newHexsonicDBCollector(db))
	})
	return &Server{cfg: cfg, db: db, store: store, signer: signer, verifier: verifier}
}

type hexsonicDBCollector struct {
	db *pgxpool.Pool
}

func newHexsonicDBCollector(db *pgxpool.Pool) *hexsonicDBCollector {
	return &hexsonicDBCollector{db: db}
}

func (c *hexsonicDBCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- prometheus.NewDesc("hexsonic_db_tracks_total", "Total tracks in database", nil, nil)
	ch <- prometheus.NewDesc("hexsonic_db_albums_total", "Total albums in database", nil, nil)
	ch <- prometheus.NewDesc("hexsonic_db_playlists_total", "Total playlists in database", nil, nil)
	ch <- prometheus.NewDesc("hexsonic_db_user_profiles_total", "Total user profiles in database", nil, nil)
	ch <- prometheus.NewDesc("hexsonic_play_events_total", "Total recorded play events", nil, nil)
	ch <- prometheus.NewDesc("hexsonic_play_events_guest_total", "Total recorded guest play events", nil, nil)
	ch <- prometheus.NewDesc("hexsonic_play_events_unique_users_total", "Distinct users seen in play events", nil, nil)
	ch <- prometheus.NewDesc("hexsonic_transcode_jobs_total", "Transcode jobs by status", []string{"status"}, nil)
}

func (c *hexsonicDBCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var tracks, albums, playlists, profiles int64
	err := c.db.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*)::bigint FROM tracks),
			(SELECT COUNT(*)::bigint FROM albums),
			(SELECT COUNT(*)::bigint FROM playlists),
			(SELECT COUNT(*)::bigint FROM user_profiles WHERE user_sub NOT LIKE 'deleted:%')
	`).Scan(&tracks, &albums, &playlists, &profiles)
	if err != nil {
		return
	}
	ch <- prometheus.MustNewConstMetric(prometheus.NewDesc("hexsonic_db_tracks_total", "Total tracks in database", nil, nil), prometheus.GaugeValue, float64(tracks))
	ch <- prometheus.MustNewConstMetric(prometheus.NewDesc("hexsonic_db_albums_total", "Total albums in database", nil, nil), prometheus.GaugeValue, float64(albums))
	ch <- prometheus.MustNewConstMetric(prometheus.NewDesc("hexsonic_db_playlists_total", "Total playlists in database", nil, nil), prometheus.GaugeValue, float64(playlists))
	ch <- prometheus.MustNewConstMetric(prometheus.NewDesc("hexsonic_db_user_profiles_total", "Total user profiles in database", nil, nil), prometheus.GaugeValue, float64(profiles))

	var playsTotal, guestTotal, uniqueUsers int64
	err = c.db.QueryRow(ctx, `
		SELECT
			COUNT(*)::bigint,
			COUNT(*) FILTER (WHERE COALESCE(user_sub,'')='')::bigint,
			COUNT(DISTINCT NULLIF(user_sub,''))::bigint
		FROM play_events
	`).Scan(&playsTotal, &guestTotal, &uniqueUsers)
	if err == nil {
		ch <- prometheus.MustNewConstMetric(prometheus.NewDesc("hexsonic_play_events_total", "Total recorded play events", nil, nil), prometheus.GaugeValue, float64(playsTotal))
		ch <- prometheus.MustNewConstMetric(prometheus.NewDesc("hexsonic_play_events_guest_total", "Total recorded guest play events", nil, nil), prometheus.GaugeValue, float64(guestTotal))
		ch <- prometheus.MustNewConstMetric(prometheus.NewDesc("hexsonic_play_events_unique_users_total", "Distinct users seen in play events", nil, nil), prometheus.GaugeValue, float64(uniqueUsers))
	}

	rows, err := c.db.Query(ctx, `SELECT status, COUNT(*)::bigint FROM transcode_jobs GROUP BY status`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int64
		if rows.Scan(&status, &count) != nil {
			continue
		}
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc("hexsonic_transcode_jobs_total", "Transcode jobs by status", []string{"status"}, nil),
			prometheus.GaugeValue,
			float64(count),
			status,
		)
	}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	if s.verifier != nil {
		r.Use(auth.Optional(s.verifier))
	}

	r.Get("/healthz", s.healthz)
	r.Handle("/metrics", promhttp.Handler())
	r.Get("/prometheus", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/prometheus/", http.StatusFound) })
	r.Get("/grafana", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/grafana/", http.StatusFound) })
	r.Handle("/prometheus/*", s.adminProxyHandler("/prometheus", s.cfg.PrometheusProxyURL))
	r.Handle("/grafana/*", s.adminProxyHandler("/grafana", s.cfg.GrafanaProxyURL))
	r.Get("/", s.index)
	r.Get("/register", s.index)
	r.Handle("/rest", http.HandlerFunc(s.subsonicHandler))
	r.Handle("/rest/", http.HandlerFunc(s.subsonicHandler))
	r.Handle("/rest/*", http.HandlerFunc(s.subsonicHandler))

	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/", s.apiIndex)
		api.Post("/auth/login", s.login)
		api.Post("/auth/refresh", s.refresh)
		api.Post("/auth/signup", s.signup)
		api.Get("/discovery", s.discoveryOverview)
		api.Get("/public/settings", s.publicSettings)
		api.Get("/albums", s.listAlbums)
		api.Get("/albums/{albumID}/comments", s.listAlbumComments)
		api.Get("/albums/{albumID}/cover", s.albumCover)
		api.Get("/tracks", s.listTracks)
		api.Get("/tracks/{trackID}", s.getTrackDetail)
		api.Get("/tracks/{trackID}/comments", s.listComments)
		api.Get("/stream/{trackID}", s.streamTrack)
		api.Get("/users/{userSub}/avatar", s.userAvatar)
		api.Get("/users/{userSub}/profile", s.userPublicProfile)
		api.Get("/users/{userSub}/uploads", s.userUploads)
		api.Get("/users/{userSub}/comments", s.listUserProfileComments)
		api.Post("/streams/sign", s.signStream)
		api.Get("/playlists", s.listPlaylists)
		api.Get("/playlists/{playlistID}", s.getPlaylist)
		api.Get("/playlists/{playlistID}/tracks", s.listPlaylistTracks)
		api.Post("/listening-events", s.recordListeningEvent)

		api.Group(func(priv chi.Router) {
			priv.Use(auth.Required(s.verifier))
			priv.Get("/me", s.me)
			priv.Get("/me/profile", s.meProfileGet)
			priv.Patch("/me/profile", s.meProfileUpdate)
			priv.Post("/me/avatar", s.meAvatarUpload)
			priv.Post("/me/password", s.mePasswordUpdate)
			priv.Post("/me/subsonic-password", s.meSubsonicPasswordUpdate)
			priv.Delete("/me/subsonic-password", s.meSubsonicPasswordDelete)
			priv.Post("/albums/{albumID}/comments", s.createAlbumComment)
			priv.Post("/albums/{albumID}/cover-sign", s.signAlbumCover)
			priv.Post("/tracks/{trackID}/comments", s.createComment)
			priv.Post("/tracks/{trackID}/rating", s.rateTrack)
			priv.Post("/users/{userSub}/comments", s.createUserProfileComment)
			priv.Post("/follow/{userSub}", s.followUser)
			priv.Delete("/follow/{userSub}", s.unfollowUser)
			priv.Post("/playlists", s.createPlaylist)
			priv.Patch("/playlists/{playlistID}", s.updatePlaylist)
			priv.Delete("/playlists/{playlistID}", s.deletePlaylist)
			priv.Post("/playlists/{playlistID}/tracks", s.addPlaylistTrack)
			priv.Delete("/playlists/{playlistID}/tracks/{trackID}", s.removePlaylistTrack)
			priv.Get("/creator/stats", s.creatorStats)
		})

		api.Group(func(uploader chi.Router) {
			uploader.Use(auth.Required(s.verifier))
			uploader.Post("/tracks/import", s.importTrack)
			uploader.Post("/tracks/import-batch", s.importTrackBatch)
		})

		api.Group(func(mgr chi.Router) {
			mgr.Use(auth.Required(s.verifier))
			mgr.Get("/manage/tracks", s.manageListTracks)
			mgr.Get("/manage/tracks/{trackID}", s.manageGetTrack)
			mgr.Patch("/manage/tracks/{trackID}", s.manageUpdateTrack)
			mgr.Delete("/manage/tracks/{trackID}", s.manageDeleteTrack)
			mgr.Post("/manage/tracks/{trackID}/cover", s.manageTrackCoverUpload)
			mgr.Post("/manage/tracks/{trackID}/lyrics", s.manageTrackLyricsUpload)
			mgr.Post("/manage/tracks/{trackID}/lyrics-plain", s.manageTrackLyricsPlainUpload)

			mgr.Get("/manage/albums", s.manageListAlbums)
			mgr.Get("/manage/albums/{albumID}", s.manageGetAlbum)
			mgr.Patch("/manage/albums/{albumID}", s.manageUpdateAlbum)
			mgr.Post("/manage/albums/{albumID}/cover", s.manageAlbumCoverUpload)
		})

		api.Group(func(admin chi.Router) {
			admin.Use(auth.Required(s.verifier))
			admin.Use(auth.RequireRole("admin"))
			admin.Get("/admin/settings", s.adminGetSettings)
			admin.Patch("/admin/settings", s.adminUpdateSettings)
			admin.Post("/admin/invites", s.adminCreateInvite)
			admin.Get("/admin/invites", s.adminListInvites)
			admin.Delete("/admin/invites/{inviteID}", s.adminDeleteInvite)
			admin.Get("/admin/users", s.adminListUsers)
			admin.Get("/admin/roles", s.adminListRoles)
			admin.Patch("/admin/users/{userID}", s.adminUpdateUser)
			admin.Delete("/admin/users/{userID}", s.adminDeleteUser)
			admin.Get("/admin/system/overview", s.adminSystemOverview)
			admin.Get("/admin/logs", s.adminListAuditLogs)
			admin.Get("/admin/debug-logs", s.adminListDebugLogs)
			admin.Post("/admin/proxy-session", s.adminCreateProxySession)
			admin.Get("/admin/transcode-jobs", s.adminListJobs)
			admin.Patch("/admin/tracks/{trackID}/visibility", s.adminSetVisibility)
			admin.Delete("/admin/tracks/{trackID}", s.adminDeleteTrack)
		})
	})
	return r
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "hexsonic"})
}

func (s *Server) apiIndex(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "hexsonic",
		"version": "v1",
		"status":  "ok",
		"docs":    "/docs/API_UPLOAD_INFO.md",
		"health":  "/healthz",
		"auth": map[string]string{
			"login":   "/api/v1/auth/login",
			"refresh": "/api/v1/auth/refresh",
			"signup":  "/api/v1/auth/signup",
		},
		"public_endpoints": []string{
			"/api/v1/albums",
			"/api/v1/tracks",
			"/api/v1/public/settings",
		},
		"upload_endpoints": []string{
			"/api/v1/tracks/import",
			"/api/v1/tracks/import-batch",
		},
	})
}

func normalizedSourceContext(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "unknown"
	}
	if len(v) > 64 {
		v = v[:64]
	}
	var b strings.Builder
	for _, ch := range v {
		switch {
		case ch >= 'a' && ch <= 'z':
			b.WriteRune(ch)
		case ch >= '0' && ch <= '9':
			b.WriteRune(ch)
		case ch == '_' || ch == '-' || ch == '/':
			b.WriteRune(ch)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_-/")
	if out == "" {
		return "unknown"
	}
	return out
}

func listeningEventAllowed(v string) bool {
	switch strings.TrimSpace(v) {
	case "play_start", "play_30s", "play_50_percent", "play_complete", "skip_early", "seek", "rating", "playlist_add":
		return true
	default:
		return false
	}
}

func parseStatsWindow(raw string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "24h", "1d":
		return "24h", "now() - interval '24 hour'"
	case "7d", "":
		return "7d", "now() - interval '7 day'"
	case "30d":
		return "30d", "now() - interval '30 day'"
	case "90d":
		return "90d", "now() - interval '90 day'"
	case "all":
		return "all", ""
	default:
		return "30d", "now() - interval '30 day'"
	}
}

func (s *Server) recordBehaviorEvent(ctx context.Context, userSub, trackID, eventType, sourceContext string, playbackSeconds, durationSeconds float64, sessionID string) error {
	trackID = strings.TrimSpace(trackID)
	if trackID == "" || !listeningEventAllowed(eventType) {
		return nil
	}
	var albumID *int64
	var aid int64
	if err := s.db.QueryRow(ctx, `SELECT COALESCE(album_id,0) FROM tracks WHERE id=$1`, trackID).Scan(&aid); err == nil && aid > 0 {
		albumID = &aid
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO listening_events(track_id, album_id, user_sub, session_id, event_type, source_context, playback_seconds, duration_seconds)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8)
	`, trackID, albumID, strings.TrimSpace(userSub), strings.TrimSpace(sessionID), strings.TrimSpace(eventType), normalizedSourceContext(sourceContext), playbackSeconds, durationSeconds)
	return err
}

func (s *Server) scanTrackCards(rows pgx.Rows) ([]map[string]any, error) {
	defer rows.Close()
	out := make([]map[string]any, 0, 32)
	for rows.Next() {
		var trackID, title, artist, album, genre, visibility, ownerSub, uploader, reason string
		var rating, duration, score float64
		if err := rows.Scan(&trackID, &title, &artist, &album, &genre, &rating, &duration, &visibility, &ownerSub, &uploader, &score, &reason); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id":               trackID,
			"title":            title,
			"artist":           artist,
			"album":            album,
			"genre":            genre,
			"rating":           rating,
			"duration_seconds": duration,
			"visibility":       visibility,
			"owner_sub":        ownerSub,
			"uploader_name":    uploader,
			"score":            score,
			"reason":           reason,
		})
	}
	return out, rows.Err()
}

func (s *Server) scanAlbumCards(rows pgx.Rows) ([]map[string]any, error) {
	defer rows.Close()
	out := make([]map[string]any, 0, 24)
	for rows.Next() {
		var albumID int64
		var title, artist, genre, visibility, ownerSub, uploader, reason string
		var trackCount int64
		var score float64
		if err := rows.Scan(&albumID, &title, &artist, &genre, &visibility, &ownerSub, &uploader, &trackCount, &score, &reason); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id":            albumID,
			"title":         title,
			"artist":        artist,
			"genre":         genre,
			"visibility":    visibility,
			"owner_sub":     ownerSub,
			"uploader_name": uploader,
			"track_count":   trackCount,
			"score":         score,
			"reason":        reason,
		})
	}
	return out, rows.Err()
}

func (s *Server) adminProxyHandler(prefix, target string) http.Handler {
	prefix = strings.TrimSpace(prefix)
	base := strings.TrimSpace(target)
	if prefix == "" || base == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "proxy target not configured", http.StatusServiceUnavailable)
		})
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "proxy target invalid", http.StatusInternalServerError)
		})
	}
	rp := httputil.NewSingleHostReverseProxy(u)
	rp.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.FromContext(r.Context())
		if !ok && s.verifier != nil {
			if c, err := r.Cookie(proxyCookieName(prefix)); err == nil {
				if verified, err := s.verifier.Verify(r.Context(), strings.TrimSpace(c.Value)); err == nil {
					claims = verified
					ok = true
				}
			}
		}
		if !ok && s.verifier != nil {
			if tok := strings.TrimSpace(r.URL.Query().Get("access_token")); tok != "" {
				if verified, err := s.verifier.Verify(r.Context(), tok); err == nil {
					claims = verified
					ok = true
					http.SetCookie(w, &http.Cookie{
						Name:     proxyCookieName(prefix),
						Value:    tok,
						Path:     ensureTrailingSlash(prefix),
						HttpOnly: true,
						Secure:   requestIsSecure(r),
						SameSite: http.SameSiteLaxMode,
						MaxAge:   8 * 60 * 60,
					})
				}
			}
		}
		if !ok {
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		}
		if !auth.HasRole(claims, "admin") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		// Keep access token out of upstream query/logs.
		q := r.URL.Query()
		q.Del("access_token")
		r.URL.RawQuery = q.Encode()
		rp.ServeHTTP(w, r)
	})
}

func requestIsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func ensureTrailingSlash(p string) string {
	if strings.HasSuffix(p, "/") {
		return p
	}
	return p + "/"
}

func proxyCookieName(prefix string) string {
	switch strings.TrimSpace(prefix) {
	case "/grafana":
		return "hex_proxy_grafana"
	case "/prometheus":
		return "hex_proxy_prometheus"
	default:
		return "hex_proxy_admin"
	}
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	// Avoid stale SPA bundles/templates in browser caches during rapid UI iterations.
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	http.ServeFile(w, r, "web/index.html")
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	_ = s.seedUserDisplayName(r.Context(), claims.Subject, claims.Username)
	creatorBadge, _ := s.userCreatorBadge(r.Context(), claims.Subject)
	writeJSON(w, http.StatusOK, map[string]any{
		"subject":       claims.Subject,
		"username":      claims.Username,
		"email":         claims.Email,
		"roles":         claims.Roles,
		"creator_badge": creatorBadge,
	})
}

func (s *Server) meProfileGet(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	displayName, bio, avatarPath, err := s.userProfileBySub(r.Context(), claims.Subject)
	if err != nil {
		http.Error(w, "profile unavailable", http.StatusInternalServerError)
		return
	}
	email := claims.Email
	if e, err := s.keycloakEmailByUsername(r.Context(), claims.Username); err == nil && strings.TrimSpace(e) != "" {
		email = e
	}
	avatarURL := ""
	if strings.TrimSpace(avatarPath) != "" {
		avatarURL = fmt.Sprintf("/api/v1/users/%s/avatar?v=%d", url.PathEscape(claims.Subject), time.Now().Unix())
	}
	creatorBadge, _ := s.userCreatorBadge(r.Context(), claims.Subject)
	hasSubsonicPassword, _ := s.userHasSubsonicPassword(r.Context(), claims.Subject)
	writeJSON(w, http.StatusOK, map[string]any{
		"subject":               claims.Subject,
		"username":              claims.Username,
		"email":                 email,
		"display_name":          displayName,
		"bio":                   bio,
		"avatar_url":            avatarURL,
		"creator_badge":         creatorBadge,
		"has_subsonic_password": hasSubsonicPassword,
	})
}

func (s *Server) meProfileUpdate(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	var req struct {
		DisplayName string `json:"display_name"`
		Bio         string `json:"bio"`
		Email       string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if len(displayName) > 120 {
		http.Error(w, "display_name too long", http.StatusBadRequest)
		return
	}
	bio := strings.TrimSpace(req.Bio)
	if len(bio) > 2000 {
		http.Error(w, "bio too long", http.StatusBadRequest)
		return
	}
	if err := s.upsertUserProfile(r.Context(), claims.Subject, displayName, bio); err != nil {
		http.Error(w, "save profile failed", http.StatusInternalServerError)
		return
	}
	email := strings.TrimSpace(req.Email)
	username, err := s.resolveUsername(claims, r.Context())
	if err != nil {
		http.Error(w, "profile identity resolution failed", http.StatusBadGateway)
		return
	}
	if email != "" && email != claims.Email {
		if err := s.updateKeycloakEmailByUsername(r.Context(), username, email); err != nil {
			http.Error(w, "email update failed", http.StatusBadGateway)
			return
		}
	}
	_ = s.writeAudit(r.Context(), claims.Subject, "user.update_profile", "user", claims.Subject, map[string]any{
		"display_name": displayName,
		"email":        email,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) meAvatarUpload(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	target, err := s.saveUserAvatarFromRequest(r.Context(), r, claims.Subject)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = s.writeAudit(r.Context(), claims.Subject, "user.update_avatar", "user", claims.Subject, nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "updated",
		"avatar_url": fmt.Sprintf("/api/v1/users/%s/avatar?v=%d", url.PathEscape(claims.Subject), time.Now().Unix()),
		"path":       target,
	})
}

func (s *Server) mePasswordUpdate(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		http.Error(w, "current_password and new_password required", http.StatusBadRequest)
		return
	}
	if len(req.NewPassword) < 8 {
		http.Error(w, "new_password must be at least 8 characters", http.StatusBadRequest)
		return
	}
	if req.CurrentPassword == req.NewPassword {
		http.Error(w, "new_password must differ from current_password", http.StatusBadRequest)
		return
	}
	username, err := s.resolveUsername(claims, r.Context())
	if err != nil {
		http.Error(w, "password identity resolution failed", http.StatusBadGateway)
		return
	}
	okCred, err := s.verifyUserCredentials(r.Context(), username, req.CurrentPassword)
	if err != nil {
		http.Error(w, "password validation backend unavailable", http.StatusBadGateway)
		return
	}
	if !okCred {
		http.Error(w, "current password invalid", http.StatusForbidden)
		return
	}
	if err := s.resetKeycloakPasswordByUsername(r.Context(), username, req.NewPassword); err != nil {
		http.Error(w, "password update failed", http.StatusBadGateway)
		return
	}
	_ = s.writeAudit(r.Context(), claims.Subject, "user.update_password", "user", claims.Subject, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) meSubsonicPasswordUpdate(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	pass := strings.TrimSpace(req.Password)
	if len(pass) < 8 {
		http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}
	if len(pass) > 128 {
		http.Error(w, "password too long", http.StatusBadRequest)
		return
	}
	if err := s.setUserSubsonicPassword(r.Context(), claims.Subject, pass); err != nil {
		http.Error(w, "subsonic password update failed", http.StatusInternalServerError)
		return
	}
	_ = s.writeAudit(r.Context(), claims.Subject, "user.update_subsonic_password", "user", claims.Subject, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) meSubsonicPasswordDelete(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	if err := s.clearUserSubsonicPassword(r.Context(), claims.Subject); err != nil {
		http.Error(w, "subsonic password delete failed", http.StatusInternalServerError)
		return
	}
	_ = s.writeAudit(r.Context(), claims.Subject, "user.delete_subsonic_password", "user", claims.Subject, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		http.Error(w, "username and password required", http.StatusBadRequest)
		return
	}

	tokenURL := strings.TrimRight(s.cfg.OIDCIssuerURL, "/") + "/protocol/openid-connect/token"
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", s.cfg.OIDCClientID)
	if scopes := strings.TrimSpace(s.cfg.OIDCScopes); scopes != "" {
		form.Set("scope", scopes)
	}
	if strings.TrimSpace(s.cfg.OIDCClientSecret) != "" {
		form.Set("client_secret", s.cfg.OIDCClientSecret)
	}
	form.Set("username", req.Username)
	form.Set("password", req.Password)

	httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		http.Error(w, "login request failed", http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		http.Error(w, "auth backend unavailable", http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 2*1024*1024))
	if res.StatusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		if len(body) == 0 {
			_, _ = w.Write([]byte(`{"error":"login failed"}`))
			return
		}
		_, _ = w.Write(body)
		return
	}

	// Validate token once, so frontend gets immediate feedback if issuer config is broken.
	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err == nil && s.verifier != nil && tokenResp.AccessToken != "" {
		if _, err := s.verifier.Verify(r.Context(), tokenResp.AccessToken); err != nil {
			http.Error(w, "token verification failed", http.StatusUnauthorized)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, bytes.NewReader(body))
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	req.RefreshToken = strings.TrimSpace(req.RefreshToken)
	if req.RefreshToken == "" {
		http.Error(w, "refresh_token required", http.StatusBadRequest)
		return
	}

	tokenURL := strings.TrimRight(s.cfg.OIDCIssuerURL, "/") + "/protocol/openid-connect/token"
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", s.cfg.OIDCClientID)
	if strings.TrimSpace(s.cfg.OIDCClientSecret) != "" {
		form.Set("client_secret", s.cfg.OIDCClientSecret)
	}
	form.Set("refresh_token", req.RefreshToken)

	httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		http.Error(w, "refresh request failed", http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		http.Error(w, "auth backend unavailable", http.StatusBadGateway)
		return
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 2*1024*1024))
	if res.StatusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		if len(body) == 0 {
			_, _ = w.Write([]byte(`{"error":"refresh failed"}`))
			return
		}
		_, _ = w.Write(body)
		return
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err == nil && s.verifier != nil && tokenResp.AccessToken != "" {
		if _, err := s.verifier.Verify(r.Context(), tokenResp.AccessToken); err != nil {
			http.Error(w, "token verification failed", http.StatusUnauthorized)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, bytes.NewReader(body))
}

func (s *Server) publicSettings(w http.ResponseWriter, r *http.Request) {
	enabled, err := s.isRegistrationEnabled(r.Context())
	if err != nil {
		http.Error(w, "settings unavailable", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"registration_enabled": enabled,
	})
}

func (s *Server) signup(w http.ResponseWriter, r *http.Request) {
	enabled, err := s.isRegistrationEnabled(r.Context())
	if err != nil {
		http.Error(w, "settings unavailable", http.StatusInternalServerError)
		return
	}

	var req struct {
		Username    string `json:"username"`
		Email       string `json:"email"`
		Password    string `json:"password"`
		InviteToken string `json:"invite_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)
	if len(req.Username) < 3 || req.Password == "" {
		http.Error(w, "username and password required", http.StatusBadRequest)
		return
	}
	if len(req.Password) < 8 {
		http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}
	req.InviteToken = strings.TrimSpace(req.InviteToken)

	inviteUsed := false
	if !enabled {
		if req.InviteToken == "" {
			http.Error(w, "registration disabled", http.StatusForbidden)
			return
		}
		ok, err := s.registrationInviteUsable(r.Context(), req.InviteToken)
		if err != nil {
			http.Error(w, "invite validation failed", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "invalid or expired invite", http.StatusForbidden)
			return
		}
		inviteUsed = true
	} else if req.InviteToken != "" {
		ok, err := s.registrationInviteUsable(r.Context(), req.InviteToken)
		if err != nil {
			http.Error(w, "invite validation failed", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "invalid or expired invite", http.StatusForbidden)
			return
		}
		inviteUsed = true
	}

	baseURL, realm, err := keycloakBaseAndRealm(s.cfg.OIDCIssuerURL)
	if err != nil {
		http.Error(w, "oidc issuer configuration invalid", http.StatusInternalServerError)
		return
	}

	adminToken, err := s.keycloakAdminToken(r.Context(), baseURL)
	if err != nil {
		http.Error(w, "registration backend unavailable", http.StatusBadGateway)
		return
	}

	userID, err := s.keycloakCreateUser(r.Context(), baseURL, realm, adminToken, req.Username, req.Email)
	if err != nil {
		if errors.Is(err, errUserAlreadyExists) {
			http.Error(w, "username already exists", http.StatusConflict)
			return
		}
		http.Error(w, "registration failed", http.StatusBadGateway)
		return
	}

	if err := s.keycloakSetPassword(r.Context(), baseURL, realm, adminToken, userID, req.Password); err != nil {
		http.Error(w, "registration failed while setting password", http.StatusBadGateway)
		return
	}

	if err := s.keycloakAssignRealmRole(r.Context(), baseURL, realm, adminToken, userID, "user"); err != nil {
		http.Error(w, "registration failed during role assignment", http.StatusBadGateway)
		return
	}
	if inviteUsed {
		_ = s.consumeRegistrationInvite(r.Context(), req.InviteToken, userID)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"status":   "created",
		"username": req.Username,
		"roles":    []string{"user"},
	})
}

var errUserAlreadyExists = errors.New("user already exists")

func keycloakBaseAndRealm(issuer string) (string, string, error) {
	u, err := url.Parse(strings.TrimSpace(issuer))
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] != "realms" {
			continue
		}
		realm := parts[i+1]
		if realm == "" {
			break
		}
		prefix := strings.Join(parts[:i], "/")
		if prefix != "" {
			return strings.TrimRight(u.Scheme+"://"+u.Host+"/"+prefix, "/"), realm, nil
		}
		return strings.TrimRight(u.Scheme+"://"+u.Host, "/"), realm, nil
	}
	return "", "", fmt.Errorf("issuer url must contain /realms/{realm}")
}

func (s *Server) keycloakAdminToken(ctx context.Context, baseURL string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", "admin-cli")
	form.Set("username", s.cfg.OIDCAdminUser)
	form.Set("password", s.cfg.OIDCAdminPassword)

	tokenURL := strings.TrimRight(baseURL, "/") + "/realms/master/protocol/openid-connect/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("admin token status: %d", res.StatusCode)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2*1024*1024)).Decode(&payload); err != nil {
		return "", err
	}
	if payload.AccessToken == "" {
		return "", fmt.Errorf("missing admin access token")
	}
	return payload.AccessToken, nil
}

func (s *Server) keycloakCreateUser(ctx context.Context, baseURL, realm, adminToken, username, email string) (string, error) {
	payload := map[string]any{
		"username":        username,
		"enabled":         true,
		"emailVerified":   true,
		"firstName":       username,
		"lastName":        "User",
		"requiredActions": []string{},
	}
	if email != "" {
		payload["email"] = email
	} else {
		payload["email"] = username + "@hexsonic.local"
	}
	body, _ := json.Marshal(payload)

	createURL := fmt.Sprintf("%s/admin/realms/%s/users", strings.TrimRight(baseURL, "/"), url.PathEscape(realm))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, createURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusConflict {
		return "", errUserAlreadyExists
	}
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusNoContent {
		return "", fmt.Errorf("create user status: %d", res.StatusCode)
	}

	return s.keycloakFindUserID(ctx, baseURL, realm, adminToken, username)
}

func (s *Server) keycloakSetPassword(ctx context.Context, baseURL, realm, adminToken, userID, password string) error {
	payload := map[string]any{
		"type":      "password",
		"value":     password,
		"temporary": false,
	}
	body, _ := json.Marshal(payload)
	setPasswordURL := fmt.Sprintf("%s/admin/realms/%s/users/%s/reset-password",
		strings.TrimRight(baseURL, "/"), url.PathEscape(realm), url.PathEscape(userID))

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, setPasswordURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("set password status: %d", res.StatusCode)
	}
	return nil
}

func (s *Server) keycloakFindUserID(ctx context.Context, baseURL, realm, adminToken, username string) (string, error) {
	findURL := fmt.Sprintf("%s/admin/realms/%s/users?username=%s&exact=true",
		strings.TrimRight(baseURL, "/"), url.PathEscape(realm), url.QueryEscape(username))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, findURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lookup user status: %d", res.StatusCode)
	}
	var users []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2*1024*1024)).Decode(&users); err != nil {
		return "", err
	}
	if len(users) == 0 || users[0].ID == "" {
		return "", fmt.Errorf("created user not found")
	}
	return users[0].ID, nil
}

func (s *Server) keycloakAssignRealmRole(ctx context.Context, baseURL, realm, adminToken, userID, roleName string) error {
	role, err := s.keycloakRealmRole(ctx, baseURL, realm, adminToken, roleName)
	if err != nil {
		return err
	}
	assignBody, _ := json.Marshal([]map[string]any{role})
	assignURL := fmt.Sprintf("%s/admin/realms/%s/users/%s/role-mappings/realm",
		strings.TrimRight(baseURL, "/"), url.PathEscape(realm), url.PathEscape(userID))
	assignReq, err := http.NewRequestWithContext(ctx, http.MethodPost, assignURL, bytes.NewReader(assignBody))
	if err != nil {
		return err
	}
	assignReq.Header.Set("Authorization", "Bearer "+adminToken)
	assignReq.Header.Set("Content-Type", "application/json")

	assignRes, err := http.DefaultClient.Do(assignReq)
	if err != nil {
		return err
	}
	defer assignRes.Body.Close()
	if assignRes.StatusCode != http.StatusNoContent {
		return fmt.Errorf("assign role status: %d", assignRes.StatusCode)
	}
	return nil
}

func (s *Server) keycloakRemoveRealmRole(ctx context.Context, baseURL, realm, adminToken, userID, roleName string) error {
	role, err := s.keycloakRealmRole(ctx, baseURL, realm, adminToken, roleName)
	if err != nil {
		return err
	}
	body, _ := json.Marshal([]map[string]any{role})
	removeURL := fmt.Sprintf("%s/admin/realms/%s/users/%s/role-mappings/realm",
		strings.TrimRight(baseURL, "/"), url.PathEscape(realm), url.PathEscape(userID))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, removeURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("remove role status: %d", res.StatusCode)
	}
	return nil
}

func (s *Server) keycloakRealmRole(ctx context.Context, baseURL, realm, adminToken, roleName string) (map[string]any, error) {
	roleURL := fmt.Sprintf("%s/admin/realms/%s/roles/%s",
		strings.TrimRight(baseURL, "/"), url.PathEscape(realm), url.PathEscape(roleName))
	load := func() (map[string]any, int, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, roleURL, nil)
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("Authorization", "Bearer "+adminToken)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, 0, err
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, res.StatusCode, nil
		}
		var role map[string]any
		if err := json.NewDecoder(io.LimitReader(res.Body, 2*1024*1024)).Decode(&role); err != nil {
			return nil, 0, err
		}
		return role, http.StatusOK, nil
	}

	role, status, err := load()
	if err != nil {
		return nil, err
	}
	if status == http.StatusOK {
		return role, nil
	}
	if status != http.StatusNotFound {
		return nil, fmt.Errorf("load role status: %d", status)
	}
	if err := s.keycloakCreateRealmRole(ctx, baseURL, realm, adminToken, roleName); err != nil {
		return nil, err
	}
	role, status, err = load()
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("load role after create status: %d", status)
	}
	return role, nil
}

func (s *Server) keycloakCreateRealmRole(ctx context.Context, baseURL, realm, adminToken, roleName string) error {
	createURL := fmt.Sprintf("%s/admin/realms/%s/roles",
		strings.TrimRight(baseURL, "/"), url.PathEscape(realm))
	body, _ := json.Marshal(map[string]any{
		"name": strings.TrimSpace(roleName),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, createURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusConflict {
		return fmt.Errorf("create role status: %d", res.StatusCode)
	}
	return nil
}

func (s *Server) keycloakListUsers(ctx context.Context, baseURL, realm, adminToken, search string, max int) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("first", "0")
	if max <= 0 {
		max = 200
	}
	q.Set("max", strconv.Itoa(max))
	if search != "" {
		q.Set("search", search)
	}
	listURL := fmt.Sprintf("%s/admin/realms/%s/users?%s",
		strings.TrimRight(baseURL, "/"), url.PathEscape(realm), q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list users status: %d", res.StatusCode)
	}
	var users []struct {
		ID        string `json:"id"`
		Username  string `json:"username"`
		Email     string `json:"email"`
		Enabled   bool   `json:"enabled"`
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 8*1024*1024)).Decode(&users); err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		roles, _ := s.keycloakUserRealmRoles(ctx, baseURL, realm, adminToken, u.ID)
		out = append(out, map[string]any{
			"id":        u.ID,
			"username":  u.Username,
			"email":     u.Email,
			"enabled":   u.Enabled,
			"firstName": u.FirstName,
			"lastName":  u.LastName,
			"roles":     roles,
		})
	}
	return out, nil
}

func (s *Server) keycloakListRealmRoles(ctx context.Context, baseURL, realm, adminToken string) ([]string, error) {
	u := fmt.Sprintf("%s/admin/realms/%s/roles",
		strings.TrimRight(baseURL, "/"), url.PathEscape(realm))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list roles status: %d", res.StatusCode)
	}
	var raw []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 4*1024*1024)).Decode(&raw); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(raw))
	for _, rr := range raw {
		name := strings.TrimSpace(rr.Name)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out, nil
}

func (s *Server) keycloakUserRealmRoles(ctx context.Context, baseURL, realm, adminToken, userID string) ([]string, error) {
	u := fmt.Sprintf("%s/admin/realms/%s/users/%s/role-mappings/realm",
		strings.TrimRight(baseURL, "/"), url.PathEscape(realm), url.PathEscape(userID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("roles status: %d", res.StatusCode)
	}
	var raw []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2*1024*1024)).Decode(&raw); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if strings.TrimSpace(r.Name) != "" {
			out = append(out, r.Name)
		}
	}
	return out, nil
}

func (s *Server) keycloakSetUserEnabled(ctx context.Context, baseURL, realm, adminToken, userID string, enabled bool) error {
	getURL := fmt.Sprintf("%s/admin/realms/%s/users/%s",
		strings.TrimRight(baseURL, "/"), url.PathEscape(realm), url.PathEscape(userID))
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, getURL, nil)
	if err != nil {
		return err
	}
	getReq.Header.Set("Authorization", "Bearer "+adminToken)
	getRes, err := http.DefaultClient.Do(getReq)
	if err != nil {
		return err
	}
	defer getRes.Body.Close()
	if getRes.StatusCode != http.StatusOK {
		return fmt.Errorf("load user status: %d", getRes.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(getRes.Body, 2*1024*1024)).Decode(&payload); err != nil {
		return err
	}
	payload["enabled"] = enabled
	body, _ := json.Marshal(payload)
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, getURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	putReq.Header.Set("Authorization", "Bearer "+adminToken)
	putReq.Header.Set("Content-Type", "application/json")
	putRes, err := http.DefaultClient.Do(putReq)
	if err != nil {
		return err
	}
	defer putRes.Body.Close()
	if putRes.StatusCode != http.StatusNoContent {
		return fmt.Errorf("set user enabled status: %d", putRes.StatusCode)
	}
	return nil
}

func (s *Server) keycloakDeleteUser(ctx context.Context, baseURL, realm, adminToken, userID string) error {
	u := fmt.Sprintf("%s/admin/realms/%s/users/%s",
		strings.TrimRight(baseURL, "/"), url.PathEscape(realm), url.PathEscape(userID))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete user status: %d", res.StatusCode)
	}
	return nil
}

func (s *Server) keycloakCountUsers(ctx context.Context, baseURL, realm, adminToken string) (int64, error) {
	u := fmt.Sprintf("%s/admin/realms/%s/users/count",
		strings.TrimRight(baseURL, "/"), url.PathEscape(realm))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("count users status: %d", res.StatusCode)
	}
	var count int64
	if err := json.NewDecoder(io.LimitReader(res.Body, 512)).Decode(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Server) listTracks(w http.ResponseWriter, r *http.Request) {
	claims, hasClaims := auth.FromContext(r.Context())
	isAdmin := hasClaims && auth.HasRole(claims, "admin")

	var rows pgx.Rows
	var err error
	if isAdmin {
		rows, err = s.db.Query(context.Background(), `
			SELECT t.id::text, t.title, COALESCE(ar.name,''), COALESCE(al.title,''), COALESCE(t.genre,''), COALESCE(t.track_number,0), COALESCE(rr.avg_rating, 0), t.duration_seconds, t.visibility, t.owner_sub, COALESCE(NULLIF(up.display_name,''), t.owner_sub), (COALESCE(t.lyrics_srt,'') <> ''), (COALESCE(t.lyrics_txt,'') <> ''), t.created_at::text
			FROM tracks t
			LEFT JOIN artists ar ON ar.id = t.artist_id
			LEFT JOIN albums al ON al.id = t.album_id
			LEFT JOIN user_profiles up ON up.user_sub=t.owner_sub
			LEFT JOIN LATERAL (
				SELECT AVG(r.rating)::float8 AS avg_rating
				FROM ratings r
				WHERE r.track_id = t.id
			) rr ON true
			ORDER BY COALESCE(t.track_number,0) ASC, t.created_at DESC
			LIMIT 500
		`)
	} else if hasClaims {
		rows, err = s.db.Query(context.Background(), `
			SELECT t.id::text, t.title, COALESCE(ar.name,''), COALESCE(al.title,''), COALESCE(t.genre,''), COALESCE(t.track_number,0), COALESCE(rr.avg_rating, 0), t.duration_seconds, t.visibility, t.owner_sub, COALESCE(NULLIF(up.display_name,''), t.owner_sub), (COALESCE(t.lyrics_srt,'') <> ''), (COALESCE(t.lyrics_txt,'') <> ''), t.created_at::text
			FROM tracks t
			LEFT JOIN artists ar ON ar.id = t.artist_id
			LEFT JOIN albums al ON al.id = t.album_id
			LEFT JOIN user_profiles up ON up.user_sub=t.owner_sub
			LEFT JOIN LATERAL (
				SELECT AVG(r.rating)::float8 AS avg_rating
				FROM ratings r
				WHERE r.track_id = t.id
			) rr ON true
			WHERE
				t.visibility = 'public'
				OR t.owner_sub = $1
				OR (
					t.visibility = 'followers_only'
					AND EXISTS (
						SELECT 1 FROM follows f
						WHERE f.follower_sub = $1 AND f.followed_sub = t.owner_sub
					)
				)
			ORDER BY COALESCE(t.track_number,0) ASC, t.created_at DESC
			LIMIT 500
		`, claims.Subject)
	} else {
		rows, err = s.db.Query(context.Background(), `
			SELECT t.id::text, t.title, COALESCE(ar.name,''), COALESCE(al.title,''), COALESCE(t.genre,''), COALESCE(t.track_number,0), COALESCE(rr.avg_rating, 0), t.duration_seconds, t.visibility, t.owner_sub, COALESCE(NULLIF(up.display_name,''), t.owner_sub), (COALESCE(t.lyrics_srt,'') <> ''), (COALESCE(t.lyrics_txt,'') <> ''), t.created_at::text
			FROM tracks t
			LEFT JOIN artists ar ON ar.id = t.artist_id
			LEFT JOIN albums al ON al.id = t.album_id
			LEFT JOIN user_profiles up ON up.user_sub=t.owner_sub
			LEFT JOIN LATERAL (
				SELECT AVG(r.rating)::float8 AS avg_rating
				FROM ratings r
				WHERE r.track_id = t.id
			) rr ON true
			WHERE t.visibility = 'public'
			ORDER BY COALESCE(t.track_number,0) ASC, t.created_at DESC
			LIMIT 500
		`)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	tracks := make([]Track, 0, 64)
	for rows.Next() {
		var t Track
		if err := rows.Scan(&t.ID, &t.Title, &t.Artist, &t.Album, &t.Genre, &t.TrackNo, &t.Rating, &t.Duration, &t.Visibility, &t.OwnerSub, &t.Uploader, &t.HasLyricsSRT, &t.HasLyricsTXT, &t.Created); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tracks = append(tracks, t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tracks": tracks})
}

func (s *Server) getTrackDetail(w http.ResponseWriter, r *http.Request) {
	trackID := chi.URLParam(r, "trackID")
	claims, hasClaims := auth.FromContext(r.Context())
	viewerSub := ""
	isAdmin := false
	if hasClaims {
		viewerSub = claims.Subject
		isAdmin = auth.HasRole(claims, "admin")
	}
	allowed, err := s.canAccessTrack(r.Context(), viewerSub, isAdmin, trackID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var out struct {
		ID         string  `json:"id"`
		Title      string  `json:"title"`
		Artist     string  `json:"artist"`
		Album      string  `json:"album"`
		Genre      string  `json:"genre"`
		TrackNo    int     `json:"track_number"`
		Rating     float64 `json:"rating"`
		Duration   float64 `json:"duration_seconds"`
		Visibility string  `json:"visibility"`
		OwnerSub   string  `json:"owner_sub"`
		Uploader   string  `json:"uploader_name"`
		LyricsSRT  string  `json:"lyrics_srt"`
		LyricsTXT  string  `json:"lyrics_txt"`
		CreatedAt  string  `json:"created_at"`
	}
	err = s.db.QueryRow(r.Context(), `
		SELECT t.id::text, t.title, COALESCE(ar.name,''), COALESCE(al.title,''), COALESCE(t.genre,''), COALESCE(t.track_number,0), COALESCE(rr.avg_rating,0), t.duration_seconds, t.visibility, t.owner_sub, COALESCE(NULLIF(up.display_name,''), t.owner_sub), COALESCE(t.lyrics_srt,''), COALESCE(t.lyrics_txt,''), t.created_at::text
		FROM tracks t
		LEFT JOIN artists ar ON ar.id=t.artist_id
		LEFT JOIN albums al ON al.id=t.album_id
		LEFT JOIN user_profiles up ON up.user_sub=t.owner_sub
		LEFT JOIN LATERAL (
			SELECT AVG(r.rating)::float8 AS avg_rating
			FROM ratings r
			WHERE r.track_id=t.id
		) rr ON true
		WHERE t.id=$1
	`, trackID).Scan(&out.ID, &out.Title, &out.Artist, &out.Album, &out.Genre, &out.TrackNo, &out.Rating, &out.Duration, &out.Visibility, &out.OwnerSub, &out.Uploader, &out.LyricsSRT, &out.LyricsTXT, &out.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listAlbums(w http.ResponseWriter, r *http.Request) {
	claims, hasClaims := auth.FromContext(r.Context())
	isAdmin := hasClaims && auth.HasRole(claims, "admin")

	var rows pgx.Rows
	var err error
	if isAdmin {
		rows, err = s.db.Query(r.Context(), `
			SELECT a.id, a.title, COALESCE(ar.name,''), a.visibility, a.created_at::text, COALESCE(a.cover_path,''), a.owner_sub, COALESCE(NULLIF(up.display_name,''), a.owner_sub), COALESCE(a.genre,'')
			FROM albums a
			LEFT JOIN artists ar ON ar.id = a.artist_id
			LEFT JOIN user_profiles up ON up.user_sub=a.owner_sub
			ORDER BY a.created_at DESC
			LIMIT 500
		`)
	} else if hasClaims {
		rows, err = s.db.Query(r.Context(), `
			SELECT a.id, a.title, COALESCE(ar.name,''), a.visibility, a.created_at::text, COALESCE(a.cover_path,''), a.owner_sub, COALESCE(NULLIF(up.display_name,''), a.owner_sub), COALESCE(a.genre,'')
			FROM albums a
			LEFT JOIN artists ar ON ar.id = a.artist_id
			LEFT JOIN user_profiles up ON up.user_sub=a.owner_sub
			WHERE a.visibility = 'public' OR a.owner_sub = $1
			ORDER BY a.created_at DESC
			LIMIT 500
		`, claims.Subject)
	} else {
		rows, err = s.db.Query(r.Context(), `
			SELECT a.id, a.title, COALESCE(ar.name,''), a.visibility, a.created_at::text, COALESCE(a.cover_path,''), a.owner_sub, COALESCE(NULLIF(up.display_name,''), a.owner_sub), COALESCE(a.genre,'')
			FROM albums a
			LEFT JOIN artists ar ON ar.id = a.artist_id
			LEFT JOIN user_profiles up ON up.user_sub=a.owner_sub
			WHERE a.visibility = 'public'
			ORDER BY a.created_at DESC
			LIMIT 500
		`)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	albums := make([]map[string]any, 0, 64)
	for rows.Next() {
		var id int64
		var title, artist, visibility, created, coverPath, ownerSub, uploaderName, genre string
		if err := rows.Scan(&id, &title, &artist, &visibility, &created, &coverPath, &ownerSub, &uploaderName, &genre); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		albums = append(albums, map[string]any{
			"id":            id,
			"title":         title,
			"artist":        artist,
			"genre":         genre,
			"visibility":    visibility,
			"created_at":    created,
			"cover_path":    coverPath,
			"owner_sub":     ownerSub,
			"uploader_name": uploaderName,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"albums": albums})
}

func (s *Server) recordListeningEvent(w http.ResponseWriter, r *http.Request) {
	claims, hasClaims := auth.FromContext(r.Context())
	viewerSub := ""
	isAdmin := false
	if hasClaims {
		viewerSub = claims.Subject
		isAdmin = auth.HasRole(claims, "admin")
	}
	var req struct {
		TrackID         string  `json:"track_id"`
		EventType       string  `json:"event_type"`
		SourceContext   string  `json:"source_context"`
		SessionID       string  `json:"session_id"`
		PlaybackSeconds float64 `json:"playback_seconds"`
		DurationSeconds float64 `json:"duration_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	req.TrackID = strings.TrimSpace(req.TrackID)
	req.EventType = strings.TrimSpace(req.EventType)
	if req.TrackID == "" || !listeningEventAllowed(req.EventType) {
		http.Error(w, "invalid event", http.StatusBadRequest)
		return
	}
	allowed, err := s.canAccessTrack(r.Context(), viewerSub, isAdmin, req.TrackID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if req.PlaybackSeconds < 0 {
		req.PlaybackSeconds = 0
	}
	if req.DurationSeconds < 0 {
		req.DurationSeconds = 0
	}
	if err := s.recordBehaviorEvent(r.Context(), viewerSub, req.TrackID, req.EventType, req.SourceContext, req.PlaybackSeconds, req.DurationSeconds, req.SessionID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) discoveryOverview(w http.ResponseWriter, r *http.Request) {
	claims, hasClaims := auth.FromContext(r.Context())
	viewerSub := ""
	if hasClaims {
		viewerSub = claims.Subject
	}

	topTracksRows, err := s.db.Query(r.Context(), `
		SELECT
			t.id::text,
			t.title,
			COALESCE(ar.name,'') AS artist,
			COALESCE(al.title,'') AS album,
			COALESCE(t.genre,'') AS genre,
			COALESCE(rr.avg_rating,0) AS rating,
			t.duration_seconds,
			t.visibility,
			t.owner_sub,
			COALESCE(NULLIF(up.display_name,''), t.owner_sub) AS uploader_name,
			(COALESCE(pe.plays,0) + COALESCE(le.event_score,0))::float8 AS score,
			'Top songs right now' AS reason
		FROM tracks t
		LEFT JOIN artists ar ON ar.id=t.artist_id
		LEFT JOIN albums al ON al.id=t.album_id
		LEFT JOIN user_profiles up ON up.user_sub=t.owner_sub
		LEFT JOIN LATERAL (
			SELECT AVG(r.rating)::float8 AS avg_rating
			FROM ratings r
			WHERE r.track_id=t.id
		) rr ON true
		LEFT JOIN LATERAL (
			SELECT COUNT(*)::bigint AS plays
			FROM play_events p
			WHERE p.track_id=t.id
		) pe ON true
		LEFT JOIN LATERAL (
			SELECT COALESCE(SUM(
				CASE event_type
					WHEN 'play_30s' THEN 2
					WHEN 'play_50_percent' THEN 3
					WHEN 'play_complete' THEN 5
					WHEN 'playlist_add' THEN 4
					WHEN 'rating' THEN 3
					WHEN 'skip_early' THEN -2
					ELSE 0
				END
			),0)::bigint AS event_score
			FROM listening_events le
			WHERE le.track_id=t.id
			  AND le.created_at >= now() - interval '30 day'
		) le ON true
		WHERE t.visibility='public'
		ORDER BY score DESC, pe.plays DESC, lower(t.title)
		LIMIT 12
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	topTracks, err := s.scanTrackCards(topTracksRows)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	trendingRows, err := s.db.Query(r.Context(), `
		SELECT
			t.id::text,
			t.title,
			COALESCE(ar.name,'') AS artist,
			COALESCE(al.title,'') AS album,
			COALESCE(t.genre,'') AS genre,
			COALESCE(rr.avg_rating,0) AS rating,
			t.duration_seconds,
			t.visibility,
			t.owner_sub,
			COALESCE(NULLIF(up.display_name,''), t.owner_sub) AS uploader_name,
			(
				COALESCE(ps.plays_7d,0) * 1.5 +
				COALESCE(ls.complete_7d,0) * 4 +
				COALESCE(ls.mid_7d,0) * 2 +
				COALESCE(ls.playlist_7d,0) * 4 +
				COALESCE(ls.rating_7d,0) * 3 -
				COALESCE(ls.skip_7d,0) * 2
			)::float8 AS score,
			'Trending this week' AS reason
		FROM tracks t
		LEFT JOIN artists ar ON ar.id=t.artist_id
		LEFT JOIN albums al ON al.id=t.album_id
		LEFT JOIN user_profiles up ON up.user_sub=t.owner_sub
		LEFT JOIN LATERAL (
			SELECT AVG(r.rating)::float8 AS avg_rating
			FROM ratings r
			WHERE r.track_id=t.id
		) rr ON true
		LEFT JOIN LATERAL (
			SELECT COUNT(*)::bigint AS plays_7d
			FROM play_events p
			WHERE p.track_id=t.id
			  AND p.played_at >= now() - interval '7 day'
		) ps ON true
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*) FILTER (WHERE event_type='play_complete')::bigint AS complete_7d,
				COUNT(*) FILTER (WHERE event_type IN ('play_30s','play_50_percent'))::bigint AS mid_7d,
				COUNT(*) FILTER (WHERE event_type='playlist_add')::bigint AS playlist_7d,
				COUNT(*) FILTER (WHERE event_type='rating')::bigint AS rating_7d,
				COUNT(*) FILTER (WHERE event_type='skip_early')::bigint AS skip_7d
			FROM listening_events le
			WHERE le.track_id=t.id
			  AND le.created_at >= now() - interval '7 day'
		) ls ON true
		WHERE t.visibility='public'
		ORDER BY score DESC, lower(t.title)
		LIMIT 12
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	trendingTracks, err := s.scanTrackCards(trendingRows)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	topAlbumsRows, err := s.db.Query(r.Context(), `
		SELECT
			a.id,
			a.title,
			COALESCE(ar.name,'') AS artist,
			COALESCE(a.genre,'') AS genre,
			a.visibility,
			a.owner_sub,
			COALESCE(NULLIF(up.display_name,''), a.owner_sub) AS uploader_name,
			COALESCE(tc.track_count,0)::bigint AS track_count,
			(COALESCE(ap.plays,0) + COALESCE(al.score,0))::float8 AS score,
			'Top albums' AS reason
		FROM albums a
		LEFT JOIN artists ar ON ar.id=a.artist_id
		LEFT JOIN user_profiles up ON up.user_sub=a.owner_sub
		LEFT JOIN LATERAL (
			SELECT COUNT(*)::bigint AS track_count
			FROM tracks t
			WHERE t.album_id=a.id
		) tc ON true
		LEFT JOIN LATERAL (
			SELECT COUNT(*)::bigint AS plays
			FROM play_events p
			WHERE p.album_id=a.id
		) ap ON true
		LEFT JOIN LATERAL (
			SELECT COALESCE(SUM(
				CASE event_type
					WHEN 'play_complete' THEN 5
					WHEN 'play_50_percent' THEN 3
					WHEN 'play_30s' THEN 2
					WHEN 'playlist_add' THEN 4
					WHEN 'rating' THEN 3
					WHEN 'skip_early' THEN -2
					ELSE 0
				END
			),0)::bigint AS score
			FROM listening_events le
			WHERE le.album_id=a.id
			  AND le.created_at >= now() - interval '30 day'
		) al ON true
		WHERE a.visibility='public'
		ORDER BY score DESC, ap.plays DESC, lower(a.title)
		LIMIT 8
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	topAlbums, err := s.scanAlbumCards(topAlbumsRows)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	personal := map[string]any{
		"favorite_genres":    []string{},
		"recommended_tracks": []map[string]any{},
		"summary":            "Login to unlock personal discovery.",
		"enabled":            false,
	}
	if viewerSub != "" {
		type genrePref struct {
			Genre string
			Score float64
		}
		prefs := make([]genrePref, 0, 3)
		prefRows, err := s.db.Query(r.Context(), `
			WITH genre_pref AS (
				SELECT genre, SUM(weight)::float8 AS score
				FROM (
					SELECT COALESCE(t.genre,'') AS genre,
						CASE le.event_type
							WHEN 'play_complete' THEN 5
							WHEN 'play_50_percent' THEN 3
							WHEN 'play_30s' THEN 2
							WHEN 'playlist_add' THEN 5
							WHEN 'rating' THEN 4
							WHEN 'skip_early' THEN -3
							ELSE 1
						END::float8 AS weight
					FROM listening_events le
					JOIN tracks t ON t.id=le.track_id
					WHERE le.user_sub=$1
					  AND le.created_at >= now() - interval '120 day'
					  AND COALESCE(t.genre,'') <> ''
					UNION ALL
					SELECT COALESCE(t.genre,'') AS genre, (r.rating * 2)::float8 AS weight
					FROM ratings r
					JOIN tracks t ON t.id=r.track_id
					WHERE r.author_sub=$1
					  AND COALESCE(t.genre,'') <> ''
				) src
				GROUP BY genre
			)
			SELECT genre, score
			FROM genre_pref
			ORDER BY score DESC, genre ASC
			LIMIT 3
		`, viewerSub)
		if err == nil {
			defer prefRows.Close()
			for prefRows.Next() {
				var p genrePref
				if prefRows.Scan(&p.Genre, &p.Score) == nil {
					prefs = append(prefs, p)
				}
			}
		}
		favoriteGenres := make([]string, 0, len(prefs))
		for _, p := range prefs {
			if strings.TrimSpace(p.Genre) != "" {
				favoriteGenres = append(favoriteGenres, p.Genre)
			}
		}
		var recommended []map[string]any
		if len(favoriteGenres) > 0 {
			recRows, err := s.db.Query(r.Context(), `
				WITH genre_pref AS (
					SELECT genre, score
					FROM (
						SELECT genre, SUM(weight)::float8 AS score
						FROM (
							SELECT COALESCE(t.genre,'') AS genre,
								CASE le.event_type
									WHEN 'play_complete' THEN 5
									WHEN 'play_50_percent' THEN 3
									WHEN 'play_30s' THEN 2
									WHEN 'playlist_add' THEN 5
									WHEN 'rating' THEN 4
									WHEN 'skip_early' THEN -3
									ELSE 1
								END::float8 AS weight
							FROM listening_events le
							JOIN tracks t ON t.id=le.track_id
							WHERE le.user_sub=$1
							  AND le.created_at >= now() - interval '120 day'
							  AND COALESCE(t.genre,'') <> ''
							UNION ALL
							SELECT COALESCE(t.genre,'') AS genre, (r.rating * 2)::float8 AS weight
							FROM ratings r
							JOIN tracks t ON t.id=r.track_id
							WHERE r.author_sub=$1
							  AND COALESCE(t.genre,'') <> ''
						) pref_src
						GROUP BY genre
					) grouped
					ORDER BY score DESC, genre ASC
					LIMIT 3
				),
				heard AS (
					SELECT DISTINCT track_id
					FROM listening_events
					WHERE user_sub=$1
					UNION
					SELECT DISTINCT track_id
					FROM play_events
					WHERE user_sub=$1
				),
				followed AS (
					SELECT followed_sub
					FROM follows
					WHERE follower_sub=$1
				)
				SELECT
					t.id::text,
					t.title,
					COALESCE(ar.name,'') AS artist,
					COALESCE(al.title,'') AS album,
					COALESCE(t.genre,'') AS genre,
					COALESCE(rr.avg_rating,0) AS rating,
					t.duration_seconds,
					t.visibility,
					t.owner_sub,
					COALESCE(NULLIF(up.display_name,''), t.owner_sub) AS uploader_name,
					(
						gp.score * 4 +
						COALESCE(gs.score, 0) +
						CASE WHEN EXISTS(SELECT 1 FROM followed f WHERE f.followed_sub=t.owner_sub) THEN 3 ELSE 0 END +
						CASE WHEN t.created_at >= now() - interval '30 day' THEN 2 ELSE 0 END
					)::float8 AS score,
					('Matches your interest in ' || gp.genre) AS reason
				FROM tracks t
				JOIN genre_pref gp ON gp.genre = COALESCE(t.genre,'')
				LEFT JOIN artists ar ON ar.id=t.artist_id
				LEFT JOIN albums al ON al.id=t.album_id
				LEFT JOIN user_profiles up ON up.user_sub=t.owner_sub
				LEFT JOIN heard h ON h.track_id=t.id
				LEFT JOIN LATERAL (
					SELECT AVG(r.rating)::float8 AS avg_rating
					FROM ratings r
					WHERE r.track_id=t.id
				) rr ON true
				LEFT JOIN LATERAL (
					SELECT
						(
							COUNT(*) FILTER (WHERE event_type='play_complete') * 5 +
							COUNT(*) FILTER (WHERE event_type='play_50_percent') * 3 +
							COUNT(*) FILTER (WHERE event_type='play_30s') * 2 +
							COUNT(*) FILTER (WHERE event_type='playlist_add') * 4 +
							COUNT(*) FILTER (WHERE event_type='rating') * 3 -
							COUNT(*) FILTER (WHERE event_type='skip_early') * 2
						)::float8 AS score
					FROM listening_events le
					WHERE le.track_id=t.id
					  AND le.created_at >= now() - interval '30 day'
				) gs ON true
				WHERE t.visibility='public'
				  AND h.track_id IS NULL
				ORDER BY score DESC, lower(t.title)
				LIMIT 12
			`, viewerSub)
			if err == nil {
				recommended, err = s.scanTrackCards(recRows)
				if err != nil {
					recommended = []map[string]any{}
				}
			}
		}
		summary := "Still learning from your listening."
		if len(favoriteGenres) > 0 {
			summary = "Built from your strongest genres and listening patterns."
		}
		personal = map[string]any{
			"favorite_genres":    favoriteGenres,
			"recommended_tracks": recommended,
			"summary":            summary,
			"enabled":            true,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"global": map[string]any{
			"top_songs":       topTracks,
			"trending_tracks": trendingTracks,
			"top_albums":      topAlbums,
		},
		"personal":    personal,
		"server_time": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) listPlaylists(w http.ResponseWriter, r *http.Request) {
	claims, hasClaims := auth.FromContext(r.Context())
	isAdmin := hasClaims && auth.HasRole(claims, "admin")

	var rows pgx.Rows
	var err error
	if isAdmin {
		rows, err = s.db.Query(r.Context(), `
			SELECT p.id, p.name, p.visibility, p.owner_sub, p.created_at::text, count(pt.track_id)
			FROM playlists p
			LEFT JOIN playlist_tracks pt ON pt.playlist_id=p.id
			GROUP BY p.id
			ORDER BY p.created_at DESC
			LIMIT 500
		`)
	} else if hasClaims {
		rows, err = s.db.Query(r.Context(), `
			SELECT p.id, p.name, p.visibility, p.owner_sub, p.created_at::text, count(pt.track_id)
			FROM playlists p
			LEFT JOIN playlist_tracks pt ON pt.playlist_id=p.id
			WHERE p.visibility='public' OR p.owner_sub=$1
			GROUP BY p.id
			ORDER BY p.created_at DESC
			LIMIT 500
		`, claims.Subject)
	} else {
		rows, err = s.db.Query(r.Context(), `
			SELECT p.id, p.name, p.visibility, p.owner_sub, p.created_at::text, count(pt.track_id)
			FROM playlists p
			LEFT JOIN playlist_tracks pt ON pt.playlist_id=p.id
			WHERE p.visibility='public'
			GROUP BY p.id
			ORDER BY p.created_at DESC
			LIMIT 500
		`)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := make([]map[string]any, 0, 64)
	for rows.Next() {
		var id int64
		var name, visibility, ownerSub, created string
		var trackCount int64
		if err := rows.Scan(&id, &name, &visibility, &ownerSub, &created, &trackCount); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out = append(out, map[string]any{
			"id":          id,
			"name":        name,
			"visibility":  visibility,
			"owner_sub":   ownerSub,
			"track_count": trackCount,
			"created_at":  created,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"playlists": out})
}

func (s *Server) getPlaylist(w http.ResponseWriter, r *http.Request) {
	playlistID, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(r, "playlistID")), 10, 64)
	if err != nil || playlistID <= 0 {
		http.Error(w, "invalid playlist id", http.StatusBadRequest)
		return
	}
	claims, hasClaims := auth.FromContext(r.Context())
	viewerSub := ""
	isAdmin := false
	if hasClaims {
		viewerSub = claims.Subject
		isAdmin = auth.HasRole(claims, "admin")
	}
	allowed, _, err := s.canAccessPlaylist(r.Context(), viewerSub, isAdmin, playlistID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var name, visibility, ownerSub, created, updated string
	var trackCount int64
	err = s.db.QueryRow(r.Context(), `
		SELECT p.name, p.visibility, p.owner_sub, p.created_at::text, p.updated_at::text, count(pt.track_id)
		FROM playlists p
		LEFT JOIN playlist_tracks pt ON pt.playlist_id=p.id
		WHERE p.id=$1
		GROUP BY p.id
	`, playlistID).Scan(&name, &visibility, &ownerSub, &created, &updated, &trackCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":          playlistID,
		"name":        name,
		"visibility":  visibility,
		"owner_sub":   ownerSub,
		"track_count": trackCount,
		"created_at":  created,
		"updated_at":  updated,
	})
}

func (s *Server) listPlaylistTracks(w http.ResponseWriter, r *http.Request) {
	playlistID, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(r, "playlistID")), 10, 64)
	if err != nil || playlistID <= 0 {
		http.Error(w, "invalid playlist id", http.StatusBadRequest)
		return
	}
	claims, hasClaims := auth.FromContext(r.Context())
	viewerSub := ""
	isAdmin := false
	if hasClaims {
		viewerSub = claims.Subject
		isAdmin = auth.HasRole(claims, "admin")
	}
	allowed, ownerSub, err := s.canAccessPlaylist(r.Context(), viewerSub, isAdmin, playlistID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	isOwner := viewerSub != "" && ownerSub == viewerSub

	var rows pgx.Rows
	if isAdmin || isOwner {
		rows, err = s.db.Query(r.Context(), `
			SELECT t.id::text, t.title, COALESCE(ar.name,''), COALESCE(al.title,''), COALESCE(t.genre,''), t.duration_seconds, t.visibility, pt.position
			FROM playlist_tracks pt
			JOIN tracks t ON t.id=pt.track_id
			LEFT JOIN artists ar ON ar.id=t.artist_id
			LEFT JOIN albums al ON al.id=t.album_id
			WHERE pt.playlist_id=$1
			ORDER BY pt.position ASC, pt.added_at ASC
		`, playlistID)
	} else if viewerSub != "" {
		rows, err = s.db.Query(r.Context(), `
			SELECT t.id::text, t.title, COALESCE(ar.name,''), COALESCE(al.title,''), COALESCE(t.genre,''), t.duration_seconds, t.visibility, pt.position
			FROM playlist_tracks pt
			JOIN tracks t ON t.id=pt.track_id
			LEFT JOIN artists ar ON ar.id=t.artist_id
			LEFT JOIN albums al ON al.id=t.album_id
			WHERE pt.playlist_id=$1
			  AND (
			    t.visibility='public'
			    OR t.owner_sub=$2
			    OR (
			      t.visibility='followers_only'
			      AND EXISTS (
			        SELECT 1 FROM follows f
			        WHERE f.follower_sub=$2 AND f.followed_sub=t.owner_sub
			      )
			    )
			  )
			ORDER BY pt.position ASC, pt.added_at ASC
		`, playlistID, viewerSub)
	} else {
		rows, err = s.db.Query(r.Context(), `
			SELECT t.id::text, t.title, COALESCE(ar.name,''), COALESCE(al.title,''), COALESCE(t.genre,''), t.duration_seconds, t.visibility, pt.position
			FROM playlist_tracks pt
			JOIN tracks t ON t.id=pt.track_id
			LEFT JOIN artists ar ON ar.id=t.artist_id
			LEFT JOIN albums al ON al.id=t.album_id
			WHERE pt.playlist_id=$1
			  AND t.visibility='public'
			ORDER BY pt.position ASC, pt.added_at ASC
		`, playlistID)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := make([]map[string]any, 0, 128)
	for rows.Next() {
		var id, title, artist, album, genre, visibility string
		var duration float64
		var position int
		if err := rows.Scan(&id, &title, &artist, &album, &genre, &duration, &visibility, &position); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out = append(out, map[string]any{
			"id":               id,
			"title":            title,
			"artist":           artist,
			"album":            album,
			"genre":            genre,
			"duration_seconds": duration,
			"visibility":       visibility,
			"position":         position,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tracks": out})
}

func (s *Server) createPlaylist(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	var req struct {
		Name       string `json:"name"`
		Visibility string `json:"visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	if len(name) > 120 {
		http.Error(w, "name too long", http.StatusBadRequest)
		return
	}
	vis := normalizePlaylistVisibility(req.Visibility)
	var id int64
	if err := s.db.QueryRow(r.Context(), `
		INSERT INTO playlists(name, visibility, owner_sub) VALUES($1, $2, $3)
		RETURNING id
	`, name, vis, claims.Subject).Scan(&id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.writeAudit(r.Context(), claims.Subject, "playlist.create", "playlist", strconv.FormatInt(id, 10), map[string]any{"visibility": vis})
	writeJSON(w, http.StatusCreated, map[string]any{"status": "created", "id": id})
}

func (s *Server) updatePlaylist(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	playlistID, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(r, "playlistID")), 10, 64)
	if err != nil || playlistID <= 0 {
		http.Error(w, "invalid playlist id", http.StatusBadRequest)
		return
	}
	allowed, err := s.canManagePlaylist(r.Context(), claims, playlistID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Name       string `json:"name"`
		Visibility string `json:"visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(req.Name)
	vis := strings.TrimSpace(req.Visibility)
	if name == "" && vis == "" {
		http.Error(w, "no changes provided", http.StatusBadRequest)
		return
	}
	if len(name) > 120 {
		http.Error(w, "name too long", http.StatusBadRequest)
		return
	}
	if vis != "" {
		vis = normalizePlaylistVisibility(vis)
	}
	if _, err := s.db.Exec(r.Context(), `
		UPDATE playlists
		SET name = CASE WHEN $2 <> '' THEN $2 ELSE name END,
		    visibility = CASE WHEN $3 <> '' THEN $3 ELSE visibility END,
		    updated_at = now()
		WHERE id=$1
	`, playlistID, name, vis); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.writeAudit(r.Context(), claims.Subject, "playlist.update", "playlist", strconv.FormatInt(playlistID, 10), map[string]any{"name": name, "visibility": vis})
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) deletePlaylist(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	playlistID, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(r, "playlistID")), 10, 64)
	if err != nil || playlistID <= 0 {
		http.Error(w, "invalid playlist id", http.StatusBadRequest)
		return
	}
	allowed, err := s.canManagePlaylist(r.Context(), claims, playlistID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM playlists WHERE id=$1`, playlistID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.writeAudit(r.Context(), claims.Subject, "playlist.delete", "playlist", strconv.FormatInt(playlistID, 10), nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) addPlaylistTrack(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	playlistID, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(r, "playlistID")), 10, 64)
	if err != nil || playlistID <= 0 {
		http.Error(w, "invalid playlist id", http.StatusBadRequest)
		return
	}
	allowed, err := s.canManagePlaylist(r.Context(), claims, playlistID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		TrackID string `json:"track_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	trackID := strings.TrimSpace(req.TrackID)
	if trackID == "" {
		http.Error(w, "track_id required", http.StatusBadRequest)
		return
	}
	canTrack, err := s.canAccessTrack(r.Context(), claims.Subject, auth.HasRole(claims, "admin"), trackID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !canTrack {
		http.Error(w, "forbidden track", http.StatusForbidden)
		return
	}

	var pos int
	if err := s.db.QueryRow(r.Context(), `SELECT COALESCE(MAX(position),0)+1 FROM playlist_tracks WHERE playlist_id=$1`, playlistID).Scan(&pos); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := s.db.Exec(r.Context(), `
		INSERT INTO playlist_tracks(playlist_id, track_id, position)
		VALUES($1, $2, $3)
		ON CONFLICT (playlist_id, track_id) DO UPDATE
		SET position = EXCLUDED.position, added_at = now()
	`, playlistID, trackID, pos); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.writeAudit(r.Context(), claims.Subject, "playlist.add_track", "playlist", strconv.FormatInt(playlistID, 10), map[string]any{"track_id": trackID})
	_ = s.recordBehaviorEvent(r.Context(), claims.Subject, trackID, "playlist_add", "playlist", 0, 0, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

func (s *Server) removePlaylistTrack(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	playlistID, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(r, "playlistID")), 10, 64)
	if err != nil || playlistID <= 0 {
		http.Error(w, "invalid playlist id", http.StatusBadRequest)
		return
	}
	allowed, err := s.canManagePlaylist(r.Context(), claims, playlistID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	trackID := strings.TrimSpace(chi.URLParam(r, "trackID"))
	if trackID == "" {
		http.Error(w, "invalid track id", http.StatusBadRequest)
		return
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM playlist_tracks WHERE playlist_id=$1 AND track_id=$2`, playlistID, trackID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.writeAudit(r.Context(), claims.Subject, "playlist.remove_track", "playlist", strconv.FormatInt(playlistID, 10), map[string]any{"track_id": trackID})
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (s *Server) albumCover(w http.ResponseWriter, r *http.Request) {
	albumIDStr := chi.URLParam(r, "albumID")
	albumID, err := strconv.ParseInt(strings.TrimSpace(albumIDStr), 10, 64)
	if err != nil || albumID <= 0 {
		http.NotFound(w, r)
		return
	}

	allowSigned := false
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token != "" {
		exp, err := security.ParseExpires(r.URL.Query().Get("expires"))
		if err == nil && s.signer.Verify("album:"+albumIDStr, "cover", token, exp, time.Now()) {
			allowSigned = true
		}
	}

	claims, hasClaims := auth.FromContext(r.Context())
	viewerSub := ""
	isAdmin := false
	if hasClaims {
		viewerSub = claims.Subject
		isAdmin = auth.HasRole(claims, "admin")
	}

	allowed, coverPath, err := s.canAccessAlbumCover(r.Context(), albumID, viewerSub, isAdmin)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if allowSigned && strings.TrimSpace(coverPath) == "" {
		if cp, err := s.albumCoverPathByID(r.Context(), albumID); err == nil {
			coverPath = cp
		}
	}
	if (!allowed && !allowSigned) || strings.TrimSpace(coverPath) == "" {
		http.NotFound(w, r)
		return
	}

	f, err := os.Open(coverPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "open cover failed", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		http.Error(w, "stat cover failed", http.StatusInternalServerError)
		return
	}
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	_, _ = f.Seek(0, io.SeekStart)
	ctype := http.DetectContentType(buf[:n])
	if ctype == "" || ctype == "application/octet-stream" {
		ctype = mime.TypeByExtension(strings.ToLower(filepath.Ext(coverPath)))
		if ctype == "" {
			ctype = "image/jpeg"
		}
	}
	w.Header().Set("Content-Type", ctype)
	http.ServeContent(w, r, filepath.Base(coverPath), st.ModTime(), f)
}

func (s *Server) userAvatar(w http.ResponseWriter, r *http.Request) {
	userSub := strings.TrimSpace(chi.URLParam(r, "userSub"))
	if userSub == "" {
		http.NotFound(w, r)
		return
	}
	var avatarPath string
	if err := s.db.QueryRow(r.Context(), `SELECT COALESCE(avatar_path,'') FROM user_profiles WHERE user_sub=$1`, userSub).Scan(&avatarPath); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "avatar lookup failed", http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(avatarPath) == "" {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(avatarPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "open avatar failed", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.Error(w, "stat avatar failed", http.StatusInternalServerError)
		return
	}
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	_, _ = f.Seek(0, io.SeekStart)
	ctype := http.DetectContentType(buf[:n])
	if ctype == "" || ctype == "application/octet-stream" {
		ctype = mime.TypeByExtension(strings.ToLower(filepath.Ext(avatarPath)))
		if ctype == "" {
			ctype = "image/jpeg"
		}
	}
	w.Header().Set("Content-Type", ctype)
	http.ServeContent(w, r, filepath.Base(avatarPath), st.ModTime(), f)
}

func (s *Server) signAlbumCover(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	albumIDStr := chi.URLParam(r, "albumID")
	albumID, err := strconv.ParseInt(strings.TrimSpace(albumIDStr), 10, 64)
	if err != nil || albumID <= 0 {
		http.NotFound(w, r)
		return
	}

	allowed, _, err := s.canAccessAlbumCover(r.Context(), albumID, claims.Subject, auth.HasRole(claims, "admin"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	expires := time.Now().Add(s.cfg.SignedURLTTL)
	sig := s.signer.Sign("album:"+albumIDStr, "cover", expires)
	writeJSON(w, http.StatusOK, map[string]any{
		"token":        sig,
		"expires_unix": expires.Unix(),
		"url":          fmt.Sprintf("/api/v1/albums/%d/cover?token=%s&expires=%d", albumID, url.QueryEscape(sig), expires.Unix()),
	})
}

type importOptions struct {
	Title      string
	Artist     string
	Album      string
	Genre      string
	Visibility string
}

type importResult struct {
	TrackID string `json:"track_id"`
	Status  string `json:"status"`
	File    string `json:"file,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) importTrack(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	canUpload, err := s.canUserUpload(r.Context(), claims)
	if err != nil {
		http.Error(w, "upload permission check failed", http.StatusInternalServerError)
		return
	}
	if !canUpload {
		http.Error(w, "creator badge required", http.StatusForbidden)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadSizeBytes)
	if err := r.ParseMultipartForm(s.cfg.MaxUploadSizeBytes); err != nil {
		http.Error(w, "invalid multipart payload", http.StatusBadRequest)
		return
	}
	f, h, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer f.Close()

	opts := importOptions{
		Title:      strings.TrimSpace(r.FormValue("title")),
		Artist:     strings.TrimSpace(r.FormValue("artist")),
		Album:      strings.TrimSpace(r.FormValue("album")),
		Genre:      strings.TrimSpace(r.FormValue("genre")),
		Visibility: fallback(r.FormValue("visibility"), "private"),
	}

	res, statusCode, err := s.importSingleTrack(r.Context(), claims, f, h, opts)
	if err != nil {
		http.Error(w, err.Error(), statusCode)
		return
	}
	writeJSON(w, statusCode, res)
}

func (s *Server) importTrackBatch(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	canUpload, err := s.canUserUpload(r.Context(), claims)
	if err != nil {
		http.Error(w, "upload permission check failed", http.StatusInternalServerError)
		return
	}
	if !canUpload {
		http.Error(w, "creator badge required", http.StatusForbidden)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadSizeBytes)
	if err := r.ParseMultipartForm(s.cfg.MaxUploadSizeBytes); err != nil {
		http.Error(w, "invalid multipart payload", http.StatusBadRequest)
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		http.Error(w, "missing files field", http.StatusBadRequest)
		return
	}

	opts := importOptions{
		Artist:     strings.TrimSpace(r.FormValue("artist")),
		Album:      strings.TrimSpace(r.FormValue("album")),
		Genre:      strings.TrimSpace(r.FormValue("genre")),
		Visibility: fallback(r.FormValue("visibility"), "private"),
	}

	results := make([]importResult, 0, len(files))
	imported := 0
	deduped := 0
	failed := 0
	skipped := 0
	for _, h := range files {
		if !isSupportedAudioFile(h.Filename, h.Header.Get("Content-Type")) {
			skipped++
			results = append(results, importResult{File: h.Filename, Status: "skipped", Error: "unsupported non-audio file"})
			continue
		}
		f, err := h.Open()
		if err != nil {
			failed++
			results = append(results, importResult{File: h.Filename, Status: "failed", Error: "open file failed"})
			continue
		}
		res, _, impErr := s.importSingleTrack(r.Context(), claims, f, h, opts)
		_ = f.Close()
		if impErr != nil {
			failed++
			results = append(results, importResult{File: h.Filename, Status: "failed", Error: impErr.Error()})
			continue
		}
		res.File = h.Filename
		if res.Status == "imported" {
			imported++
		}
		if res.Status == "deduplicated" {
			deduped++
		}
		results = append(results, res)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"status":   "completed",
		"imported": imported,
		"deduped":  deduped,
		"failed":   failed,
		"skipped":  skipped,
		"results":  results,
	})
}

func (s *Server) importSingleTrack(ctx context.Context, claims auth.Claims, f multipart.File, h *multipart.FileHeader, opts importOptions) (importResult, int, error) {
	if !isSupportedAudioFile(h.Filename, h.Header.Get("Content-Type")) {
		return importResult{}, http.StatusBadRequest, fmt.Errorf("unsupported non-audio file")
	}

	visibility := normalizeVisibility(opts.Visibility)
	albumVisibility := "private"
	if visibility == "public" {
		albumVisibility = "public"
	}

	tempPath, hashHex, size, err := s.writeTempAndHash(f, h)
	if err != nil {
		return importResult{}, http.StatusInternalServerError, fmt.Errorf("failed to persist upload: %w", err)
	}
	defer os.Remove(tempPath)

	probe, err := media.ProbeFile(ctx, s.cfg.FFprobeBin, tempPath)
	if err != nil {
		return importResult{}, http.StatusBadRequest, fmt.Errorf("ffprobe failed: %w", err)
	}

	fileTitle := trimExt(filepath.Base(h.Filename))
	title := chooseImportedTitle(opts.Title, probe.Title, fileTitle)
	artist := normalizeImportedText(fallback(opts.Artist, fallback(probe.Artist, "Unknown Artist")), "Unknown Artist")
	album := normalizeImportedText(fallback(opts.Album, fallback(probe.Album, "Unknown Album")), "Unknown Album")
	genre := normalizeImportedText(fallback(opts.Genre, strings.TrimSpace(probe.Genre)), "")

	ext := strings.TrimPrefix(filepath.Ext(h.Filename), ".")
	if ext == "" {
		ext = codecToExt(probe.Codec)
	}
	if ext == "" {
		ext = "bin"
	}

	finalPath := s.store.OriginalsPath(hashHex, ext)
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return importResult{}, http.StatusInternalServerError, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return importResult{}, http.StatusInternalServerError, err
	}
	defer tx.Rollback(ctx)

	var existingTrackID string
	err = tx.QueryRow(ctx, `SELECT track_id::text FROM track_files WHERE file_hash=$1 LIMIT 1`, hashHex).Scan(&existingTrackID)
	if err == nil {
		_ = tx.Rollback(ctx)
		return importResult{TrackID: existingTrackID, Status: "deduplicated"}, http.StatusOK, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return importResult{}, http.StatusInternalServerError, err
	}

	if _, err := os.Stat(finalPath); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(tempPath, finalPath); err != nil {
			return importResult{}, http.StatusInternalServerError, fmt.Errorf("failed to move original: %w", err)
		}
	}

	artistID, err := upsertArtist(ctx, tx, artist)
	if err != nil {
		return importResult{}, http.StatusInternalServerError, err
	}
	albumID, err := upsertAlbum(ctx, tx, artistID, album, albumVisibility, claims.Subject, genre)
	if err != nil {
		return importResult{}, http.StatusInternalServerError, err
	}

	if probe.HasCover {
		coverPath := s.store.AlbumCoverPath(albumID)
		if _, statErr := os.Stat(coverPath); errors.Is(statErr, os.ErrNotExist) {
			if coverErr := media.ExtractCoverJPEG(ctx, s.cfg.FFmpegBin, finalPath, coverPath); coverErr == nil {
				if _, dbErr := tx.Exec(ctx, `UPDATE albums SET cover_path=$2 WHERE id=$1 AND COALESCE(cover_path,'')=''`, albumID, coverPath); dbErr != nil {
					return importResult{}, http.StatusInternalServerError, dbErr
				}
			}
		}
	}

	trackID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO tracks(id, title, artist_id, album_id, duration_seconds, visibility, owner_sub, genre, track_number)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, trackID, title, artistID, albumID, probe.Duration, visibility, claims.Subject, genre, probe.TrackNo); err != nil {
		return importResult{}, http.StatusInternalServerError, err
	}

	fileID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO track_files(id, track_id, file_hash, file_path, codec, bitrate, sample_rate, channels, size_bytes, is_original)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,true)
	`, fileID, trackID, hashHex, finalPath, probe.Codec, probe.Bitrate, probe.Rate, probe.Channels, size); err != nil {
		return importResult{}, http.StatusInternalServerError, err
	}

	if _, err := tx.Exec(ctx, `INSERT INTO transcode_jobs(track_id, source_file_id, status) VALUES($1,$2,'queued')`, trackID, fileID); err != nil {
		return importResult{}, http.StatusInternalServerError, err
	}

	if err := tx.Commit(ctx); err != nil {
		return importResult{}, http.StatusInternalServerError, err
	}

	if s.cfg.EnableDerivedSync {
		go s.processDerived(trackID, finalPath)
	}

	return importResult{TrackID: trackID, Status: "imported"}, http.StatusCreated, nil
}

func (s *Server) signStream(w http.ResponseWriter, r *http.Request) {
	claims, hasClaims := auth.FromContext(r.Context())
	viewerSub := ""
	isAdmin := false
	if hasClaims {
		viewerSub = claims.Subject
		isAdmin = auth.HasRole(claims, "admin")
	}

	var req struct {
		TrackID string `json:"track_id"`
		Format  string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	if req.TrackID == "" {
		http.Error(w, "track_id required", http.StatusBadRequest)
		return
	}
	if req.Format == "" {
		req.Format = "original"
	}

	allowed, err := s.canAccessTrack(r.Context(), viewerSub, isAdmin, req.TrackID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	expires := time.Now().Add(s.cfg.SignedURLTTL)
	sig := s.signer.Sign(req.TrackID, req.Format, expires)
	_ = s.registerPlayToken(r.Context(), sig, req.TrackID, viewerSub, expires)
	writeJSON(w, http.StatusOK, map[string]any{"token": sig, "expires_unix": expires.Unix()})
}

func (s *Server) streamTrack(w http.ResponseWriter, r *http.Request) {
	trackID := chi.URLParam(r, "trackID")
	format := fallback(r.URL.Query().Get("format"), "original")
	token := r.URL.Query().Get("token")
	exp, err := security.ParseExpires(r.URL.Query().Get("expires"))
	if err != nil || !s.signer.Verify(trackID, format, token, exp, time.Now()) {
		http.Error(w, "invalid or expired signature", http.StatusUnauthorized)
		return
	}

	path, mimeType, err := s.resolveStreamPath(r.Context(), trackID, format)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "open media failed", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		http.Error(w, "stat media failed", http.StatusInternalServerError)
		return
	}
	_ = s.recordPlayByToken(r.Context(), token)
	w.Header().Set("Content-Type", mimeType)
	http.ServeContent(w, r, filepath.Base(path), st.ModTime(), f)
}

func playbackTokenHash(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func (s *Server) registerPlayToken(ctx context.Context, token, trackID, userSub string, expires time.Time) error {
	hash := playbackTokenHash(token)
	if hash == "" || strings.TrimSpace(trackID) == "" {
		return nil
	}
	if strings.TrimSpace(userSub) == "" {
		userSub = ""
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO stream_play_tokens(token_hash, track_id, user_sub, expires_at)
		VALUES($1, $2, $3, $4)
		ON CONFLICT (token_hash) DO NOTHING
	`, hash, strings.TrimSpace(trackID), userSub, expires.UTC())
	return err
}

func (s *Server) recordPlayByToken(ctx context.Context, token string) error {
	hash := playbackTokenHash(token)
	if hash == "" {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var trackID string
	var userSub string
	err = tx.QueryRow(ctx, `
		UPDATE stream_play_tokens
		SET played_at = now()
		WHERE token_hash=$1
			AND played_at IS NULL
			AND expires_at > now()
		RETURNING track_id::text, COALESCE(user_sub,'')
	`, hash).Scan(&trackID, &userSub)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}

	var albumID *int64
	var aid int64
	if err := tx.QueryRow(ctx, `SELECT album_id FROM tracks WHERE id=$1`, trackID).Scan(&aid); err == nil {
		albumID = &aid
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO play_events(track_id, album_id, user_sub)
		VALUES($1, $2, $3)
	`, trackID, albumID, userSub); err != nil {
		return err
	}

	// Best-effort cleanup for expired tokens to keep table bounded.
	_, _ = tx.Exec(ctx, `DELETE FROM stream_play_tokens WHERE expires_at < now() - interval '1 day'`)

	return tx.Commit(ctx)
}

func (s *Server) createComment(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	trackID := chi.URLParam(r, "trackID")
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Content) == "" {
		http.Error(w, "invalid content", http.StatusBadRequest)
		return
	}
	if len(req.Content) > 2000 {
		http.Error(w, "content too long", http.StatusBadRequest)
		return
	}
	if _, err := s.db.Exec(r.Context(), `
		INSERT INTO comments(track_id, author_sub, author_name, content)
		VALUES($1, $2, $3, $4)
	`, trackID, claims.Subject, claims.Username, strings.TrimSpace(req.Content)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

func (s *Server) listComments(w http.ResponseWriter, r *http.Request) {
	trackID := chi.URLParam(r, "trackID")
	claims, hasClaims := auth.FromContext(r.Context())
	allowed, err := s.canAccessTrack(r.Context(), claims.Subject, hasClaims && auth.HasRole(claims, "admin"), trackID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT id, author_sub, COALESCE(author_name,''), content, created_at::text
		FROM comments WHERE track_id=$1 ORDER BY created_at DESC LIMIT 200
	`, trackID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0, 32)
	for rows.Next() {
		var id int64
		var authorSub, authorName, content, created string
		if err := rows.Scan(&id, &authorSub, &authorName, &content, &created); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out = append(out, map[string]any{"id": id, "author_sub": authorSub, "author_name": authorName, "content": content, "created_at": created})
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": out})
}

func (s *Server) createAlbumComment(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	albumID, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(r, "albumID")), 10, 64)
	if err != nil || albumID <= 0 {
		http.Error(w, "invalid album id", http.StatusBadRequest)
		return
	}
	allowed, err := s.canAccessAlbum(r.Context(), claims.Subject, auth.HasRole(claims, "admin"), albumID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Content) == "" {
		http.Error(w, "invalid content", http.StatusBadRequest)
		return
	}
	if len(req.Content) > 2000 {
		http.Error(w, "content too long", http.StatusBadRequest)
		return
	}
	if _, err := s.db.Exec(r.Context(), `
		INSERT INTO album_comments(album_id, author_sub, author_name, content)
		VALUES($1, $2, $3, $4)
	`, albumID, claims.Subject, claims.Username, strings.TrimSpace(req.Content)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

func (s *Server) listAlbumComments(w http.ResponseWriter, r *http.Request) {
	albumID, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(r, "albumID")), 10, 64)
	if err != nil || albumID <= 0 {
		http.Error(w, "invalid album id", http.StatusBadRequest)
		return
	}
	claims, hasClaims := auth.FromContext(r.Context())
	viewerSub := ""
	isAdmin := false
	if hasClaims {
		viewerSub = claims.Subject
		isAdmin = auth.HasRole(claims, "admin")
	}
	allowed, err := s.canAccessAlbum(r.Context(), viewerSub, isAdmin, albumID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT id, author_sub, COALESCE(author_name,''), content, created_at::text
		FROM album_comments
		WHERE album_id=$1
		ORDER BY created_at DESC
		LIMIT 200
	`, albumID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0, 32)
	for rows.Next() {
		var id int64
		var authorSub, authorName, content, created string
		if err := rows.Scan(&id, &authorSub, &authorName, &content, &created); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out = append(out, map[string]any{
			"id":          id,
			"album_id":    albumID,
			"author_sub":  authorSub,
			"author_name": authorName,
			"content":     content,
			"created_at":  created,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": out})
}

func (s *Server) rateTrack(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	trackID := chi.URLParam(r, "trackID")
	var req struct {
		Rating int `json:"rating"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Rating < 1 || req.Rating > 5 {
		http.Error(w, "rating must be 1..5", http.StatusBadRequest)
		return
	}
	allowed, err := s.canAccessTrack(r.Context(), claims.Subject, auth.HasRole(claims, "admin"), trackID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if _, err := s.db.Exec(r.Context(), `
		INSERT INTO ratings(track_id, author_sub, rating)
		VALUES($1,$2,$3)
		ON CONFLICT (track_id, author_sub)
		DO UPDATE SET rating=EXCLUDED.rating, created_at=now()
	`, trackID, claims.Subject, req.Rating); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.recordBehaviorEvent(r.Context(), claims.Subject, trackID, "rating", "track_rating", float64(req.Rating), 5, "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "rated"})
}

func (s *Server) userPublicProfile(w http.ResponseWriter, r *http.Request) {
	targetSub := strings.TrimSpace(chi.URLParam(r, "userSub"))
	if targetSub == "" {
		http.Error(w, "invalid user", http.StatusBadRequest)
		return
	}
	claims, hasClaims := auth.FromContext(r.Context())
	viewerSub := ""
	if hasClaims {
		viewerSub = claims.Subject
	}
	displayName, bio, avatarPath, err := s.userProfileBySub(r.Context(), targetSub)
	if err != nil {
		http.Error(w, "profile unavailable", http.StatusInternalServerError)
		return
	}
	var followerCount, followingCount int64
	_ = s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM follows WHERE followed_sub=$1`, targetSub).Scan(&followerCount)
	_ = s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM follows WHERE follower_sub=$1`, targetSub).Scan(&followingCount)
	var trackCount, albumCount int64
	_ = s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM tracks WHERE owner_sub=$1`, targetSub).Scan(&trackCount)
	_ = s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM albums WHERE owner_sub=$1`, targetSub).Scan(&albumCount)
	isFollowing := false
	if viewerSub != "" && viewerSub != targetSub {
		_ = s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM follows WHERE follower_sub=$1 AND followed_sub=$2)`, viewerSub, targetSub).Scan(&isFollowing)
	}
	creatorBadge, _ := s.userCreatorBadge(r.Context(), targetSub)
	avatarURL := ""
	if strings.TrimSpace(avatarPath) != "" {
		avatarURL = fmt.Sprintf("/api/v1/users/%s/avatar?v=%d", url.PathEscape(targetSub), time.Now().Unix())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"subject":       targetSub,
		"display_name":  displayName,
		"bio":           bio,
		"avatar_url":    avatarURL,
		"creator_badge": creatorBadge,
		"followers":     followerCount,
		"following":     followingCount,
		"track_uploads": trackCount,
		"album_uploads": albumCount,
		"is_following":  isFollowing,
		"is_self":       viewerSub != "" && viewerSub == targetSub,
	})
}

func (s *Server) userUploads(w http.ResponseWriter, r *http.Request) {
	targetSub := strings.TrimSpace(chi.URLParam(r, "userSub"))
	if targetSub == "" {
		http.Error(w, "invalid user", http.StatusBadRequest)
		return
	}
	claims, hasClaims := auth.FromContext(r.Context())
	viewerSub := ""
	isAdmin := false
	if hasClaims {
		viewerSub = claims.Subject
		isAdmin = auth.HasRole(claims, "admin")
	}
	self := viewerSub != "" && viewerSub == targetSub
	canSeeFollowersOnly := false
	if viewerSub != "" && viewerSub != targetSub {
		_ = s.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM follows WHERE follower_sub=$1 AND followed_sub=$2)`, viewerSub, targetSub).Scan(&canSeeFollowersOnly)
	}

	albumWhere := `a.owner_sub=$1 AND a.visibility='public'`
	trackWhere := `t.owner_sub=$1 AND t.visibility='public'`
	args := []any{targetSub}
	if isAdmin || self {
		albumWhere = `a.owner_sub=$1`
		trackWhere = `t.owner_sub=$1`
	} else if canSeeFollowersOnly {
		trackWhere = `t.owner_sub=$1 AND (t.visibility='public' OR t.visibility='followers_only')`
	}
	alRows, err := s.db.Query(r.Context(), `
		SELECT a.id, a.title, COALESCE(ar.name,''), a.visibility, a.created_at::text
		FROM albums a
		LEFT JOIN artists ar ON ar.id=a.artist_id
		WHERE `+albumWhere+`
		ORDER BY a.created_at DESC
		LIMIT 200
	`, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer alRows.Close()
	albums := make([]map[string]any, 0, 64)
	for alRows.Next() {
		var id int64
		var title, artist, vis, created string
		if err := alRows.Scan(&id, &title, &artist, &vis, &created); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		albums = append(albums, map[string]any{"id": id, "title": title, "artist": artist, "visibility": vis, "created_at": created})
	}

	trRows, err := s.db.Query(r.Context(), `
		SELECT t.id::text, t.title, COALESCE(ar.name,''), COALESCE(al.title,''), COALESCE(rr.avg_rating,0), t.visibility, t.created_at::text
		FROM tracks t
		LEFT JOIN artists ar ON ar.id=t.artist_id
		LEFT JOIN albums al ON al.id=t.album_id
		LEFT JOIN LATERAL (
			SELECT AVG(r.rating)::float8 AS avg_rating FROM ratings r WHERE r.track_id=t.id
		) rr ON true
		WHERE `+trackWhere+`
		ORDER BY t.created_at DESC
		LIMIT 500
	`, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer trRows.Close()
	tracks := make([]map[string]any, 0, 128)
	for trRows.Next() {
		var id, title, artist, album, vis, created string
		var rating float64
		if err := trRows.Scan(&id, &title, &artist, &album, &rating, &vis, &created); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tracks = append(tracks, map[string]any{"id": id, "title": title, "artist": artist, "album": album, "rating": rating, "visibility": vis, "created_at": created})
	}
	writeJSON(w, http.StatusOK, map[string]any{"albums": albums, "tracks": tracks})
}

func (s *Server) creatorStats(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	creatorBadge, _ := s.userCreatorBadge(r.Context(), claims.Subject)
	if !creatorBadge && !auth.HasRole(claims, "admin") {
		http.Error(w, "creator badge required", http.StatusForbidden)
		return
	}
	windowName, windowSQL := parseStatsWindow(r.URL.Query().Get("window"))
	targetSub := claims.Subject
	if auth.HasRole(claims, "admin") {
		if qs := strings.TrimSpace(r.URL.Query().Get("user_sub")); qs != "" {
			targetSub = qs
		}
	}
	filterPlay := ``
	filterListen := ``
	filterRatings := ``
	if windowSQL != "" {
		filterPlay = ` AND p.played_at >= ` + windowSQL
		filterListen = ` AND le.created_at >= ` + windowSQL
		filterRatings = ` AND r.created_at >= ` + windowSQL
	}

	overview := map[string]any{
		"tracks_total":      int64(0),
		"albums_total":      int64(0),
		"public_tracks":     int64(0),
		"public_albums":     int64(0),
		"plays_total":       int64(0),
		"unique_listeners":  int64(0),
		"guest_plays":       int64(0),
		"qualified_listens": int64(0),
		"completed_listens": int64(0),
		"early_skips":       int64(0),
		"playlist_adds":     int64(0),
		"ratings_count":     int64(0),
		"avg_rating":        float64(0),
	}
	var tracksTotal, albumsTotal, publicTracks, publicAlbums int64
	_ = s.db.QueryRow(r.Context(), `
		SELECT
			(SELECT COUNT(*)::bigint FROM tracks WHERE owner_sub=$1),
			(SELECT COUNT(*)::bigint FROM albums WHERE owner_sub=$1),
			(SELECT COUNT(*)::bigint FROM tracks WHERE owner_sub=$1 AND visibility='public'),
			(SELECT COUNT(*)::bigint FROM albums WHERE owner_sub=$1 AND visibility='public')
	`, targetSub).Scan(&tracksTotal, &albumsTotal, &publicTracks, &publicAlbums)
	overview["tracks_total"] = tracksTotal
	overview["albums_total"] = albumsTotal
	overview["public_tracks"] = publicTracks
	overview["public_albums"] = publicAlbums

	playsQuery := `
		SELECT
			COUNT(*)::bigint,
			COUNT(DISTINCT NULLIF(p.user_sub,''))::bigint,
			COUNT(*) FILTER (WHERE COALESCE(p.user_sub,'')='')::bigint
		FROM play_events p
		JOIN tracks t ON t.id=p.track_id
		WHERE t.owner_sub=$1` + filterPlay
	var playsTotal, uniqueListeners, guestPlays int64
	_ = s.db.QueryRow(r.Context(), playsQuery, targetSub).Scan(&playsTotal, &uniqueListeners, &guestPlays)
	overview["plays_total"] = playsTotal
	overview["unique_listeners"] = uniqueListeners
	overview["guest_plays"] = guestPlays

	listensQuery := `
		SELECT
			COUNT(*) FILTER (WHERE le.event_type IN ('play_30s','play_50_percent','play_complete'))::bigint,
			COUNT(*) FILTER (WHERE le.event_type='play_complete')::bigint,
			COUNT(*) FILTER (WHERE le.event_type='skip_early')::bigint,
			COUNT(*) FILTER (WHERE le.event_type='playlist_add')::bigint
		FROM listening_events le
		JOIN tracks t ON t.id=le.track_id
		WHERE t.owner_sub=$1` + filterListen
	var qualifiedListens, completedListens, earlySkips, playlistAdds int64
	_ = s.db.QueryRow(r.Context(), listensQuery, targetSub).Scan(&qualifiedListens, &completedListens, &earlySkips, &playlistAdds)
	overview["qualified_listens"] = qualifiedListens
	overview["completed_listens"] = completedListens
	overview["early_skips"] = earlySkips
	overview["playlist_adds"] = playlistAdds

	ratingQuery := `
		SELECT COUNT(*)::bigint, COALESCE(AVG(r.rating),0)::float8
		FROM ratings r
		JOIN tracks t ON t.id=r.track_id
		WHERE t.owner_sub=$1` + filterRatings
	var ratingsCount int64
	var avgRating float64
	_ = s.db.QueryRow(r.Context(), ratingQuery, targetSub).Scan(&ratingsCount, &avgRating)
	overview["ratings_count"] = ratingsCount
	overview["avg_rating"] = avgRating

	topTracksRows, err := s.db.Query(r.Context(), `
		SELECT
			t.id::text,
			t.title,
			COALESCE(ar.name,'') AS artist,
			COALESCE(al.title,'') AS album,
			COALESCE(t.genre,'') AS genre,
			COALESCE(rr.avg_rating,0) AS rating,
			t.duration_seconds,
			t.visibility,
			t.owner_sub,
			COALESCE(NULLIF(up.display_name,''), t.owner_sub) AS uploader_name,
			(
				COALESCE(ps.plays,0) +
				COALESCE(ls.complete_count,0) * 4 +
				COALESCE(ls.mid_count,0) * 2 +
				COALESCE(ls.playlist_count,0) * 4 +
				COALESCE(ls.rating_count,0) * 3 -
				COALESCE(ls.skip_count,0) * 2
			)::float8 AS score,
			'Creator top track' AS reason
		FROM tracks t
		LEFT JOIN artists ar ON ar.id=t.artist_id
		LEFT JOIN albums al ON al.id=t.album_id
		LEFT JOIN user_profiles up ON up.user_sub=t.owner_sub
		LEFT JOIN LATERAL (
			SELECT AVG(r.rating)::float8 AS avg_rating
			FROM ratings r
			WHERE r.track_id=t.id
		) rr ON true
		LEFT JOIN LATERAL (
			SELECT COUNT(*)::bigint AS plays
			FROM play_events p
			WHERE p.track_id=t.id`+strings.ReplaceAll(filterPlay, "p.", "p.")+`
		) ps ON true
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*) FILTER (WHERE event_type='play_complete')::bigint AS complete_count,
				COUNT(*) FILTER (WHERE event_type IN ('play_30s','play_50_percent'))::bigint AS mid_count,
				COUNT(*) FILTER (WHERE event_type='playlist_add')::bigint AS playlist_count,
				COUNT(*) FILTER (WHERE event_type='rating')::bigint AS rating_count,
				COUNT(*) FILTER (WHERE event_type='skip_early')::bigint AS skip_count
			FROM listening_events le
			WHERE le.track_id=t.id`+strings.ReplaceAll(filterListen, "le.", "le.")+`
		) ls ON true
		WHERE t.owner_sub=$1
		ORDER BY score DESC, lower(t.title)
		LIMIT 12
	`, targetSub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	topTracks, err := s.scanTrackCards(topTracksRows)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	topAlbumsRows, err := s.db.Query(r.Context(), `
		SELECT
			a.id,
			a.title,
			COALESCE(ar.name,'') AS artist,
			COALESCE(a.genre,'') AS genre,
			a.visibility,
			a.owner_sub,
			COALESCE(NULLIF(up.display_name,''), a.owner_sub) AS uploader_name,
			COALESCE(tc.track_count,0)::bigint AS track_count,
			(
				COALESCE(ps.plays,0) +
				COALESCE(ls.complete_count,0) * 4 +
				COALESCE(ls.mid_count,0) * 2 +
				COALESCE(ls.playlist_count,0) * 4 +
				COALESCE(ls.rating_count,0) * 3 -
				COALESCE(ls.skip_count,0) * 2
			)::float8 AS score,
			'Creator top album' AS reason
		FROM albums a
		LEFT JOIN artists ar ON ar.id=a.artist_id
		LEFT JOIN user_profiles up ON up.user_sub=a.owner_sub
		LEFT JOIN LATERAL (
			SELECT COUNT(*)::bigint AS track_count
			FROM tracks t
			WHERE t.album_id=a.id
		) tc ON true
		LEFT JOIN LATERAL (
			SELECT COUNT(*)::bigint AS plays
			FROM play_events p
			WHERE p.album_id=a.id`+strings.ReplaceAll(filterPlay, "p.", "p.")+`
		) ps ON true
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*) FILTER (WHERE event_type='play_complete')::bigint AS complete_count,
				COUNT(*) FILTER (WHERE event_type IN ('play_30s','play_50_percent'))::bigint AS mid_count,
				COUNT(*) FILTER (WHERE event_type='playlist_add')::bigint AS playlist_count,
				COUNT(*) FILTER (WHERE event_type='rating')::bigint AS rating_count,
				COUNT(*) FILTER (WHERE event_type='skip_early')::bigint AS skip_count
			FROM listening_events le
			WHERE le.album_id=a.id`+strings.ReplaceAll(filterListen, "le.", "le.")+`
		) ls ON true
		WHERE a.owner_sub=$1
		ORDER BY score DESC, lower(a.title)
		LIMIT 8
	`, targetSub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	topAlbums, err := s.scanAlbumCards(topAlbumsRows)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sourceRows, err := s.db.Query(r.Context(), `
		SELECT COALESCE(NULLIF(le.source_context,''), 'unknown') AS source_context, COUNT(*)::bigint AS events
		FROM listening_events le
		JOIN tracks t ON t.id=le.track_id
		WHERE t.owner_sub=$1`+filterListen+`
		GROUP BY source_context
		ORDER BY events DESC, source_context ASC
		LIMIT 12
	`, targetSub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer sourceRows.Close()
	sources := make([]map[string]any, 0, 12)
	for sourceRows.Next() {
		var source string
		var events int64
		if err := sourceRows.Scan(&source, &events); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		sources = append(sources, map[string]any{"source_context": source, "events": events})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"window":      windowName,
		"user_sub":    targetSub,
		"overview":    overview,
		"top_tracks":  topTracks,
		"top_albums":  topAlbums,
		"sources":     sources,
		"server_time": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) createUserProfileComment(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	targetSub := strings.TrimSpace(chi.URLParam(r, "userSub"))
	if targetSub == "" {
		http.Error(w, "invalid target", http.StatusBadRequest)
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Content) == "" {
		http.Error(w, "invalid content", http.StatusBadRequest)
		return
	}
	if len(req.Content) > 2000 {
		http.Error(w, "content too long", http.StatusBadRequest)
		return
	}
	if _, err := s.db.Exec(r.Context(), `
		INSERT INTO user_profile_comments(target_sub, author_sub, author_name, content)
		VALUES($1,$2,$3,$4)
	`, targetSub, claims.Subject, claims.Username, strings.TrimSpace(req.Content)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

func (s *Server) listUserProfileComments(w http.ResponseWriter, r *http.Request) {
	targetSub := strings.TrimSpace(chi.URLParam(r, "userSub"))
	if targetSub == "" {
		http.Error(w, "invalid target", http.StatusBadRequest)
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT id, author_sub, COALESCE(author_name,''), content, created_at::text
		FROM user_profile_comments
		WHERE target_sub=$1
		ORDER BY created_at DESC
		LIMIT 200
	`, targetSub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0, 64)
	for rows.Next() {
		var id int64
		var authorSub, authorName, content, created string
		if err := rows.Scan(&id, &authorSub, &authorName, &content, &created); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out = append(out, map[string]any{"id": id, "target_sub": targetSub, "author_sub": authorSub, "author_name": authorName, "content": content, "created_at": created})
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": out})
}

func (s *Server) followUser(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	target := chi.URLParam(r, "userSub")
	if strings.TrimSpace(target) == "" || target == claims.Subject {
		http.Error(w, "invalid target", http.StatusBadRequest)
		return
	}
	if _, err := s.db.Exec(r.Context(), `
		INSERT INTO follows(follower_sub, followed_sub) VALUES($1,$2)
		ON CONFLICT DO NOTHING
	`, claims.Subject, target); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "following"})
}

func (s *Server) unfollowUser(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.FromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	target := chi.URLParam(r, "userSub")
	if _, err := s.db.Exec(r.Context(), `DELETE FROM follows WHERE follower_sub=$1 AND followed_sub=$2`, claims.Subject, target); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unfollowed"})
}

func (s *Server) manageListTracks(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	isAdmin := auth.HasRole(claims, "admin")
	var rows pgx.Rows
	var err error
	if isAdmin {
		rows, err = s.db.Query(r.Context(), `
			SELECT t.id::text, t.title, COALESCE(ar.name,''), COALESCE(al.title,''), COALESCE(t.genre,''), t.visibility, (COALESCE(t.lyrics_srt,'') <> ''), (COALESCE(t.lyrics_txt,'') <> ''), t.created_at::text
			FROM tracks t
			LEFT JOIN artists ar ON ar.id=t.artist_id
			LEFT JOIN albums al ON al.id=t.album_id
			ORDER BY t.created_at DESC
			LIMIT 2000
		`)
	} else {
		rows, err = s.db.Query(r.Context(), `
			SELECT t.id::text, t.title, COALESCE(ar.name,''), COALESCE(al.title,''), COALESCE(t.genre,''), t.visibility, (COALESCE(t.lyrics_srt,'') <> ''), (COALESCE(t.lyrics_txt,'') <> ''), t.created_at::text
			FROM tracks t
			LEFT JOIN artists ar ON ar.id=t.artist_id
			LEFT JOIN albums al ON al.id=t.album_id
			WHERE t.owner_sub=$1
			ORDER BY t.created_at DESC
			LIMIT 2000
		`, claims.Subject)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := make([]map[string]any, 0, 256)
	for rows.Next() {
		var id, title, artist, album, genre, visibility, created string
		var hasLyricsSRT, hasLyricsTXT bool
		if err := rows.Scan(&id, &title, &artist, &album, &genre, &visibility, &hasLyricsSRT, &hasLyricsTXT, &created); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out = append(out, map[string]any{
			"id":             id,
			"title":          title,
			"artist":         artist,
			"album":          album,
			"genre":          genre,
			"visibility":     visibility,
			"has_lyrics":     hasLyricsSRT || hasLyricsTXT,
			"has_lyrics_srt": hasLyricsSRT,
			"has_lyrics_txt": hasLyricsTXT,
			"created_at":     created,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tracks": out})
}

func (s *Server) manageGetTrack(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	trackID := chi.URLParam(r, "trackID")
	allowed, err := s.canManageTrack(r.Context(), claims, trackID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var title, artist, album, genre, visibility, lyricsSRT, lyricsTXT, ownerSub string
	var albumID int64
	err = s.db.QueryRow(r.Context(), `
		SELECT t.title, COALESCE(ar.name,''), COALESCE(al.title,''), COALESCE(t.genre,''), t.visibility, COALESCE(t.lyrics_srt,''), COALESCE(t.lyrics_txt,''), t.owner_sub, COALESCE(al.id,0)
		FROM tracks t
		LEFT JOIN artists ar ON ar.id=t.artist_id
		LEFT JOIN albums al ON al.id=t.album_id
		WHERE t.id=$1
	`, trackID).Scan(&title, &artist, &album, &genre, &visibility, &lyricsSRT, &lyricsTXT, &ownerSub, &albumID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var coverPath string
	if albumID > 0 {
		_ = s.db.QueryRow(r.Context(), `SELECT COALESCE(cover_path,'') FROM albums WHERE id=$1`, albumID).Scan(&coverPath)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":         trackID,
		"title":      title,
		"artist":     artist,
		"album":      album,
		"genre":      genre,
		"visibility": visibility,
		"lyrics_srt": lyricsSRT,
		"lyrics_txt": lyricsTXT,
		"owner_sub":  ownerSub,
		"album_id":   albumID,
		"cover_path": coverPath,
	})
}

func (s *Server) manageUpdateTrack(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	trackID := chi.URLParam(r, "trackID")
	allowed, err := s.canManageTrack(r.Context(), claims, trackID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Title      *string `json:"title"`
		Artist     *string `json:"artist"`
		Album      *string `json:"album"`
		Genre      *string `json:"genre"`
		Visibility *string `json:"visibility"`
		LyricsSRT  *string `json:"lyrics_srt"`
		LyricsTXT  *string `json:"lyrics_txt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	var ownerSub, currentTitle, currentArtist, currentAlbum, currentGenre, currentVisibility, currentLyricsSRT, currentLyricsTXT string
	if err := tx.QueryRow(r.Context(), `SELECT owner_sub FROM tracks WHERE id=$1`, trackID).Scan(&ownerSub); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tx.QueryRow(r.Context(), `
		SELECT t.title, COALESCE(ar.name,''), COALESCE(al.title,''), COALESCE(t.genre,''), t.visibility, COALESCE(t.lyrics_srt,''), COALESCE(t.lyrics_txt,'')
		FROM tracks t
		LEFT JOIN artists ar ON ar.id=t.artist_id
		LEFT JOIN albums al ON al.id=t.album_id
		WHERE t.id=$1
	`, trackID).Scan(&currentTitle, &currentArtist, &currentAlbum, &currentGenre, &currentVisibility, &currentLyricsSRT, &currentLyricsTXT); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	title := currentTitle
	if req.Title != nil {
		title = fallback(strings.TrimSpace(*req.Title), "Untitled")
	}
	artist := currentArtist
	if req.Artist != nil {
		artist = fallback(strings.TrimSpace(*req.Artist), "Unknown Artist")
	}
	album := currentAlbum
	if req.Album != nil {
		album = fallback(strings.TrimSpace(*req.Album), "Unknown Album")
	}
	genre := currentGenre
	if req.Genre != nil {
		genre = strings.TrimSpace(*req.Genre)
	}
	vis := currentVisibility
	if req.Visibility != nil {
		vis = normalizeVisibility(strings.TrimSpace(*req.Visibility))
	}
	lyricsSRT := currentLyricsSRT
	if req.LyricsSRT != nil {
		lyricsSRT = *req.LyricsSRT
	}
	lyricsTXT := currentLyricsTXT
	if req.LyricsTXT != nil {
		lyricsTXT = *req.LyricsTXT
	}
	artistID, err := upsertArtist(r.Context(), tx, artist)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	albumVisibility := "private"
	if vis == "public" {
		albumVisibility = "public"
	}
	albumID, err := upsertAlbum(r.Context(), tx, artistID, album, albumVisibility, ownerSub, genre)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE tracks
		SET title=$2, artist_id=$3, album_id=$4, genre=$5, visibility=$6, lyrics_srt=$7, lyrics_txt=$8
		WHERE id=$1
	`, trackID, title, artistID, albumID, genre, vis, lyricsSRT, lyricsTXT); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) manageDeleteTrack(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	trackID := chi.URLParam(r, "trackID")
	allowed, err := s.canManageTrack(r.Context(), claims, trackID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM tracks WHERE id=$1`, trackID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.writeAudit(r.Context(), claims.Subject, "manage.delete_track", "track", trackID, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) manageTrackCoverUpload(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	trackID := chi.URLParam(r, "trackID")
	allowed, err := s.canManageTrack(r.Context(), claims, trackID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var albumID int64
	if err := s.db.QueryRow(r.Context(), `SELECT COALESCE(album_id,0) FROM tracks WHERE id=$1`, trackID).Scan(&albumID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if albumID == 0 {
		http.Error(w, "track has no album", http.StatusBadRequest)
		return
	}
	if err := s.saveAlbumCoverFromRequest(r.Context(), r, albumID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cover_updated"})
}

func (s *Server) manageTrackLyricsUpload(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	trackID := chi.URLParam(r, "trackID")
	allowed, err := s.canManageTrack(r.Context(), claims, trackID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1*1024*1024)
	if err := r.ParseMultipartForm(1 * 1024 * 1024); err != nil {
		http.Error(w, "invalid multipart payload", http.StatusBadRequest)
		return
	}
	f, h, err := r.FormFile("lyrics")
	if err != nil {
		http.Error(w, "missing lyrics file", http.StatusBadRequest)
		return
	}
	defer f.Close()
	if strings.ToLower(filepath.Ext(h.Filename)) != ".srt" {
		http.Error(w, "lyrics must be .srt", http.StatusBadRequest)
		return
	}
	data, err := io.ReadAll(io.LimitReader(f, 1*1024*1024))
	if err != nil {
		http.Error(w, "read lyrics failed", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		http.Error(w, "empty lyrics", http.StatusBadRequest)
		return
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE tracks SET lyrics_srt=$2 WHERE id=$1`, trackID, text); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "lyrics_updated"})
}

func (s *Server) manageTrackLyricsPlainUpload(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	trackID := chi.URLParam(r, "trackID")
	allowed, err := s.canManageTrack(r.Context(), claims, trackID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1*1024*1024)
	if err := r.ParseMultipartForm(1 * 1024 * 1024); err != nil {
		http.Error(w, "invalid multipart payload", http.StatusBadRequest)
		return
	}
	f, h, err := r.FormFile("lyrics")
	if err != nil {
		http.Error(w, "missing lyrics file", http.StatusBadRequest)
		return
	}
	defer f.Close()
	if strings.ToLower(filepath.Ext(h.Filename)) != ".txt" {
		http.Error(w, "lyrics must be .txt", http.StatusBadRequest)
		return
	}
	data, err := io.ReadAll(io.LimitReader(f, 1*1024*1024))
	if err != nil {
		http.Error(w, "read lyrics failed", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		http.Error(w, "empty lyrics", http.StatusBadRequest)
		return
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE tracks SET lyrics_txt=$2 WHERE id=$1`, trackID, text); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "lyrics_plain_updated"})
}

func (s *Server) manageListAlbums(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	isAdmin := auth.HasRole(claims, "admin")
	var rows pgx.Rows
	var err error
	if isAdmin {
		rows, err = s.db.Query(r.Context(), `
			SELECT a.id, a.title, COALESCE(ar.name,''), a.visibility, COALESCE(a.genre,''), COALESCE(a.cover_path,''), count(t.id)
			FROM albums a
			LEFT JOIN artists ar ON ar.id=a.artist_id
			LEFT JOIN tracks t ON t.album_id=a.id
			GROUP BY a.id, ar.name
			ORDER BY a.created_at DESC
			LIMIT 1000
		`)
	} else {
		rows, err = s.db.Query(r.Context(), `
			SELECT a.id, a.title, COALESCE(ar.name,''), a.visibility, COALESCE(a.genre,''), COALESCE(a.cover_path,''), count(t.id)
			FROM albums a
			LEFT JOIN artists ar ON ar.id=a.artist_id
			LEFT JOIN tracks t ON t.album_id=a.id
			WHERE a.owner_sub=$1
			GROUP BY a.id, ar.name
			ORDER BY a.created_at DESC
			LIMIT 1000
		`, claims.Subject)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0, 128)
	for rows.Next() {
		var id int64
		var title, artist, visibility, genre, coverPath string
		var tracks int64
		if err := rows.Scan(&id, &title, &artist, &visibility, &genre, &coverPath, &tracks); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out = append(out, map[string]any{
			"id":          id,
			"title":       title,
			"artist":      artist,
			"visibility":  visibility,
			"genre":       genre,
			"cover_path":  coverPath,
			"track_count": tracks,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"albums": out})
}

func (s *Server) manageGetAlbum(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	albumID, err := strconv.ParseInt(chi.URLParam(r, "albumID"), 10, 64)
	if err != nil || albumID <= 0 {
		http.NotFound(w, r)
		return
	}
	allowed, err := s.canManageAlbum(r.Context(), claims, albumID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var title, artist, visibility, genre, coverPath, ownerSub string
	if err := s.db.QueryRow(r.Context(), `
		SELECT a.title, COALESCE(ar.name,''), a.visibility, COALESCE(a.genre,''), COALESCE(a.cover_path,''), a.owner_sub
		FROM albums a
		LEFT JOIN artists ar ON ar.id=a.artist_id
		WHERE a.id=$1
	`, albumID).Scan(&title, &artist, &visibility, &genre, &coverPath, &ownerSub); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":         albumID,
		"title":      title,
		"artist":     artist,
		"visibility": visibility,
		"genre":      genre,
		"cover_path": coverPath,
		"owner_sub":  ownerSub,
	})
}

func (s *Server) manageUpdateAlbum(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	albumID, err := strconv.ParseInt(chi.URLParam(r, "albumID"), 10, 64)
	if err != nil || albumID <= 0 {
		http.NotFound(w, r)
		return
	}
	allowed, err := s.canManageAlbum(r.Context(), claims, albumID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Title      *string `json:"title"`
		Artist     *string `json:"artist"`
		Visibility *string `json:"visibility"`
		Genre      *string `json:"genre"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())
	var currentTitle, currentArtist, currentVisibility, currentGenre string
	if err := tx.QueryRow(r.Context(), `
		SELECT a.title, COALESCE(ar.name,''), a.visibility, COALESCE(a.genre,'')
		FROM albums a
		LEFT JOIN artists ar ON ar.id=a.artist_id
		WHERE a.id=$1
	`, albumID).Scan(&currentTitle, &currentArtist, &currentVisibility, &currentGenre); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	title := currentTitle
	if req.Title != nil {
		title = fallback(strings.TrimSpace(*req.Title), "Unknown Album")
	}
	artist := currentArtist
	if req.Artist != nil {
		artist = fallback(strings.TrimSpace(*req.Artist), "Unknown Artist")
	}
	genre := currentGenre
	if req.Genre != nil {
		genre = strings.TrimSpace(*req.Genre)
	}
	vis := currentVisibility
	if req.Visibility != nil {
		switch strings.TrimSpace(*req.Visibility) {
		case "public", "private":
			vis = strings.TrimSpace(*req.Visibility)
		default:
			vis = "private"
		}
	}
	artistID, err := upsertArtist(r.Context(), tx, artist)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE albums SET title=$2, artist_id=$3, visibility=$4, genre=$5 WHERE id=$1
	`, albumID, title, artistID, vis, genre); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Hierarchy rule: public album implies all contained tracks become public.
	if vis == "public" {
		if _, err := tx.Exec(r.Context(), `UPDATE tracks SET visibility='public' WHERE album_id=$1 AND visibility <> 'public'`, albumID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) manageAlbumCoverUpload(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	albumID, err := strconv.ParseInt(chi.URLParam(r, "albumID"), 10, 64)
	if err != nil || albumID <= 0 {
		http.NotFound(w, r)
		return
	}
	allowed, err := s.canManageAlbum(r.Context(), claims, albumID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := s.saveAlbumCoverFromRequest(r.Context(), r, albumID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cover_updated"})
}

func (s *Server) adminListJobs(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `
		SELECT
			j.id,
			j.track_id::text,
			COALESCE(t.title,'') AS track_title,
			COALESCE(a.title,'') AS album_title,
			j.status,
			j.attempts,
			COALESCE(j.error_text,''),
			j.updated_at::text
		FROM transcode_jobs j
		LEFT JOIN tracks t ON t.id=j.track_id
		LEFT JOIN albums a ON a.id=t.album_id
		ORDER BY j.updated_at DESC
		LIMIT 500
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	jobs := make([]map[string]any, 0, 64)
	for rows.Next() {
		var id int64
		var trackID, trackTitle, albumTitle, status, errText, updated string
		var attempts int
		if err := rows.Scan(&id, &trackID, &trackTitle, &albumTitle, &status, &attempts, &errText, &updated); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jobs = append(jobs, map[string]any{
			"id":          id,
			"track_id":    trackID,
			"track_title": trackTitle,
			"album_title": albumTitle,
			"status":      status,
			"attempts":    attempts,
			"error":       errText,
			"updated_at":  updated,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (s *Server) adminSetVisibility(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	trackID := chi.URLParam(r, "trackID")
	var req struct {
		Visibility string `json:"visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	vis := strings.TrimSpace(req.Visibility)
	switch vis {
	case "private", "unlisted", "followers_only", "public":
	default:
		http.Error(w, "invalid visibility", http.StatusBadRequest)
		return
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE tracks SET visibility=$2 WHERE id=$1`, trackID, vis); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.writeAudit(r.Context(), claims.Subject, "admin.set_visibility", "track", trackID, map[string]any{"visibility": vis})
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) adminDeleteTrack(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	trackID := chi.URLParam(r, "trackID")
	if _, err := s.db.Exec(r.Context(), `DELETE FROM tracks WHERE id=$1`, trackID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.writeAudit(r.Context(), claims.Subject, "admin.delete_track", "track", trackID, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) canAccessTrack(ctx context.Context, viewerSub string, isAdmin bool, trackID string) (bool, error) {
	var vis, owner string
	err := s.db.QueryRow(ctx, `SELECT visibility, owner_sub FROM tracks WHERE id=$1`, trackID).Scan(&vis, &owner)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if isAdmin || (viewerSub != "" && viewerSub == owner) {
		return true, nil
	}
	switch vis {
	case "public", "unlisted":
		return true, nil
	case "private":
		return false, nil
	case "followers_only":
		if viewerSub == "" {
			return false, nil
		}
		var exists bool
		if err := s.db.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM follows WHERE follower_sub=$1 AND followed_sub=$2)
		`, viewerSub, owner).Scan(&exists); err != nil {
			return false, err
		}
		return exists, nil
	default:
		return false, nil
	}
}

func (s *Server) canAccessAlbum(ctx context.Context, viewerSub string, isAdmin bool, albumID int64) (bool, error) {
	var vis, owner string
	err := s.db.QueryRow(ctx, `SELECT visibility, owner_sub FROM albums WHERE id=$1`, albumID).Scan(&vis, &owner)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if isAdmin || (viewerSub != "" && viewerSub == owner) {
		return true, nil
	}
	return vis == "public", nil
}

func (s *Server) canAccessAlbumCover(ctx context.Context, albumID int64, viewerSub string, isAdmin bool) (bool, string, error) {
	var visibility, ownerSub, coverPath string
	err := s.db.QueryRow(ctx, `SELECT visibility, owner_sub, COALESCE(cover_path,'') FROM albums WHERE id=$1`, albumID).
		Scan(&visibility, &ownerSub, &coverPath)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, "", nil
		}
		return false, "", err
	}
	if isAdmin || (viewerSub != "" && viewerSub == ownerSub) || visibility == "public" {
		return true, coverPath, nil
	}
	return false, "", nil
}

func (s *Server) canAccessPlaylist(ctx context.Context, viewerSub string, isAdmin bool, playlistID int64) (bool, string, error) {
	var visibility, ownerSub string
	err := s.db.QueryRow(ctx, `SELECT visibility, owner_sub FROM playlists WHERE id=$1`, playlistID).Scan(&visibility, &ownerSub)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, "", nil
		}
		return false, "", err
	}
	if isAdmin || (viewerSub != "" && viewerSub == ownerSub) || visibility == "public" {
		return true, ownerSub, nil
	}
	return false, ownerSub, nil
}

func (s *Server) albumCoverPathByID(ctx context.Context, albumID int64) (string, error) {
	var coverPath string
	if err := s.db.QueryRow(ctx, `SELECT COALESCE(cover_path,'') FROM albums WHERE id=$1`, albumID).Scan(&coverPath); err != nil {
		return "", err
	}
	return coverPath, nil
}

func (s *Server) canManageTrack(ctx context.Context, claims auth.Claims, trackID string) (bool, error) {
	if auth.HasRole(claims, "admin") {
		return true, nil
	}
	var ownerSub string
	if err := s.db.QueryRow(ctx, `SELECT owner_sub FROM tracks WHERE id=$1`, trackID).Scan(&ownerSub); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return ownerSub != "" && ownerSub == claims.Subject, nil
}

func (s *Server) canManageAlbum(ctx context.Context, claims auth.Claims, albumID int64) (bool, error) {
	if auth.HasRole(claims, "admin") {
		return true, nil
	}
	var ownerSub string
	if err := s.db.QueryRow(ctx, `SELECT owner_sub FROM albums WHERE id=$1`, albumID).Scan(&ownerSub); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return ownerSub != "" && ownerSub == claims.Subject, nil
}

func (s *Server) canManagePlaylist(ctx context.Context, claims auth.Claims, playlistID int64) (bool, error) {
	if auth.HasRole(claims, "admin") {
		return true, nil
	}
	var ownerSub string
	if err := s.db.QueryRow(ctx, `SELECT owner_sub FROM playlists WHERE id=$1`, playlistID).Scan(&ownerSub); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return ownerSub != "" && ownerSub == claims.Subject, nil
}

func (s *Server) saveAlbumCoverFromRequest(ctx context.Context, r *http.Request, albumID int64) error {
	if err := r.ParseMultipartForm(10 * 1024 * 1024); err != nil {
		return fmt.Errorf("invalid multipart payload")
	}
	f, h, err := r.FormFile("cover")
	if err != nil {
		return fmt.Errorf("missing cover file")
	}
	defer f.Close()

	ext := strings.ToLower(filepath.Ext(h.Filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
	default:
		return fmt.Errorf("cover must be jpg/png/webp")
	}
	target := s.store.AlbumCoverPathWithExt(albumID, strings.TrimPrefix(ext, "."))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.store.TempDir(), "cover-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, io.LimitReader(f, 10*1024*1024)); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `UPDATE albums SET cover_path=$2 WHERE id=$1`, albumID, target); err != nil {
		return err
	}
	return nil
}

func (s *Server) adminGetSettings(w http.ResponseWriter, r *http.Request) {
	enabled, err := s.isRegistrationEnabled(r.Context())
	if err != nil {
		http.Error(w, "settings unavailable", http.StatusInternalServerError)
		return
	}
	debugEnabled, err := s.isDebugLoggingEnabled(r.Context())
	if err != nil {
		http.Error(w, "settings unavailable", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"registration_enabled":  enabled,
		"debug_logging_enabled": debugEnabled,
	})
}

func (s *Server) adminUpdateSettings(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	var req struct {
		RegistrationEnabled *bool `json:"registration_enabled"`
		DebugLoggingEnabled *bool `json:"debug_logging_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	if req.RegistrationEnabled == nil && req.DebugLoggingEnabled == nil {
		http.Error(w, "no changes provided", http.StatusBadRequest)
		return
	}
	updated := map[string]any{}
	if req.RegistrationEnabled != nil {
		if err := s.setRegistrationEnabled(r.Context(), *req.RegistrationEnabled); err != nil {
			http.Error(w, "update settings failed", http.StatusInternalServerError)
			return
		}
		updated["registration_enabled"] = *req.RegistrationEnabled
	}
	if req.DebugLoggingEnabled != nil {
		if err := s.setDebugLoggingEnabled(r.Context(), *req.DebugLoggingEnabled); err != nil {
			http.Error(w, "update settings failed", http.StatusInternalServerError)
			return
		}
		updated["debug_logging_enabled"] = *req.DebugLoggingEnabled
	}
	_ = s.writeAudit(r.Context(), claims.Subject, "admin.update_settings", "system", "settings", updated)
	writeJSON(w, http.StatusOK, map[string]any{"status": "updated", "updated": updated})
}

func (s *Server) adminCreateInvite(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	var req struct {
		TTLMinutes int `json:"ttl_minutes"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	ttl := time.Duration(req.TTLMinutes) * time.Minute
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	if ttl > 7*24*time.Hour {
		ttl = 7 * 24 * time.Hour
	}
	token, expiresAt, err := s.createRegistrationInvite(r.Context(), claims.Subject, ttl)
	if err != nil {
		http.Error(w, "invite creation failed", http.StatusInternalServerError)
		return
	}
	link := fmt.Sprintf("%s/register?invite=%s", externalBaseURL(r), url.QueryEscape(token))
	_ = s.writeAudit(r.Context(), claims.Subject, "admin.create_invite", "registration_invite", token[:8], map[string]any{"expires_at": expiresAt.UTC().Format(time.RFC3339)})
	writeJSON(w, http.StatusCreated, map[string]any{
		"status":      "created",
		"invite_link": link,
		"token":       token,
		"expires_at":  expiresAt.UTC().Format(time.RFC3339),
		"ttl_minutes": int(ttl.Minutes()),
	})
}

func (s *Server) adminListInvites(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `
		SELECT id, COALESCE(token_plain,''), created_by, COALESCE(used_by,''), expires_at::text, COALESCE(used_at::text,''), created_at::text
		FROM registration_invites
		ORDER BY created_at DESC
		LIMIT 300
	`)
	if err != nil {
		http.Error(w, "list invites failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	base := externalBaseURL(r)
	out := make([]map[string]any, 0, 64)
	for rows.Next() {
		var id int64
		var tokenPlain, createdBy, usedBy, expiresAt, usedAt, createdAt string
		if err := rows.Scan(&id, &tokenPlain, &createdBy, &usedBy, &expiresAt, &usedAt, &createdAt); err != nil {
			http.Error(w, "list invites failed", http.StatusInternalServerError)
			return
		}
		link := ""
		if strings.TrimSpace(tokenPlain) != "" {
			link = fmt.Sprintf("%s/register?invite=%s", base, url.QueryEscape(tokenPlain))
		}
		out = append(out, map[string]any{
			"id":          id,
			"invite_link": link,
			"created_by":  createdBy,
			"used_by":     usedBy,
			"expires_at":  expiresAt,
			"used_at":     usedAt,
			"created_at":  createdAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"invites": out})
}

func (s *Server) adminDeleteInvite(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(r, "inviteID")), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid invite id", http.StatusBadRequest)
		return
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM registration_invites WHERE id=$1`, id); err != nil {
		http.Error(w, "delete invite failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) adminListUsers(w http.ResponseWriter, r *http.Request) {
	baseURL, realm, err := keycloakBaseAndRealm(s.cfg.OIDCIssuerURL)
	if err != nil {
		http.Error(w, "oidc issuer configuration invalid", http.StatusInternalServerError)
		return
	}
	adminToken, err := s.keycloakAdminToken(r.Context(), baseURL)
	if err != nil {
		http.Error(w, "user backend unavailable", http.StatusBadGateway)
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	users, err := s.keycloakListUsers(r.Context(), baseURL, realm, adminToken, search, 200)
	if err != nil {
		http.Error(w, "list users failed", http.StatusBadGateway)
		return
	}
	for _, u := range users {
		id, _ := u["id"].(string)
		if strings.TrimSpace(id) == "" {
			continue
		}
		creatorBadge, _ := s.userCreatorBadge(r.Context(), id)
		u["creator_badge"] = creatorBadge
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (s *Server) adminListRoles(w http.ResponseWriter, r *http.Request) {
	baseURL, realm, err := keycloakBaseAndRealm(s.cfg.OIDCIssuerURL)
	if err != nil {
		http.Error(w, "oidc issuer configuration invalid", http.StatusInternalServerError)
		return
	}
	adminToken, err := s.keycloakAdminToken(r.Context(), baseURL)
	if err != nil {
		http.Error(w, "roles backend unavailable", http.StatusBadGateway)
		return
	}
	roles, err := s.keycloakListRealmRoles(r.Context(), baseURL, realm, adminToken)
	if err != nil {
		http.Error(w, "list roles failed", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": roles})
}

func (s *Server) adminUpdateUser(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	userID := strings.TrimSpace(chi.URLParam(r, "userID"))
	if userID == "" {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}
	var req struct {
		Enabled      *bool    `json:"enabled"`
		CreatorBadge *bool    `json:"creator_badge"`
		AddRoles     []string `json:"add_roles"`
		RemoveRoles  []string `json:"remove_roles"`
		SetRoles     []string `json:"set_roles"`
		TopRole      string   `json:"top_role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	baseURL, realm, err := keycloakBaseAndRealm(s.cfg.OIDCIssuerURL)
	if err != nil {
		http.Error(w, "oidc issuer configuration invalid", http.StatusInternalServerError)
		return
	}
	adminToken, err := s.keycloakAdminToken(r.Context(), baseURL)
	if err != nil {
		http.Error(w, "user backend unavailable", http.StatusBadGateway)
		return
	}

	if req.Enabled != nil {
		if err := s.keycloakSetUserEnabled(r.Context(), baseURL, realm, adminToken, userID, *req.Enabled); err != nil {
			http.Error(w, "set user status failed", http.StatusBadGateway)
			return
		}
	}
	if req.CreatorBadge != nil {
		if err := s.setUserCreatorBadge(r.Context(), userID, *req.CreatorBadge); err != nil {
			http.Error(w, "set creator badge failed", http.StatusInternalServerError)
			return
		}
	}
	if strings.TrimSpace(req.TopRole) != "" {
		top := strings.ToLower(strings.TrimSpace(req.TopRole))
		targetManaged, ok := managedRolesForTop(top)
		if !ok {
			http.Error(w, "invalid top_role", http.StatusBadRequest)
			return
		}
		currentRoles, err := s.keycloakUserRealmRoles(r.Context(), baseURL, realm, adminToken, userID)
		if err != nil {
			http.Error(w, "load current roles failed", http.StatusBadGateway)
			return
		}
		currentSet := map[string]string{}
		for _, role := range currentRoles {
			name := strings.TrimSpace(strings.ToLower(role))
			if name != "" {
				currentSet[name] = strings.TrimSpace(role)
			}
		}
		targetSet := map[string]bool{}
		for _, role := range targetManaged {
			targetSet[role] = true
		}
		for name := range currentSet {
			if !isManagedHierarchyRole(name) {
				targetSet[name] = true
			}
		}
		for role := range targetSet {
			if _, exists := currentSet[role]; !exists {
				if err := s.keycloakAssignRealmRole(r.Context(), baseURL, realm, adminToken, userID, role); err != nil {
					http.Error(w, "top role add failed", http.StatusBadGateway)
					return
				}
			}
		}
		for role, original := range currentSet {
			if !targetSet[role] {
				if err := s.keycloakRemoveRealmRole(r.Context(), baseURL, realm, adminToken, userID, original); err != nil {
					http.Error(w, "top role remove failed", http.StatusBadGateway)
					return
				}
			}
		}
	}
	if req.SetRoles != nil {
		currentRoles, err := s.keycloakUserRealmRoles(r.Context(), baseURL, realm, adminToken, userID)
		if err != nil {
			http.Error(w, "load current roles failed", http.StatusBadGateway)
			return
		}
		currentSet := map[string]bool{}
		for _, role := range currentRoles {
			role = strings.TrimSpace(strings.ToLower(role))
			if role != "" {
				currentSet[role] = true
			}
		}
		targetSet := map[string]bool{}
		for _, role := range req.SetRoles {
			role = strings.TrimSpace(strings.ToLower(role))
			if role != "" {
				targetSet[role] = true
			}
		}
		for role := range targetSet {
			if !currentSet[role] {
				if err := s.keycloakAssignRealmRole(r.Context(), baseURL, realm, adminToken, userID, role); err != nil {
					http.Error(w, "set roles add failed", http.StatusBadGateway)
					return
				}
			}
		}
		for role := range currentSet {
			if !targetSet[role] {
				if err := s.keycloakRemoveRealmRole(r.Context(), baseURL, realm, adminToken, userID, role); err != nil {
					http.Error(w, "set roles remove failed", http.StatusBadGateway)
					return
				}
			}
		}
	}
	for _, role := range req.AddRoles {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		if err := s.keycloakAssignRealmRole(r.Context(), baseURL, realm, adminToken, userID, role); err != nil {
			http.Error(w, "add role failed", http.StatusBadGateway)
			return
		}
	}
	for _, role := range req.RemoveRoles {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		if err := s.keycloakRemoveRealmRole(r.Context(), baseURL, realm, adminToken, userID, role); err != nil {
			http.Error(w, "remove role failed", http.StatusBadGateway)
			return
		}
	}
	_ = s.writeAudit(r.Context(), claims.Subject, "admin.update_user", "user", userID, map[string]any{
		"enabled":       req.Enabled,
		"add_roles":     req.AddRoles,
		"remove_roles":  req.RemoveRoles,
		"set_roles":     req.SetRoles,
		"top_role":      req.TopRole,
		"creator_badge": req.CreatorBadge,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) adminDeleteUser(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	userID := strings.TrimSpace(chi.URLParam(r, "userID"))
	if userID == "" {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}
	baseURL, realm, err := keycloakBaseAndRealm(s.cfg.OIDCIssuerURL)
	if err != nil {
		http.Error(w, "oidc issuer configuration invalid", http.StatusInternalServerError)
		return
	}
	adminToken, err := s.keycloakAdminToken(r.Context(), baseURL)
	if err != nil {
		http.Error(w, "user backend unavailable", http.StatusBadGateway)
		return
	}
	if err := s.keycloakDeleteUser(r.Context(), baseURL, realm, adminToken, userID); err != nil {
		http.Error(w, "delete user failed", http.StatusBadGateway)
		return
	}
	if err := s.reassignDeletedUserReferences(r.Context(), userID, claims.Subject); err != nil {
		http.Error(w, "local cleanup failed", http.StatusInternalServerError)
		return
	}
	_ = s.writeAudit(r.Context(), claims.Subject, "admin.delete_user", "user", userID, map[string]any{
		"dummy_sub": deletedUserDummySub(userID),
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func deletedUserDummySub(originalSub string) string {
	return "deleted:" + strings.TrimSpace(originalSub)
}

func (s *Server) reassignDeletedUserReferences(ctx context.Context, originalSub, deletedBy string) error {
	originalSub = strings.TrimSpace(originalSub)
	if originalSub == "" {
		return fmt.Errorf("empty original sub")
	}
	dummySub := deletedUserDummySub(originalSub)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO deleted_user_refs(original_sub, dummy_sub, deleted_by, deleted_at, restored_to_sub, restored_at)
		VALUES($1, $2, $3, now(), '', NULL)
		ON CONFLICT (original_sub) DO UPDATE
		SET dummy_sub=EXCLUDED.dummy_sub,
		    deleted_by=EXCLUDED.deleted_by,
		    deleted_at=now(),
		    restored_to_sub='',
		    restored_at=NULL
	`, originalSub, dummySub, strings.TrimSpace(deletedBy)); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO user_profiles(user_sub, display_name, bio, avatar_path, creator_badge, created_at, updated_at)
		VALUES($1, 'Deleted User', $2, '', false, now(), now())
		ON CONFLICT (user_sub) DO UPDATE
		SET display_name='Deleted User',
		    bio=EXCLUDED.bio,
		    creator_badge=false,
		    updated_at=now()
	`, dummySub, "Former account. Original subject: "+originalSub); err != nil {
		return err
	}

	updates := []string{
		`UPDATE tracks SET owner_sub=$2 WHERE owner_sub=$1`,
		`UPDATE albums SET owner_sub=$2 WHERE owner_sub=$1`,
		`UPDATE playlists SET owner_sub=$2 WHERE owner_sub=$1`,
		`UPDATE comments SET author_sub=$2 WHERE author_sub=$1`,
		`UPDATE album_comments SET author_sub=$2 WHERE author_sub=$1`,
		`UPDATE ratings SET author_sub=$2 WHERE author_sub=$1`,
		`UPDATE user_profile_comments SET author_sub=$2 WHERE author_sub=$1`,
		`UPDATE user_profile_comments SET target_sub=$2 WHERE target_sub=$1`,
		`UPDATE play_events SET user_sub=$2 WHERE user_sub=$1`,
		`UPDATE stream_play_tokens SET user_sub=$2 WHERE user_sub=$1`,
	}
	for _, q := range updates {
		if _, err := tx.Exec(ctx, q, originalSub, dummySub); err != nil {
			return err
		}
	}

	// Keep follower graph consistent and conflict-free after subject rewrite.
	if _, err := tx.Exec(ctx, `DELETE FROM follows WHERE follower_sub=$1 OR followed_sub=$1`, originalSub); err != nil {
		return err
	}

	// Remove stale profile row of deleted account.
	if _, err := tx.Exec(ctx, `DELETE FROM user_profiles WHERE user_sub=$1`, originalSub); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *Server) adminSystemOverview(w http.ResponseWriter, r *http.Request) {
	monitoringCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	keycloak := s.probeKeycloak(monitoringCtx)

	userCount := int64(0)
	baseURL, realm, err := keycloakBaseAndRealm(s.cfg.OIDCIssuerURL)
	if err != nil {
		keycloak["admin_error"] = "oidc issuer configuration invalid"
	} else {
		adminToken, adminErr := s.keycloakAdminToken(r.Context(), baseURL)
		if adminErr != nil {
			keycloak["admin_error"] = "admin token unavailable"
		} else {
			keycloak["admin_api"] = true
			userCount, _ = s.keycloakCountUsers(r.Context(), baseURL, realm, adminToken)
		}
	}
	regEnabled, _ := s.isRegistrationEnabled(r.Context())

	stats := map[string]int64{}
	for k, q := range map[string]string{
		"tracks":    "SELECT COUNT(*) FROM tracks",
		"albums":    "SELECT COUNT(*) FROM albums",
		"playlists": "SELECT COUNT(*) FROM playlists",
		"jobs":      "SELECT COUNT(*) FROM transcode_jobs",
	} {
		var v int64
		if err := s.db.QueryRow(r.Context(), q).Scan(&v); err == nil {
			stats[k] = v
		}
	}
	var playsTotal int64
	var uniqueListeners int64
	var guestPlays int64
	_ = s.db.QueryRow(r.Context(), `
		SELECT
			COUNT(*)::bigint,
			COUNT(DISTINCT NULLIF(user_sub,''))::bigint,
			COUNT(*) FILTER (WHERE COALESCE(user_sub,'')='')::bigint
		FROM play_events
	`).Scan(&playsTotal, &uniqueListeners, &guestPlays)

	queued := int64(0)
	processing := int64(0)
	failed := int64(0)
	done := int64(0)
	_ = s.db.QueryRow(r.Context(), `
		SELECT
			COUNT(*) FILTER (WHERE status='queued'),
			COUNT(*) FILTER (WHERE status='processing'),
			COUNT(*) FILTER (WHERE status='failed'),
			COUNT(*) FILTER (WHERE status='done')
		FROM transcode_jobs
	`).Scan(&queued, &processing, &failed, &done)

	valkey := s.probeValkey(monitoringCtx)
	prom := s.probePrometheus(monitoringCtx)
	grafana := s.probeGrafana(monitoringCtx)
	promPublic, grafanaPublic := s.publicMonitoringLinks(r)
	if promPublic != "" {
		prom["public_url"] = promPublic
	}
	if grafanaPublic != "" {
		grafana["public_url"] = grafanaPublic
	}
	topTracks := make([]map[string]any, 0, 10)
	trackRows, err := s.db.Query(r.Context(), `
		SELECT
			p.track_id::text,
			COALESCE(t.title, p.track_id::text) AS title,
			COALESCE(ar.name, '') AS artist,
			COUNT(*)::bigint AS plays
		FROM play_events p
		LEFT JOIN tracks t ON t.id = p.track_id
		LEFT JOIN artists ar ON ar.id = t.artist_id
		GROUP BY p.track_id, t.title, ar.name
		ORDER BY plays DESC, title ASC
		LIMIT 10
	`)
	if err == nil {
		defer trackRows.Close()
		for trackRows.Next() {
			var trackID, title, artist string
			var plays int64
			if err := trackRows.Scan(&trackID, &title, &artist, &plays); err != nil {
				continue
			}
			topTracks = append(topTracks, map[string]any{
				"track_id": trackID,
				"title":    title,
				"artist":   artist,
				"plays":    plays,
			})
		}
	}
	topAlbums := make([]map[string]any, 0, 10)
	albumRows, err := s.db.Query(r.Context(), `
		SELECT
			p.album_id,
			COALESCE(al.title, 'Unknown album') AS title,
			COALESCE(ar.name, '') AS artist,
			COUNT(*)::bigint AS plays
		FROM play_events p
		LEFT JOIN albums al ON al.id = p.album_id
		LEFT JOIN artists ar ON ar.id = al.artist_id
		WHERE p.album_id IS NOT NULL
		GROUP BY p.album_id, al.title, ar.name
		ORDER BY plays DESC, title ASC
		LIMIT 10
	`)
	if err == nil {
		defer albumRows.Close()
		for albumRows.Next() {
			var albumID int64
			var title, artist string
			var plays int64
			if err := albumRows.Scan(&albumID, &title, &artist, &plays); err != nil {
				continue
			}
			topAlbums = append(topAlbums, map[string]any{
				"album_id": albumID,
				"title":    title,
				"artist":   artist,
				"plays":    plays,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"users":                userCount,
		"registration_enabled": regEnabled,
		"stats":                stats,
		"jobs_by_status": map[string]int64{
			"queued":     queued,
			"processing": processing,
			"failed":     failed,
			"done":       done,
		},
		"monitoring": map[string]any{
			"valkey":     valkey,
			"prometheus": prom,
			"grafana":    grafana,
			"keycloak":   keycloak,
		},
		"playback": map[string]any{
			"plays_total":      playsTotal,
			"unique_listeners": uniqueListeners,
			"guest_plays":      guestPlays,
			"top_tracks":       topTracks,
			"top_albums":       topAlbums,
		},
		"server_time": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) adminCreateProxySession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	target := strings.ToLower(strings.TrimSpace(req.Target))
	var path string
	switch target {
	case "grafana":
		path = "/grafana/"
	case "prometheus":
		path = "/prometheus/"
	default:
		http.Error(w, "invalid target", http.StatusBadRequest)
		return
	}
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if h == "" || !strings.HasPrefix(strings.ToLower(h), "bearer ") {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}
	tok := strings.TrimSpace(h[len("Bearer "):])
	if tok == "" {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     proxyCookieName(strings.TrimRight(path, "/")),
		Value:    tok,
		Path:     path,
		HttpOnly: true,
		Secure:   requestIsSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   8 * 60 * 60,
	})
	writeJSON(w, http.StatusOK, map[string]string{"url": path})
}

func (s *Server) probeValkey(ctx context.Context) map[string]any {
	out := map[string]any{
		"addr":      s.cfg.RedisAddr,
		"reachable": false,
	}
	if strings.TrimSpace(s.cfg.RedisAddr) == "" {
		out["error"] = "redis addr not configured"
		return out
	}
	start := time.Now()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", s.cfg.RedisAddr)
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	out["latency_ms"] = time.Since(start).Milliseconds()
	reader := bufio.NewReader(conn)

	if strings.TrimSpace(s.cfg.RedisPassword) != "" {
		authReply, err := redisRoundTrip(reader, conn, "AUTH", s.cfg.RedisPassword)
		if err != nil {
			out["error"] = "auth failed"
			return out
		}
		if strings.HasPrefix(authReply, "-") {
			out["error"] = strings.TrimPrefix(authReply, "-")
			return out
		}
	}

	infoReply, err := redisRoundTrip(reader, conn, "INFO")
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	info, ok := parseRedisInfo(infoReply)
	if !ok {
		out["error"] = "unexpected INFO response"
		return out
	}
	out["reachable"] = true
	if v := info["redis_version"]; v != "" {
		out["version"] = v
	}
	if v := info["uptime_in_seconds"]; v != "" {
		out["uptime_seconds"] = v
	}
	if v := info["connected_clients"]; v != "" {
		out["connected_clients"] = v
	}
	if v := info["used_memory_human"]; v != "" {
		out["used_memory_human"] = v
	}
	if v := info["db0"]; v != "" {
		out["db0"] = v
	}
	return out
}

func (s *Server) probePrometheus(ctx context.Context) map[string]any {
	out := map[string]any{
		"url":       strings.TrimRight(s.cfg.PrometheusURL, "/"),
		"reachable": false,
	}
	base := strings.TrimRight(s.cfg.PrometheusURL, "/")
	if base == "" {
		out["error"] = "prometheus url not configured"
		return out
	}
	client := &http.Client{Timeout: 2 * time.Second}
	healthPaths := []string{"/-/healthy", "/prometheus/-/healthy"}
	queryPaths := []string{"/api/v1/query?query=up", "/prometheus/api/v1/query?query=up"}
	if strings.Contains(base, "/prometheus") {
		healthPaths = []string{"/-/healthy"}
		queryPaths = []string{"/api/v1/query?query=up"}
	}
	healthy := false
	for _, hp := range healthPaths {
		start := time.Now()
		healthReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+hp, nil)
		res, err := client.Do(healthReq)
		if err != nil {
			continue
		}
		out["latency_ms"] = time.Since(start).Milliseconds()
		out["status_code"] = res.StatusCode
		_ = res.Body.Close()
		if res.StatusCode >= 200 && res.StatusCode <= 299 {
			healthy = true
			break
		}
	}
	if !healthy {
		out["error"] = "health endpoint returned non-2xx"
		return out
	}
	out["reachable"] = true

	var queryRes *http.Response
	var err error
	for _, qp := range queryPaths {
		queryReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+qp, nil)
		queryRes, err = client.Do(queryReq)
		if err == nil && queryRes.StatusCode >= 200 && queryRes.StatusCode <= 299 {
			break
		}
		if queryRes != nil {
			_ = queryRes.Body.Close()
		}
		queryRes = nil
	}
	if queryRes == nil {
		out["query_error"] = "query endpoint unavailable"
		return out
	}
	defer queryRes.Body.Close()
	var payload struct {
		Status string `json:"status"`
		Data   struct {
			Result []any `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(queryRes.Body).Decode(&payload); err == nil && payload.Status == "success" {
		total := int64(len(payload.Data.Result))
		out["targets_total"] = total
		out["targets_up"] = total
	}
	return out
}

func (s *Server) probeGrafana(ctx context.Context) map[string]any {
	out := map[string]any{
		"url":       strings.TrimRight(s.cfg.GrafanaURL, "/"),
		"reachable": false,
	}
	base := strings.TrimRight(s.cfg.GrafanaURL, "/")
	if base == "" {
		out["error"] = "grafana url not configured"
		return out
	}
	client := &http.Client{Timeout: 2 * time.Second}
	healthPaths := []string{"/api/health", "/grafana/api/health"}
	if strings.Contains(base, "/grafana") {
		healthPaths = []string{"/api/health"}
	}
	var res *http.Response
	healthy := false
	for _, hp := range healthPaths {
		start := time.Now()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+hp, nil)
		rres, err := client.Do(req)
		if err != nil {
			continue
		}
		out["latency_ms"] = time.Since(start).Milliseconds()
		out["status_code"] = rres.StatusCode
		if rres.StatusCode >= 200 && rres.StatusCode <= 299 {
			res = rres
			healthy = true
			break
		}
		_ = rres.Body.Close()
	}
	if !healthy || res == nil {
		out["error"] = "health endpoint returned non-2xx"
		return out
	}
	defer res.Body.Close()
	out["reachable"] = true
	var health map[string]any
	if err := json.NewDecoder(res.Body).Decode(&health); err == nil {
		if version, ok := health["version"]; ok {
			out["version"] = version
		}
		if database, ok := health["database"]; ok {
			out["database"] = database
		}
		if commit, ok := health["commit"]; ok {
			out["commit"] = commit
		}
	}
	return out
}

func (s *Server) probeKeycloak(ctx context.Context) map[string]any {
	issuer := strings.TrimSpace(s.cfg.OIDCIssuerURL)
	issuer = strings.TrimRight(issuer, "/")
	out := map[string]any{
		"issuer":     issuer,
		"public_url": issuer,
		"reachable":  false,
	}
	if issuer == "" {
		out["error"] = "oidc issuer not configured"
		return out
	}
	wellKnownURL := issuer + "/.well-known/openid-configuration"
	start := time.Now()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, wellKnownURL, nil)
	res, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	defer res.Body.Close()
	out["status_code"] = res.StatusCode
	out["latency_ms"] = time.Since(start).Milliseconds()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		out["error"] = "well-known endpoint returned non-2xx"
		return out
	}
	out["reachable"] = true
	var payload struct {
		Issuer string `json:"issuer"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2*1024*1024)).Decode(&payload); err == nil {
		if strings.TrimSpace(payload.Issuer) != "" {
			out["discovered_issuer"] = strings.TrimSpace(payload.Issuer)
		}
	}
	return out
}

func (s *Server) publicMonitoringLinks(r *http.Request) (string, string) {
	if strings.TrimSpace(s.cfg.PrometheusPublic) != "" {
		return strings.TrimSpace(s.cfg.PrometheusPublic), firstNonEmpty(strings.TrimSpace(s.cfg.GrafanaPublic), "/grafana/")
	}
	if strings.TrimSpace(s.cfg.GrafanaPublic) != "" {
		return firstNonEmpty(strings.TrimSpace(s.cfg.PrometheusPublic), "/prometheus/graph?g0.expr=up&g0.tab=0"), strings.TrimSpace(s.cfg.GrafanaPublic)
	}
	return "/prometheus/graph?g0.expr=up&g0.tab=0", "/grafana/"
}

func firstNonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func redisRoundTrip(reader *bufio.Reader, conn net.Conn, cmd ...string) (string, error) {
	_, err := conn.Write(redisCommand(cmd...))
	if err != nil {
		return "", err
	}
	return redisReadReply(reader)
}

func redisCommand(args ...string) []byte {
	var b strings.Builder
	b.WriteString("*")
	b.WriteString(strconv.Itoa(len(args)))
	b.WriteString("\r\n")
	for _, arg := range args {
		b.WriteString("$")
		b.WriteString(strconv.Itoa(len(arg)))
		b.WriteString("\r\n")
		b.WriteString(arg)
		b.WriteString("\r\n")
	}
	return []byte(b.String())
}

func redisReadReply(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	if len(line) == 0 {
		return "", io.EOF
	}
	switch line[0] {
	case '+', '-':
		return strings.TrimRight(line, "\r\n"), nil
	case '$':
		sizeStr := strings.TrimSpace(line[1:])
		n, err := strconv.Atoi(sizeStr)
		if err != nil || n < 0 {
			return "", fmt.Errorf("invalid bulk size")
		}
		data := make([]byte, n+2)
		if _, err := io.ReadFull(reader, data); err != nil {
			return "", err
		}
		return string(data[:n]), nil
	default:
		return strings.TrimRight(line, "\r\n"), nil
	}
}

func parseRedisInfo(payload string) (map[string]string, bool) {
	lines := strings.Split(payload, "\n")
	if len(lines) == 0 {
		return nil, false
	}
	out := map[string]string{}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		out[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return out, len(out) > 0
}

func (s *Server) userProfileBySub(ctx context.Context, userSub string) (string, string, string, error) {
	var displayName, bio, avatarPath string
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(display_name,''), COALESCE(bio,''), COALESCE(avatar_path,'')
		FROM user_profiles
		WHERE user_sub=$1
	`, userSub).Scan(&displayName, &bio, &avatarPath)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", "", nil
		}
		return "", "", "", err
	}
	return displayName, bio, avatarPath, nil
}

func (s *Server) upsertUserProfile(ctx context.Context, userSub, displayName, bio string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO user_profiles(user_sub, display_name, bio, updated_at)
		VALUES($1, $2, $3, now())
		ON CONFLICT (user_sub) DO UPDATE
		SET display_name=EXCLUDED.display_name,
		    bio=EXCLUDED.bio,
		    updated_at=now()
	`, userSub, displayName, bio)
	return err
}

func (s *Server) userHasSubsonicPassword(ctx context.Context, userSub string) (bool, error) {
	var has bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM user_profiles
			WHERE user_sub=$1 AND COALESCE(subsonic_password,'') <> ''
		)
	`, userSub).Scan(&has)
	return has, err
}

func (s *Server) userSubsonicPasswordBySub(ctx context.Context, userSub string) (string, error) {
	var stored string
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(subsonic_password,'')
		FROM user_profiles
		WHERE user_sub=$1
	`, userSub).Scan(&stored)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return "", nil
	}
	plain, legacy, err := s.decryptSubsonicPassword(stored)
	if err != nil {
		return "", err
	}
	// Auto-migrate legacy plaintext rows on first read.
	if legacy && plain != "" {
		_ = s.setUserSubsonicPassword(ctx, userSub, plain)
	}
	return plain, nil
}

func (s *Server) setUserSubsonicPassword(ctx context.Context, userSub, password string) error {
	enc, err := s.encryptSubsonicPassword(password)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO user_profiles(user_sub, subsonic_password, updated_at)
		VALUES($1, $2, now())
		ON CONFLICT (user_sub) DO UPDATE
		SET subsonic_password=EXCLUDED.subsonic_password,
		    updated_at=now()
	`, userSub, enc)
	return err
}

func (s *Server) clearUserSubsonicPassword(ctx context.Context, userSub string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO user_profiles(user_sub, subsonic_password, updated_at)
		VALUES($1, '', now())
		ON CONFLICT (user_sub) DO UPDATE
		SET subsonic_password='',
		    updated_at=now()
	`, userSub)
	return err
}

func (s *Server) subsonicKey() [32]byte {
	base := strings.TrimSpace(s.cfg.SubsonicSecretKey)
	if base == "" {
		base = strings.TrimSpace(s.cfg.SigningKey)
	}
	return sha256.Sum256([]byte(base))
}

func (s *Server) encryptSubsonicPassword(plain string) (string, error) {
	key := s.subsonicKey()
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nil, nonce, []byte(plain), nil)
	out := append(nonce, ct...)
	return "enc:v1:" + base64.StdEncoding.EncodeToString(out), nil
}

func (s *Server) decryptSubsonicPassword(stored string) (plain string, legacy bool, err error) {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return "", false, nil
	}
	const prefix = "enc:v1:"
	if !strings.HasPrefix(stored, prefix) {
		return stored, true, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
	if err != nil {
		return "", false, err
	}
	key := s.subsonicKey()
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", false, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", false, err
	}
	if len(raw) < gcm.NonceSize() {
		return "", false, errors.New("invalid encrypted subsonic password")
	}
	nonce := raw[:gcm.NonceSize()]
	ct := raw[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", false, err
	}
	return string(pt), false, nil
}

func (s *Server) seedUserDisplayName(ctx context.Context, userSub, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO user_profiles(user_sub, display_name, updated_at)
		VALUES($1, $2, now())
		ON CONFLICT (user_sub) DO UPDATE
		SET display_name = CASE
			WHEN COALESCE(user_profiles.display_name,'') = '' THEN EXCLUDED.display_name
			ELSE user_profiles.display_name
		END,
		updated_at=now()
	`, userSub, username)
	return err
}

func (s *Server) saveUserAvatarFromRequest(ctx context.Context, r *http.Request, userSub string) (string, error) {
	if err := r.ParseMultipartForm(10 * 1024 * 1024); err != nil {
		return "", fmt.Errorf("invalid multipart payload")
	}
	f, h, err := r.FormFile("avatar")
	if err != nil {
		return "", fmt.Errorf("missing avatar file")
	}
	defer f.Close()

	ext := strings.ToLower(filepath.Ext(h.Filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
	default:
		return "", fmt.Errorf("avatar must be jpg/png/webp")
	}
	target := s.store.UserAvatarPathWithExt(userSub, strings.TrimPrefix(ext, "."))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(s.store.TempDir(), "avatar-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, io.LimitReader(f, 10*1024*1024)); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		return "", err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO user_profiles(user_sub, avatar_path, updated_at)
		VALUES($1, $2, now())
		ON CONFLICT (user_sub) DO UPDATE
		SET avatar_path=EXCLUDED.avatar_path, updated_at=now()
	`, userSub, target)
	if err != nil {
		return "", err
	}
	return target, nil
}

func (s *Server) verifyUserCredentials(ctx context.Context, username, password string) (bool, error) {
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", s.cfg.OIDCClientID)
	if scopes := strings.TrimSpace(s.cfg.OIDCScopes); scopes != "" {
		form.Set("scope", scopes)
	}
	if strings.TrimSpace(s.cfg.OIDCClientSecret) != "" {
		form.Set("client_secret", s.cfg.OIDCClientSecret)
	}
	form.Set("username", username)
	form.Set("password", password)
	tokenURL := strings.TrimRight(s.cfg.OIDCIssuerURL, "/") + "/protocol/openid-connect/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusOK {
		return true, nil
	}
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusBadRequest {
		return false, nil
	}
	return false, fmt.Errorf("credentials check status: %d", res.StatusCode)
}

func (s *Server) resetKeycloakPasswordByUsername(ctx context.Context, username, newPassword string) error {
	baseURL, realm, err := keycloakBaseAndRealm(s.cfg.OIDCIssuerURL)
	if err != nil {
		return err
	}
	adminToken, err := s.keycloakAdminToken(ctx, baseURL)
	if err != nil {
		return err
	}
	userID, err := s.keycloakFindUserID(ctx, baseURL, realm, adminToken, username)
	if err != nil {
		return err
	}
	return s.keycloakSetPassword(ctx, baseURL, realm, adminToken, userID, newPassword)
}

func (s *Server) updateKeycloakEmailByUsername(ctx context.Context, username, email string) error {
	baseURL, realm, err := keycloakBaseAndRealm(s.cfg.OIDCIssuerURL)
	if err != nil {
		return err
	}
	adminToken, err := s.keycloakAdminToken(ctx, baseURL)
	if err != nil {
		return err
	}
	userID, err := s.keycloakFindUserID(ctx, baseURL, realm, adminToken, username)
	if err != nil {
		return err
	}
	userURL := fmt.Sprintf("%s/admin/realms/%s/users/%s",
		strings.TrimRight(baseURL, "/"), url.PathEscape(realm), url.PathEscape(userID))
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, userURL, nil)
	if err != nil {
		return err
	}
	getReq.Header.Set("Authorization", "Bearer "+adminToken)
	getRes, err := http.DefaultClient.Do(getReq)
	if err != nil {
		return err
	}
	defer getRes.Body.Close()
	if getRes.StatusCode != http.StatusOK {
		return fmt.Errorf("load user for email update status: %d", getRes.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(getRes.Body, 2*1024*1024)).Decode(&payload); err != nil {
		return err
	}
	payload["email"] = email
	payload["emailVerified"] = false
	body, _ := json.Marshal(payload)
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, userURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	putReq.Header.Set("Authorization", "Bearer "+adminToken)
	putReq.Header.Set("Content-Type", "application/json")
	putRes, err := http.DefaultClient.Do(putReq)
	if err != nil {
		return err
	}
	defer putRes.Body.Close()
	if putRes.StatusCode != http.StatusNoContent {
		return fmt.Errorf("update user email status: %d", putRes.StatusCode)
	}
	return nil
}

func (s *Server) keycloakEmailByUsername(ctx context.Context, username string) (string, error) {
	baseURL, realm, err := keycloakBaseAndRealm(s.cfg.OIDCIssuerURL)
	if err != nil {
		return "", err
	}
	adminToken, err := s.keycloakAdminToken(ctx, baseURL)
	if err != nil {
		return "", err
	}
	userID, err := s.keycloakFindUserID(ctx, baseURL, realm, adminToken, username)
	if err != nil {
		return "", err
	}
	userURL := fmt.Sprintf("%s/admin/realms/%s/users/%s",
		strings.TrimRight(baseURL, "/"), url.PathEscape(realm), url.PathEscape(userID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("load user email status: %d", res.StatusCode)
	}
	var payload struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2*1024*1024)).Decode(&payload); err != nil {
		return "", err
	}
	return strings.TrimSpace(payload.Email), nil
}

func (s *Server) keycloakUsernameBySub(ctx context.Context, userSub string) (string, error) {
	baseURL, realm, err := keycloakBaseAndRealm(s.cfg.OIDCIssuerURL)
	if err != nil {
		return "", err
	}
	adminToken, err := s.keycloakAdminToken(ctx, baseURL)
	if err != nil {
		return "", err
	}
	userURL := fmt.Sprintf("%s/admin/realms/%s/users/%s",
		strings.TrimRight(baseURL, "/"), url.PathEscape(realm), url.PathEscape(strings.TrimSpace(userSub)))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("load user by sub status: %d", res.StatusCode)
	}
	var payload struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2*1024*1024)).Decode(&payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.Username) == "" {
		return "", fmt.Errorf("username missing for subject")
	}
	return strings.TrimSpace(payload.Username), nil
}

func (s *Server) resolveUsername(claims auth.Claims, ctx context.Context) (string, error) {
	if strings.TrimSpace(claims.Username) != "" {
		return strings.TrimSpace(claims.Username), nil
	}
	return s.keycloakUsernameBySub(ctx, claims.Subject)
}

func (s *Server) resolveStreamPath(ctx context.Context, trackID, format string) (string, string, error) {
	resolveOriginal := func() (string, string, error) {
		var path, codec string
		err := s.db.QueryRow(ctx, `
			SELECT file_path, codec FROM track_files
			WHERE track_id = $1 AND is_original = true
			ORDER BY created_at ASC LIMIT 1
		`, trackID).Scan(&path, &codec)
		if err != nil {
			return "", "", err
		}
		return path, codecToMime(codec), nil
	}

	switch format {
	case "original":
		return resolveOriginal()
	case "mp3", "opus", "aac", "m4a", "mobile":
		name := map[string]string{
			"mp3":    "320.mp3",
			"opus":   "opus160.opus",
			"aac":    "mobile.m4a",
			"m4a":    "mobile.m4a",
			"mobile": "mobile.m4a",
		}[format]
		path := filepath.Join(s.store.DerivedTrackDir(trackID), name)
		if _, err := os.Stat(path); err == nil {
			if format == "mp3" {
				return path, "audio/mpeg", nil
			}
			if format == "aac" || format == "m4a" || format == "mobile" {
				return path, "audio/mp4", nil
			}
			return path, "audio/ogg", nil
		}
		// Fallback to original source if derived file does not exist yet.
		return resolveOriginal()
	default:
		// Unknown format requests are served as original for client compatibility.
		return resolveOriginal()
	}
}

func (s *Server) writeTempAndHash(file multipart.File, _ *multipart.FileHeader) (string, string, int64, error) {
	tempFile, err := os.CreateTemp(s.store.TempDir(), "upload-*.tmp")
	if err != nil {
		return "", "", 0, err
	}
	defer tempFile.Close()

	h := blake3.New()
	mw := io.MultiWriter(tempFile, h)
	n, err := io.Copy(mw, file)
	if err != nil {
		return "", "", 0, err
	}
	hashHex := hex.EncodeToString(h.Sum(nil))
	return tempFile.Name(), hashHex, n, nil
}

func upsertArtist(ctx context.Context, tx pgx.Tx, name string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO artists(name) VALUES($1)
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, name).Scan(&id)
	return id, err
}

func upsertAlbum(ctx context.Context, tx pgx.Tx, artistID int64, title, visibility, ownerSub, genre string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO albums(artist_id, title, visibility, owner_sub, genre) VALUES($1, $2, $3, $4, $5)
		ON CONFLICT (artist_id, title) DO UPDATE
		SET visibility = CASE
			WHEN albums.visibility = 'public' OR EXCLUDED.visibility = 'public' THEN 'public'
			ELSE albums.visibility
		END,
		genre = CASE
			WHEN COALESCE(albums.genre,'') = '' THEN EXCLUDED.genre
			ELSE albums.genre
		END
		RETURNING id
	`, artistID, title, visibility, ownerSub, strings.TrimSpace(genre)).Scan(&id)
	return id, err
}

func fallback(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return strings.TrimSpace(v)
}

func normalizeVisibility(v string) string {
	switch strings.TrimSpace(v) {
	case "public", "private", "followers_only", "unlisted":
		return strings.TrimSpace(v)
	default:
		return "private"
	}
}

func normalizePlaylistVisibility(v string) string {
	switch strings.TrimSpace(v) {
	case "public", "private":
		return strings.TrimSpace(v)
	default:
		return "private"
	}
}

func externalBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if xfProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); xfProto != "" {
		parts := strings.Split(xfProto, ",")
		if strings.TrimSpace(parts[0]) != "" {
			scheme = strings.TrimSpace(parts[0])
		}
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func inviteTokenHash(token string) string {
	sum := blake3.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func (s *Server) createRegistrationInvite(ctx context.Context, createdBy string, ttl time.Duration) (string, time.Time, error) {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", time.Time{}, err
	}
	token := hex.EncodeToString(buf)
	expires := time.Now().UTC().Add(ttl)
	_, err := s.db.Exec(ctx, `
		INSERT INTO registration_invites(token_hash, token_plain, created_by, expires_at)
		VALUES($1, $2, $3, $4)
	`, inviteTokenHash(token), token, createdBy, expires)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func (s *Server) registrationInviteUsable(ctx context.Context, token string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM registration_invites
			WHERE token_hash=$1
			  AND used_at IS NULL
			  AND expires_at > now()
		)
	`, inviteTokenHash(token)).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (s *Server) consumeRegistrationInvite(ctx context.Context, token, usedBy string) error {
	ct, err := s.db.Exec(ctx, `
		UPDATE registration_invites
		SET used_at=now(), used_by=$2
		WHERE token_hash=$1
		  AND used_at IS NULL
		  AND expires_at > now()
	`, inviteTokenHash(token), usedBy)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("invite not usable")
	}
	return nil
}

func managedRolesForTop(top string) ([]string, bool) {
	switch strings.ToLower(strings.TrimSpace(top)) {
	case "user":
		return []string{"user"}, true
	case "creator":
		return []string{"user", "creator"}, true
	case "moderator":
		return []string{"user", "creator", "moderator"}, true
	case "admin":
		return []string{"user", "creator", "moderator", "admin"}, true
	default:
		return nil, false
	}
}

func isManagedHierarchyRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user", "member", "creator", "moderator", "admin":
		return true
	default:
		return false
	}
}

func (s *Server) userCreatorBadge(ctx context.Context, userSub string) (bool, error) {
	var creator bool
	err := s.db.QueryRow(ctx, `SELECT COALESCE(creator_badge,false) FROM user_profiles WHERE user_sub=$1`, userSub).Scan(&creator)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return creator, nil
}

func (s *Server) setUserCreatorBadge(ctx context.Context, userSub string, enabled bool) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO user_profiles(user_sub, creator_badge, updated_at)
		VALUES($1, $2, now())
		ON CONFLICT (user_sub) DO UPDATE
		SET creator_badge=EXCLUDED.creator_badge, updated_at=now()
	`, userSub, enabled)
	return err
}

func (s *Server) canUserUpload(ctx context.Context, claims auth.Claims) (bool, error) {
	if auth.HasRole(claims, "admin") {
		return true, nil
	}
	return s.userCreatorBadge(ctx, claims.Subject)
}

func (s *Server) isRegistrationEnabled(ctx context.Context) (bool, error) {
	var raw string
	err := s.db.QueryRow(ctx, `
		SELECT value_text FROM app_settings WHERE key='registration_enabled'
	`).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return true, nil
		}
		return false, err
	}
	b, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return true, nil
	}
	return b, nil
}

func (s *Server) setRegistrationEnabled(ctx context.Context, enabled bool) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO app_settings(key, value_text, updated_at)
		VALUES('registration_enabled', $1, now())
		ON CONFLICT (key) DO UPDATE
		SET value_text=EXCLUDED.value_text, updated_at=now()
	`, strconv.FormatBool(enabled))
	return err
}

func (s *Server) isDebugLoggingEnabled(ctx context.Context) (bool, error) {
	var raw string
	err := s.db.QueryRow(ctx, `
		SELECT value_text FROM app_settings WHERE key='debug_logging_enabled'
	`).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	b, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, nil
	}
	return b, nil
}

func (s *Server) setDebugLoggingEnabled(ctx context.Context, enabled bool) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO app_settings(key, value_text, updated_at)
		VALUES('debug_logging_enabled', $1, now())
		ON CONFLICT (key) DO UPDATE
		SET value_text=EXCLUDED.value_text, updated_at=now()
	`, strconv.FormatBool(enabled))
	return err
}

func clientIPFromRequest(r *http.Request) string {
	xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if xff != "" {
		if idx := strings.Index(xff, ","); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return xff
	}
	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		return xrip
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func (s *Server) writeDebugLogEvent(ctx context.Context, eventType, endpoint, httpMethod string, statusCode int, actorSub, clientIP, userAgent string, details map[string]any) error {
	enabled, err := s.isDebugLoggingEnabled(ctx)
	if err != nil || !enabled {
		return err
	}
	payload := []byte("{}")
	if details != nil {
		b, err := json.Marshal(details)
		if err != nil {
			return err
		}
		payload = b
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO debug_log_events(event_type, endpoint, http_method, status_code, actor_sub, client_ip, user_agent, details)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb)
	`, strings.TrimSpace(eventType), strings.TrimSpace(endpoint), strings.TrimSpace(httpMethod), statusCode, strings.TrimSpace(actorSub), strings.TrimSpace(clientIP), strings.TrimSpace(userAgent), string(payload))
	s.maybeSweepDebugLogs(ctx)
	return err
}

func (s *Server) maybeSweepDebugLogs(ctx context.Context) {
	s.debugLogCleanupMu.Lock()
	defer s.debugLogCleanupMu.Unlock()
	// Keep cleanup lightweight: run at most once per minute.
	if !s.lastDebugLogSweep.IsZero() && time.Since(s.lastDebugLogSweep) < time.Minute {
		return
	}
	s.lastDebugLogSweep = time.Now()
	_, _ = s.db.Exec(ctx, `DELETE FROM debug_log_events WHERE created_at < now() - interval '7 days'`)
}

func isSupportedAudioFile(filename, contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if strings.HasPrefix(ct, "audio/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".mp3", ".wav", ".flac", ".m4a", ".aac", ".ogg", ".opus", ".aif", ".aiff", ".wma":
		return true
	default:
		return false
	}
}

func trimExt(v string) string {
	base := filepath.Base(v)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

func chooseImportedTitle(optsTitle, probeTitle, fileTitle string) string {
	if t := normalizeImportedText(optsTitle, ""); t != "" {
		return t
	}
	probe := normalizeImportedText(probeTitle, "")
	if probe != "" && !looksCorruptImportText(probe) {
		return probe
	}
	if t := normalizeImportedText(fileTitle, ""); t != "" {
		return t
	}
	if probe != "" {
		return probe
	}
	return "Untitled"
}

func looksCorruptImportText(v string) bool {
	return !utf8.ValidString(v) || strings.ContainsRune(v, '\uFFFD')
}

func normalizeImportedText(v, fallbackValue string) string {
	s := strings.TrimSpace(v)
	if s == "" {
		return fallbackValue
	}
	s = strings.ReplaceAll(s, "�s", "’s")
	s = strings.ReplaceAll(s, "�S", "’S")
	s = strings.ReplaceAll(s, "�m", "’m")
	s = strings.ReplaceAll(s, "�M", "’M")
	s = strings.ReplaceAll(s, " � ", " – ")
	s = strings.ReplaceAll(s, "�", "")
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return fallbackValue
	}
	return s
}

func codecToExt(codec string) string {
	switch strings.ToLower(codec) {
	case "flac":
		return "flac"
	case "mp3":
		return "mp3"
	case "opus":
		return "opus"
	case "aac":
		return "m4a"
	default:
		return ""
	}
}

func codecToMime(codec string) string {
	switch strings.ToLower(codec) {
	case "flac":
		return "audio/flac"
	case "mp3":
		return "audio/mpeg"
	case "opus":
		return "audio/ogg"
	case "aac":
		return "audio/mp4"
	default:
		return "application/octet-stream"
	}
}

func (s *Server) processDerived(trackID, source string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	dir := s.store.DerivedTrackDir(trackID)
	_ = media.TranscodeMP3(ctx, s.cfg.FFmpegBin, source, filepath.Join(dir, "320.mp3"))
	_ = media.TranscodeAAC(ctx, s.cfg.FFmpegBin, source, filepath.Join(dir, "mobile.m4a"))
	_ = media.TranscodeOpus(ctx, s.cfg.FFmpegBin, source, filepath.Join(dir, "opus160.opus"))
	_ = media.BuildWaveform(ctx, s.cfg.FFmpegBin, source, filepath.Join(dir, "waveform.json"))
}

func (s *Server) writeAudit(ctx context.Context, actorSub, action, targetType, targetID string, details any) error {
	payload := []byte("{}")
	if details != nil {
		b, err := json.Marshal(details)
		if err != nil {
			return err
		}
		payload = b
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO audit_logs(actor_sub, action, target_type, target_id, details)
		VALUES($1,$2,$3,$4,$5::jsonb)
	`, actorSub, action, targetType, targetID, string(payload))
	return err
}

func (s *Server) adminListAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if v, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit"))); err == nil && v > 0 {
		if v > 1000 {
			v = 1000
		}
		limit = v
	}
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	actor := strings.TrimSpace(r.URL.Query().Get("actor_sub"))
	targetType := strings.TrimSpace(r.URL.Query().Get("target_type"))

	rows, err := s.db.Query(r.Context(), `
		SELECT id, COALESCE(actor_sub,''), action, COALESCE(target_type,''), COALESCE(target_id,''), COALESCE(details::text,'{}'), created_at::text
		FROM audit_logs
		WHERE ($1='' OR action=$1)
		  AND ($2='' OR actor_sub=$2)
		  AND ($3='' OR target_type=$3)
		ORDER BY created_at DESC
		LIMIT $4
	`, action, actor, targetType, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := make([]map[string]any, 0, limit)
	for rows.Next() {
		var id int64
		var actorSub, actionV, targetTypeV, targetID, detailsRaw, createdAt string
		if err := rows.Scan(&id, &actorSub, &actionV, &targetTypeV, &targetID, &detailsRaw, &createdAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		details := map[string]any{}
		_ = json.Unmarshal([]byte(detailsRaw), &details)
		out = append(out, map[string]any{
			"id":          id,
			"actor_sub":   actorSub,
			"action":      actionV,
			"target_type": targetTypeV,
			"target_id":   targetID,
			"details":     details,
			"created_at":  createdAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"logs":  out,
		"count": len(out),
		"limit": limit,
	})
}

func (s *Server) adminListDebugLogs(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if v, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit"))); err == nil && v > 0 {
		if v > 1000 {
			v = 1000
		}
		limit = v
	}
	eventType := strings.TrimSpace(r.URL.Query().Get("event_type"))
	if eventType == "" {
		eventType = strings.TrimSpace(r.URL.Query().Get("action"))
	}
	endpoint := strings.TrimSpace(r.URL.Query().Get("endpoint"))

	rows, err := s.db.Query(r.Context(), `
		SELECT id, COALESCE(actor_sub,''), COALESCE(event_type,''), COALESCE(endpoint,''), COALESCE(http_method,''), COALESCE(status_code,0), COALESCE(client_ip,''), COALESCE(user_agent,''), COALESCE(details::text,'{}'), created_at::text
		FROM debug_log_events
		WHERE ($1='' OR event_type=$1)
		  AND ($2='' OR endpoint=$2)
		ORDER BY created_at DESC
		LIMIT $3
	`, eventType, endpoint, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := make([]map[string]any, 0, limit)
	for rows.Next() {
		var id int64
		var actorSub, evType, ep, method, ip, ua, detailsRaw, createdAt string
		var statusCode int
		if err := rows.Scan(&id, &actorSub, &evType, &ep, &method, &statusCode, &ip, &ua, &detailsRaw, &createdAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		details := map[string]any{}
		_ = json.Unmarshal([]byte(detailsRaw), &details)
		details["http_method"] = method
		details["status_code"] = statusCode
		details["client_ip"] = ip
		details["user_agent"] = ua
		out = append(out, map[string]any{
			"id":          id,
			"actor_sub":   actorSub,
			"action":      evType,
			"target_type": "endpoint",
			"target_id":   ep,
			"details":     details,
			"created_at":  createdAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"logs":  out,
		"count": len(out),
		"limit": limit,
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
