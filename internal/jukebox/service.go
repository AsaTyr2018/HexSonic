package jukebox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"hexsonic/internal/config"
)

type Service struct {
	cfg config.Config
	db  *pgxpool.Pool
}

type StartRequest struct {
	Mode           string `json:"mode"`
	SeedGenre      string `json:"seed_genre"`
	SeedCreatorSub string `json:"seed_creator_sub"`
	SeedAlbumID    int64  `json:"seed_album_id"`
}

type NextRequest struct {
	SessionID       string `json:"session_id"`
	CurrentPosition int    `json:"current_position"`
}

type FeedbackRequest struct {
	SessionID string `json:"session_id"`
	TrackID   string `json:"track_id"`
	Action    string `json:"action"`
}

type SessionSnapshot struct {
	SessionID      string          `json:"session_id"`
	Mode           string          `json:"mode"`
	SeedGenre      string          `json:"seed_genre"`
	SeedCreatorSub string          `json:"seed_creator_sub"`
	SeedAlbumID    int64           `json:"seed_album_id"`
	Summary        string          `json:"summary"`
	Options        sessionOptions  `json:"options"`
	Queue          []QueuedTrack   `json:"queue"`
	Controls       map[string]bool `json:"controls"`
}

type QueuedTrack struct {
	Position        int     `json:"position"`
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Artist          string  `json:"artist"`
	Album           string  `json:"album"`
	AlbumID         int64   `json:"album_id"`
	Genre           string  `json:"genre"`
	DurationSeconds float64 `json:"duration_seconds"`
	OwnerSub        string  `json:"owner_sub"`
	UploaderName    string  `json:"uploader_name"`
	Score           float64 `json:"score"`
	Reason          string  `json:"reason"`
}

type sessionState struct {
	ID             uuid.UUID
	UserSub        string
	Mode           string
	SeedGenre      string
	SeedCreatorSub string
	SeedAlbumID    int64
	Options        sessionOptions
}

type sessionOptions struct {
	BoostedGenres   []string `json:"boosted_genres,omitempty"`
	MutedGenres     []string `json:"muted_genres,omitempty"`
	BoostedCreators []string `json:"boosted_creators,omitempty"`
	MutedCreators   []string `json:"muted_creators,omitempty"`
	BannedTrackIDs  []string `json:"banned_track_ids,omitempty"`
	FixedGenre      string   `json:"fixed_genre,omitempty"`
	SurpriseBias    int      `json:"surprise_bias,omitempty"`
}

type tasteProfile struct {
	PreferredGenres map[string]float64
	GenreSignals    map[string]float64
	CreatorSignals  map[string]float64
	AlbumSignals    map[int64]float64
}

type candidateTrack struct {
	ID              string
	Title           string
	Artist          string
	Album           string
	AlbumID         int64
	Genre           string
	DurationSeconds float64
	OwnerSub        string
	UploaderName    string
	Rating          float64
	Plays           int64
	RecentScore     float64
}

type repeatPolicy struct {
	MaxTrackPlaysPerHour    int
	MaxCreatorTracksPerHour int
}

type recentConstraints struct {
	TrackCounts    map[string]int
	CreatorCounts  map[string]int
	SessionTrackID map[string]bool
}

type albumSeedContext struct {
	Artist string
	Genres []string
}

const jukeboxTargetQueueSize = 6

func New(cfg config.Config, db *pgxpool.Pool) *Service {
	return &Service{cfg: cfg, db: db}
}

func (s *Service) Router() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"service": "hexsonic-jukebox", "status": "ok"})
	})
	r.Post("/internal/jukebox/start", s.handleStart)
	r.Post("/internal/jukebox/next", s.handleNext)
	r.Post("/internal/jukebox/feedback", s.handleFeedback)
	r.Get("/internal/jukebox/sessions/{sessionID}", s.handleGetSession)
	return r
}

func (s *Service) handleStart(w http.ResponseWriter, r *http.Request) {
	userSub := strings.TrimSpace(r.Header.Get("X-Hexsonic-User-Sub"))
	if userSub == "" {
		http.Error(w, "missing user", http.StatusUnauthorized)
		return
	}
	var req StartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	snap, err := s.StartSession(r.Context(), userSub, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Service) handleNext(w http.ResponseWriter, r *http.Request) {
	userSub := strings.TrimSpace(r.Header.Get("X-Hexsonic-User-Sub"))
	if userSub == "" {
		http.Error(w, "missing user", http.StatusUnauthorized)
		return
	}
	var req NextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	snap, err := s.Next(r.Context(), userSub, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Service) handleFeedback(w http.ResponseWriter, r *http.Request) {
	userSub := strings.TrimSpace(r.Header.Get("X-Hexsonic-User-Sub"))
	if userSub == "" {
		http.Error(w, "missing user", http.StatusUnauthorized)
		return
	}
	var req FeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	snap, err := s.Feedback(r.Context(), userSub, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Service) handleGetSession(w http.ResponseWriter, r *http.Request) {
	userSub := strings.TrimSpace(r.Header.Get("X-Hexsonic-User-Sub"))
	if userSub == "" {
		http.Error(w, "missing user", http.StatusUnauthorized)
		return
	}
	sessionID := chi.URLParam(r, "sessionID")
	snap, err := s.GetSession(r.Context(), userSub, sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Service) StartSession(ctx context.Context, userSub string, req StartRequest) (SessionSnapshot, error) {
	mode := normalizeMode(req.Mode)
	if mode == "" {
		return SessionSnapshot{}, errors.New("invalid mode")
	}
	profile, err := s.loadTasteProfile(ctx, strings.TrimSpace(userSub))
	if err != nil {
		return SessionSnapshot{}, err
	}
	state := sessionState{
		ID:             uuid.New(),
		UserSub:        strings.TrimSpace(userSub),
		Mode:           mode,
		SeedGenre:      strings.TrimSpace(req.SeedGenre),
		SeedCreatorSub: strings.TrimSpace(req.SeedCreatorSub),
		SeedAlbumID:    req.SeedAlbumID,
		Options:        sessionOptions{},
	}
	if state.Mode == "genre" && state.SeedGenre == "" {
		state.SeedGenre, _ = s.inferSeedGenre(ctx, profile)
	}
	if state.Mode == "creator" && state.SeedCreatorSub == "" {
		state.SeedCreatorSub, _ = s.inferSeedCreator(ctx, profile)
	}
	if state.Mode == "album" && state.SeedAlbumID <= 0 {
		state.SeedAlbumID, _ = s.inferSeedAlbum(ctx, profile)
	}
	if state.Mode == "genre" && state.SeedGenre == "" {
		return SessionSnapshot{}, errors.New("seed_genre required")
	}
	if state.Mode == "creator" && state.SeedCreatorSub == "" {
		return SessionSnapshot{}, errors.New("seed_creator_sub required")
	}
	if state.Mode == "album" && state.SeedAlbumID <= 0 {
		return SessionSnapshot{}, errors.New("seed_album_id required")
	}
	if reused, ok, err := s.findReusableSession(ctx, state); err != nil {
		return SessionSnapshot{}, err
	} else if ok {
		if err := s.ensureQueue(ctx, &reused, jukeboxTargetQueueSize); err != nil {
			return SessionSnapshot{}, err
		}
		_, _ = s.db.Exec(ctx, `UPDATE jukebox_sessions SET updated_at=now(), last_activity_at=now() WHERE id=$1`, reused.ID)
		return s.snapshot(ctx, reused)
	}
	optionsJSON, _ := json.Marshal(state.Options)
	if _, err := s.db.Exec(ctx, `
		INSERT INTO jukebox_sessions(id, user_sub, mode, seed_genre, seed_creator_sub, seed_album_id, options_json, status, created_at, updated_at, last_activity_at)
		VALUES($1, $2, $3, $4, $5, NULLIF($6, 0), $7, 'active', now(), now(), now())
	`, state.ID, state.UserSub, state.Mode, state.SeedGenre, state.SeedCreatorSub, state.SeedAlbumID, optionsJSON); err != nil {
		return SessionSnapshot{}, err
	}
	if err := s.ensureQueue(ctx, &state, jukeboxTargetQueueSize); err != nil {
		return SessionSnapshot{}, err
	}
	return s.snapshot(ctx, state)
}

func (s *Service) findReusableSession(ctx context.Context, target sessionState) (sessionState, bool, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
			id,
			user_sub,
			mode,
			COALESCE(seed_genre, ''),
			COALESCE(seed_creator_sub, ''),
			COALESCE(seed_album_id, 0),
			COALESCE(options_json, '{}'::jsonb)
		FROM jukebox_sessions
		WHERE user_sub=$1
		  AND mode=$2
		  AND status='active'
		ORDER BY created_at DESC
	`, target.UserSub, target.Mode)
	if err != nil {
		return sessionState{}, false, err
	}
	defer rows.Close()

	var newest *sessionState
	var stale []uuid.UUID
	for rows.Next() {
		var candidate sessionState
		var optionsRaw []byte
		if err := rows.Scan(
			&candidate.ID,
			&candidate.UserSub,
			&candidate.Mode,
			&candidate.SeedGenre,
			&candidate.SeedCreatorSub,
			&candidate.SeedAlbumID,
			&optionsRaw,
		); err != nil {
			return sessionState{}, false, err
		}
		_ = json.Unmarshal(optionsRaw, &candidate.Options)
		if !sameSessionIdentity(candidate, target) {
			continue
		}
		if newest == nil {
			copy := candidate
			newest = &copy
			continue
		}
		stale = append(stale, candidate.ID)
	}
	if newest == nil {
		return sessionState{}, false, nil
	}
	if len(stale) > 0 {
		_, _ = s.db.Exec(ctx, `
			UPDATE jukebox_sessions
			SET status='ended', updated_at=now()
			WHERE id = ANY($1::uuid[])
		`, stale)
	}
	return *newest, true, nil
}

func sameSessionIdentity(a, b sessionState) bool {
	if a.Mode != b.Mode || a.UserSub != b.UserSub {
		return false
	}
	switch a.Mode {
	case "genre":
		return strings.EqualFold(strings.TrimSpace(a.SeedGenre), strings.TrimSpace(b.SeedGenre))
	case "creator":
		return strings.TrimSpace(a.SeedCreatorSub) == strings.TrimSpace(b.SeedCreatorSub)
	case "album":
		return a.SeedAlbumID == b.SeedAlbumID
	default:
		return true
	}
}

func (s *Service) Next(ctx context.Context, userSub string, req NextRequest) (SessionSnapshot, error) {
	state, err := s.loadSession(ctx, userSub, req.SessionID)
	if err != nil {
		return SessionSnapshot{}, err
	}
	if req.CurrentPosition > 0 {
		_, _ = s.db.Exec(ctx, `
			UPDATE jukebox_session_tracks jst
			SET played_at = COALESCE(jst.played_at, now())
			WHERE jst.id IN (
				SELECT id
				FROM jukebox_session_tracks
				WHERE session_id=$1
				  AND played_at IS NULL
				ORDER BY created_at, id
				LIMIT $2
			)
		`, state.ID, req.CurrentPosition)
	}
	targetSize := jukeboxTargetQueueSize
	if err := s.ensureQueue(ctx, &state, targetSize); err != nil {
		return SessionSnapshot{}, err
	}
	return s.snapshot(ctx, state)
}

func (s *Service) Feedback(ctx context.Context, userSub string, req FeedbackRequest) (SessionSnapshot, error) {
	state, err := s.loadSession(ctx, userSub, req.SessionID)
	if err != nil {
		return SessionSnapshot{}, err
	}
	action := normalizeFeedbackAction(req.Action)
	if action == "" {
		return SessionSnapshot{}, errors.New("invalid action")
	}
	track, err := s.trackByID(ctx, req.TrackID)
	if err != nil && req.TrackID != "" {
		return SessionSnapshot{}, err
	}
	applyFeedbackToOptions(&state.Options, track, action)
	payloadJSON, _ := json.Marshal(map[string]any{
		"genre":     track.Genre,
		"owner_sub": track.OwnerSub,
		"album_id":  track.AlbumID,
	})
	if _, err := s.db.Exec(ctx, `
		INSERT INTO jukebox_feedback_events(session_id, user_sub, track_id, action, payload_json, created_at)
		VALUES($1, $2, NULLIF($3, '')::uuid, $4, $5, now())
	`, state.ID, state.UserSub, strings.TrimSpace(req.TrackID), action, payloadJSON); err != nil {
		return SessionSnapshot{}, err
	}
	optionsJSON, _ := json.Marshal(state.Options)
	if _, err := s.db.Exec(ctx, `
		UPDATE jukebox_sessions
		SET options_json=$2, updated_at=now(), last_activity_at=now()
		WHERE id=$1
	`, state.ID, optionsJSON); err != nil {
		return SessionSnapshot{}, err
	}
	if action == "skip" && req.TrackID != "" {
		_, _ = s.db.Exec(ctx, `
			UPDATE jukebox_session_tracks
			SET played_at = COALESCE(played_at, now())
			WHERE session_id=$1 AND track_id=$2::uuid
		`, state.ID, strings.TrimSpace(req.TrackID))
	}
	if req.TrackID != "" {
		_, _ = s.db.Exec(ctx, `
			UPDATE jukebox_session_tracks
			SET played_at = COALESCE(played_at, now())
			WHERE session_id=$1 AND track_id=$2::uuid
		`, state.ID, strings.TrimSpace(req.TrackID))
	}
	_, _ = s.db.Exec(ctx, `
		DELETE FROM jukebox_session_tracks
		WHERE session_id=$1
		  AND played_at IS NULL
	`, state.ID)
	if err := s.ensureQueue(ctx, &state, jukeboxTargetQueueSize); err != nil {
		return SessionSnapshot{}, err
	}
	return s.snapshot(ctx, state)
}

func (s *Service) GetSession(ctx context.Context, userSub, sessionID string) (SessionSnapshot, error) {
	state, err := s.loadSession(ctx, userSub, sessionID)
	if err != nil {
		return SessionSnapshot{}, err
	}
	return s.snapshot(ctx, state)
}

func (s *Service) loadSession(ctx context.Context, userSub, sessionID string) (sessionState, error) {
	id, err := uuid.Parse(strings.TrimSpace(sessionID))
	if err != nil {
		return sessionState{}, errors.New("invalid session_id")
	}
	var out sessionState
	var optionsRaw []byte
	if err := s.db.QueryRow(ctx, `
		SELECT id, user_sub, mode, COALESCE(seed_genre,''), COALESCE(seed_creator_sub,''), COALESCE(seed_album_id,0), COALESCE(options_json, '{}'::jsonb)
		FROM jukebox_sessions
		WHERE id=$1 AND user_sub=$2 AND status='active'
	`, id, strings.TrimSpace(userSub)).Scan(
		&out.ID,
		&out.UserSub,
		&out.Mode,
		&out.SeedGenre,
		&out.SeedCreatorSub,
		&out.SeedAlbumID,
		&optionsRaw,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sessionState{}, errors.New("session not found")
		}
		return sessionState{}, err
	}
	_ = json.Unmarshal(optionsRaw, &out.Options)
	return out, nil
}

func (s *Service) snapshot(ctx context.Context, state sessionState) (SessionSnapshot, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
			row_number() OVER (ORDER BY jst.created_at, jst.id)::int AS position,
			t.id::text,
			t.title,
			COALESCE(ar.name,'') AS artist,
			COALESCE(al.title,'') AS album,
			COALESCE(al.id,0) AS album_id,
			COALESCE(t.genre,'') AS genre,
			COALESCE(t.duration_seconds,0)::float8 AS duration_seconds,
			t.owner_sub,
			COALESCE(NULLIF(up.display_name,''), t.owner_sub) AS uploader_name,
			jst.score,
			jst.reason
		FROM jukebox_session_tracks jst
		JOIN tracks t ON t.id=jst.track_id
		LEFT JOIN artists ar ON ar.id=t.artist_id
		LEFT JOIN albums al ON al.id=t.album_id
		LEFT JOIN user_profiles up ON up.user_sub=t.owner_sub
		WHERE jst.session_id=$1
		  AND jst.played_at IS NULL
		ORDER BY jst.created_at, jst.id
		LIMIT $2
	`, state.ID, jukeboxTargetQueueSize)
	if err != nil {
		return SessionSnapshot{}, err
	}
	defer rows.Close()
	queue := make([]QueuedTrack, 0, 16)
	for rows.Next() {
		var item QueuedTrack
		if err := rows.Scan(
			&item.Position,
			&item.ID,
			&item.Title,
			&item.Artist,
			&item.Album,
			&item.AlbumID,
			&item.Genre,
			&item.DurationSeconds,
			&item.OwnerSub,
			&item.UploaderName,
			&item.Score,
			&item.Reason,
		); err != nil {
			return SessionSnapshot{}, err
		}
		queue = append(queue, item)
	}
	_, _ = s.db.Exec(ctx, `UPDATE jukebox_sessions SET updated_at=now(), last_activity_at=now() WHERE id=$1`, state.ID)
	controls := map[string]bool{
		"skip": true,
	}
	if state.Mode != "radio" {
		controls["more_like_this"] = true
		controls["less_like_this"] = true
		controls["stay_in_genre"] = true
		controls["surprise_me"] = true
	}
	return SessionSnapshot{
		SessionID:      state.ID.String(),
		Mode:           state.Mode,
		SeedGenre:      state.SeedGenre,
		SeedCreatorSub: state.SeedCreatorSub,
		SeedAlbumID:    state.SeedAlbumID,
		Summary:        buildSummary(state),
		Options:        state.Options,
		Queue:          queue,
		Controls:       controls,
	}, nil
}

func (s *Service) ensureQueue(ctx context.Context, state *sessionState, desiredTotal int) error {
	var existingTotal int
	if err := s.db.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM jukebox_session_tracks
		WHERE session_id=$1
		  AND played_at IS NULL
	`, state.ID).Scan(&existingTotal); err != nil {
		return err
	}
	if existingTotal >= desiredTotal {
		return nil
	}
	needed := desiredTotal - existingTotal
	profile, err := s.loadTasteProfile(ctx, state.UserSub)
	if err != nil {
		return err
	}
	policy, err := s.loadRepeatPolicy(ctx)
	if err != nil {
		return err
	}
	recent, err := s.loadRecentConstraints(ctx, state.UserSub, state.ID)
	if err != nil {
		return err
	}
	candidates, err := s.loadCandidates(ctx)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return nil
	}
	if state.Mode == "album" && state.SeedAlbumID > 0 && strings.TrimSpace(state.SeedGenre) == "" {
		state.SeedGenre, _ = s.albumGenre(ctx, state.SeedAlbumID)
	}
	if state.Mode == "creator" && strings.TrimSpace(state.SeedGenre) == "" {
		if genres := dominantGenresForCreator(candidates, state.SeedCreatorSub, 1); len(genres) > 0 {
			state.SeedGenre = genres[0]
		}
	}
	candidates = filterCandidatesForMode(candidates, *state, profile)
	if len(candidates) == 0 {
		return nil
	}
	scored := rankCandidates(candidates, *state, profile, policy, recent)
	if len(scored) == 0 {
		relaxedCreators := recent
		relaxedCreators.CreatorCounts = map[string]int{}
		scored = rankCandidates(candidates, *state, profile, policy, relaxedCreators)
	}
	if len(scored) == 0 {
		relaxedTracks := recent
		relaxedTracks.CreatorCounts = map[string]int{}
		relaxedTracks.TrackCounts = map[string]int{}
		scored = rankCandidates(candidates, *state, profile, policy, relaxedTracks)
	}
	picks := diversifyTracks(scored, needed, state.Mode)
	for _, pick := range picks {
		tag, err := s.db.Exec(ctx, `
			INSERT INTO jukebox_session_tracks(session_id, position, track_id, score, reason, created_at)
			VALUES($1, 0, $2::uuid, $3, $4, now())
			ON CONFLICT DO NOTHING
		`, state.ID, pick.ID, pick.Score, pick.Reason)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			continue
		}
		recent.SessionTrackID[pick.ID] = true
		recent.TrackCounts[pick.ID]++
		recent.CreatorCounts[pick.OwnerSub]++
	}
	return nil
}

func filterCandidatesForMode(candidates []candidateTrack, state sessionState, profile tasteProfile) []candidateTrack {
	switch state.Mode {
	case "genre":
		out := make([]candidateTrack, 0, len(candidates))
		for _, c := range candidates {
			if genreMatchStrength(state.SeedGenre, c.Genre) > 0 {
				out = append(out, c)
			}
		}
		if len(out) > 0 {
			return out
		}
		return candidates
	case "creator":
		seedGenres := dominantGenresForCreator(candidates, state.SeedCreatorSub, 3)
		out := make([]candidateTrack, 0, len(candidates))
		for _, c := range candidates {
			if c.OwnerSub == state.SeedCreatorSub {
				out = append(out, c)
				continue
			}
			if matchesAnyGenre(seedGenres, c.Genre) {
				out = append(out, c)
				continue
			}
			if c.OwnerSub != "" && profile.CreatorSignals[c.OwnerSub] > 0 {
				out = append(out, c)
			}
		}
		if len(out) > 0 {
			return out
		}
		return candidates
	case "album":
		ctx := seedAlbumContext(candidates, state.SeedAlbumID)
		out := make([]candidateTrack, 0, len(candidates))
		for _, c := range candidates {
			if c.AlbumID == state.SeedAlbumID {
				out = append(out, c)
				continue
			}
			if ctx.Artist != "" && strings.EqualFold(strings.TrimSpace(c.Artist), ctx.Artist) {
				out = append(out, c)
				continue
			}
			if matchesAnyGenre(ctx.Genres, c.Genre) {
				out = append(out, c)
			}
		}
		if len(out) > 0 {
			return out
		}
		return candidates
	case "try_me":
		playCounts := make([]int64, 0, len(candidates))
		for _, c := range candidates {
			playCounts = append(playCounts, c.Plays)
		}
		if len(playCounts) == 0 {
			return candidates
		}
		slices.Sort(playCounts)
		threshold := playCounts[min(len(playCounts)-1, max(0, int(float64(len(playCounts))*0.20)))]
		out := make([]candidateTrack, 0, len(candidates))
		for _, c := range candidates {
			if c.Plays <= threshold || c.RecentScore <= 2 {
				out = append(out, c)
			}
		}
		if len(out) > 0 {
			return out
		}
		return candidates
	default:
		return candidates
	}
}

func dominantGenresForCreator(candidates []candidateTrack, creatorSub string, limit int) []string {
	if strings.TrimSpace(creatorSub) == "" || limit <= 0 {
		return nil
	}
	counts := map[string]int{}
	for _, c := range candidates {
		if c.OwnerSub != creatorSub {
			continue
		}
		g := strings.ToLower(strings.TrimSpace(c.Genre))
		if g == "" {
			continue
		}
		counts[g]++
	}
	type pair struct {
		genre string
		count int
	}
	items := make([]pair, 0, len(counts))
	for genre, count := range counts {
		items = append(items, pair{genre: genre, count: count})
	}
	slices.SortFunc(items, func(a, b pair) int {
		if a.count == b.count {
			return strings.Compare(a.genre, b.genre)
		}
		if a.count > b.count {
			return -1
		}
		return 1
	})
	out := make([]string, 0, min(limit, len(items)))
	for i := 0; i < len(items) && i < limit; i++ {
		out = append(out, items[i].genre)
	}
	return out
}

func seedAlbumContext(candidates []candidateTrack, albumID int64) albumSeedContext {
	out := albumSeedContext{Genres: []string{}}
	if albumID <= 0 {
		return out
	}
	genres := map[string]bool{}
	for _, c := range candidates {
		if c.AlbumID != albumID {
			continue
		}
		if out.Artist == "" && strings.TrimSpace(c.Artist) != "" {
			out.Artist = strings.TrimSpace(c.Artist)
		}
		g := strings.ToLower(strings.TrimSpace(c.Genre))
		if g != "" {
			genres[g] = true
		}
	}
	for genre := range genres {
		out.Genres = append(out.Genres, genre)
	}
	slices.Sort(out.Genres)
	return out
}

func matchesAnyGenre(seeds []string, candidate string) bool {
	for _, seed := range seeds {
		if genreMatchStrength(seed, candidate) > 0 {
			return true
		}
	}
	return false
}

func genreMatchStrength(seed, candidate string) float64 {
	seedKey := strings.ToLower(strings.TrimSpace(seed))
	candidateKey := strings.ToLower(strings.TrimSpace(candidate))
	if seedKey == "" || candidateKey == "" {
		return 0
	}
	if seedKey == candidateKey {
		return 1
	}
	seedTokens := genreTokens(seedKey)
	candidateTokens := genreTokens(candidateKey)
	if len(seedTokens) == 0 || len(candidateTokens) == 0 {
		return 0
	}
	common := 0
	for token := range seedTokens {
		if candidateTokens[token] {
			common++
		}
	}
	if common == 0 {
		return 0
	}
	shorter := min(len(seedTokens), len(candidateTokens))
	return float64(common) / float64(shorter)
}

func genreTokens(value string) map[string]bool {
	fields := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(value)), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	out := map[string]bool{}
	for _, field := range fields {
		if len(field) < 3 {
			continue
		}
		out[field] = true
	}
	return out
}

func (s *Service) inferSeedGenre(ctx context.Context, profile tasteProfile) (string, error) {
	bestGenre := ""
	bestScore := -math.MaxFloat64
	for genre, score := range profile.PreferredGenres {
		if score > bestScore && strings.TrimSpace(genre) != "" {
			bestGenre, bestScore = genre, score
		}
	}
	for genre, score := range profile.GenreSignals {
		if score > bestScore && strings.TrimSpace(genre) != "" {
			bestGenre, bestScore = genre, score
		}
	}
	if bestGenre != "" {
		return bestGenre, nil
	}
	var genre string
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(NULLIF(LOWER(t.genre), ''), '')
		FROM tracks t
		WHERE t.visibility='public' AND COALESCE(t.genre,'') <> ''
		GROUP BY LOWER(t.genre)
		ORDER BY COUNT(*) DESC, LOWER(t.genre)
		LIMIT 1
	`).Scan(&genre)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(genre), nil
}

func (s *Service) inferSeedCreator(ctx context.Context, profile tasteProfile) (string, error) {
	bestCreator := ""
	bestScore := -math.MaxFloat64
	for creator, score := range profile.CreatorSignals {
		if score > bestScore && strings.TrimSpace(creator) != "" {
			bestCreator, bestScore = creator, score
		}
	}
	if bestCreator != "" {
		return bestCreator, nil
	}
	var ownerSub string
	err := s.db.QueryRow(ctx, `
		SELECT t.owner_sub
		FROM tracks t
		WHERE t.visibility='public' AND COALESCE(t.owner_sub,'') <> ''
		GROUP BY t.owner_sub
		ORDER BY COUNT(*) DESC, t.owner_sub
		LIMIT 1
	`).Scan(&ownerSub)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(ownerSub), nil
}

func (s *Service) inferSeedAlbum(ctx context.Context, profile tasteProfile) (int64, error) {
	var bestAlbumID int64
	bestScore := -math.MaxFloat64
	for albumID, score := range profile.AlbumSignals {
		if albumID > 0 && score > bestScore {
			bestAlbumID, bestScore = albumID, score
		}
	}
	if bestAlbumID > 0 {
		return bestAlbumID, nil
	}
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(t.album_id, 0)
		FROM tracks t
		WHERE t.visibility='public' AND t.album_id IS NOT NULL
		GROUP BY t.album_id
		ORDER BY COUNT(*) DESC, t.album_id DESC
		LIMIT 1
	`).Scan(&bestAlbumID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return bestAlbumID, nil
}

func (s *Service) loadTasteProfile(ctx context.Context, userSub string) (tasteProfile, error) {
	out := tasteProfile{
		PreferredGenres: map[string]float64{},
		GenreSignals:    map[string]float64{},
		CreatorSignals:  map[string]float64{},
		AlbumSignals:    map[int64]float64{},
	}
	var preferred []string
	if err := s.db.QueryRow(ctx, `
		SELECT COALESCE(jukebox_preferred_genres, '{}'::TEXT[])
		FROM user_profiles
		WHERE user_sub=$1
	`, userSub).Scan(&preferred); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	for _, genre := range preferred {
		g := strings.ToLower(strings.TrimSpace(genre))
		if g != "" {
			out.PreferredGenres[g] = 4
		}
	}
	rows, err := s.db.Query(ctx, `
		SELECT
			COALESCE(LOWER(NULLIF(t.genre,'')), '') AS genre,
			t.owner_sub,
			COALESCE(t.album_id, 0) AS album_id,
			SUM(
				CASE le.event_type
					WHEN 'play_complete' THEN 5
					WHEN 'play_50_percent' THEN 3
					WHEN 'play_30s' THEN 2
					WHEN 'playlist_add' THEN 4
					WHEN 'rating' THEN 3
					WHEN 'skip_early' THEN -3
					ELSE 0
				END
			)::float8 AS signal
		FROM listening_events le
		JOIN tracks t ON t.id=le.track_id
		WHERE le.user_sub=$1
		  AND le.created_at >= now() - interval '180 day'
		GROUP BY genre, t.owner_sub, t.album_id
	`, userSub)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var genre, ownerSub string
		var albumID int64
		var signal float64
		if err := rows.Scan(&genre, &ownerSub, &albumID, &signal); err != nil {
			return out, err
		}
		if genre != "" {
			out.GenreSignals[genre] += signal
		}
		if ownerSub != "" {
			out.CreatorSignals[ownerSub] += signal
		}
		if albumID > 0 {
			out.AlbumSignals[albumID] += signal
		}
	}
	return out, nil
}

func (s *Service) loadRepeatPolicy(ctx context.Context) (repeatPolicy, error) {
	out := repeatPolicy{MaxTrackPlaysPerHour: 1, MaxCreatorTracksPerHour: 12}
	if v, err := s.appSettingInt(ctx, "jukebox_max_track_plays_per_hour", out.MaxTrackPlaysPerHour); err == nil {
		out.MaxTrackPlaysPerHour = max(v, 1)
	} else {
		return out, err
	}
	if v, err := s.appSettingInt(ctx, "jukebox_max_creator_tracks_per_hour", out.MaxCreatorTracksPerHour); err == nil {
		out.MaxCreatorTracksPerHour = max(v, 1)
	} else {
		return out, err
	}
	return out, nil
}

func (s *Service) loadRecentConstraints(ctx context.Context, userSub string, sessionID uuid.UUID) (recentConstraints, error) {
	out := recentConstraints{
		TrackCounts:    map[string]int{},
		CreatorCounts:  map[string]int{},
		SessionTrackID: map[string]bool{},
	}
	rows, err := s.db.Query(ctx, `
		SELECT le.track_id::text, COALESCE(t.owner_sub, '')
		FROM listening_events le
		JOIN tracks t ON t.id=le.track_id
		WHERE le.user_sub=$1
		  AND le.event_type IN ('play_start','play_30s','play_50_percent','play_complete')
		  AND le.created_at >= now() - interval '1 hour'
	`, userSub)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var trackID, ownerSub string
		if err := rows.Scan(&trackID, &ownerSub); err != nil {
			rows.Close()
			return out, err
		}
		out.TrackCounts[trackID]++
		if ownerSub != "" {
			out.CreatorCounts[ownerSub]++
		}
	}
	rows.Close()
	rows, err = s.db.Query(ctx, `
		SELECT jst.track_id::text, COALESCE(t.owner_sub,'')
		FROM jukebox_session_tracks jst
		JOIN tracks t ON t.id=jst.track_id
		WHERE jst.session_id=$1
		  AND jst.created_at >= now() - interval '1 hour'
	`, sessionID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var trackID, ownerSub string
		if err := rows.Scan(&trackID, &ownerSub); err != nil {
			return out, err
		}
		out.SessionTrackID[trackID] = true
		out.TrackCounts[trackID]++
		if ownerSub != "" {
			out.CreatorCounts[ownerSub]++
		}
	}
	return out, nil
}

func (s *Service) loadCandidates(ctx context.Context) ([]candidateTrack, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
			t.id::text,
			t.title,
			COALESCE(ar.name,'') AS artist,
			COALESCE(al.title,'') AS album,
			COALESCE(al.id,0) AS album_id,
			COALESCE(t.genre,'') AS genre,
			COALESCE(t.duration_seconds,0)::float8 AS duration_seconds,
			t.owner_sub,
			COALESCE(NULLIF(up.display_name,''), t.owner_sub) AS uploader_name,
			COALESCE(rr.avg_rating,0)::float8 AS rating,
			COALESCE(pe.plays,0)::bigint AS plays,
			COALESCE(rs.recent_score,0)::float8 AS recent_score
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
				CASE le.event_type
					WHEN 'play_complete' THEN 5
					WHEN 'play_50_percent' THEN 3
					WHEN 'play_30s' THEN 2
					WHEN 'playlist_add' THEN 4
					WHEN 'rating' THEN 3
					WHEN 'skip_early' THEN -2
					ELSE 0
				END
			),0)::float8 AS recent_score
			FROM listening_events le
			WHERE le.track_id=t.id
			  AND le.created_at >= now() - interval '30 day'
		) rs ON true
		WHERE t.visibility='public'
		ORDER BY COALESCE(pe.plays,0) DESC, lower(t.title)
		LIMIT 600
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]candidateTrack, 0, 256)
	for rows.Next() {
		var item candidateTrack
		if err := rows.Scan(
			&item.ID,
			&item.Title,
			&item.Artist,
			&item.Album,
			&item.AlbumID,
			&item.Genre,
			&item.DurationSeconds,
			&item.OwnerSub,
			&item.UploaderName,
			&item.Rating,
			&item.Plays,
			&item.RecentScore,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Service) appSettingInt(ctx context.Context, key string, fallback int) (int, error) {
	var raw string
	err := s.db.QueryRow(ctx, `SELECT value_text FROM app_settings WHERE key=$1`, key).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fallback, nil
		}
		return fallback, err
	}
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback, nil
	}
	return v, nil
}

func (s *Service) trackByID(ctx context.Context, trackID string) (candidateTrack, error) {
	var item candidateTrack
	if strings.TrimSpace(trackID) == "" {
		return item, nil
	}
	err := s.db.QueryRow(ctx, `
		SELECT
			t.id::text,
			t.title,
			COALESCE(ar.name,'') AS artist,
			COALESCE(al.title,'') AS album,
			COALESCE(al.id,0) AS album_id,
			COALESCE(t.genre,'') AS genre,
			COALESCE(t.duration_seconds,0)::float8 AS duration_seconds,
			t.owner_sub,
			COALESCE(NULLIF(up.display_name,''), t.owner_sub) AS uploader_name,
			0::float8,
			0::bigint,
			0::float8
		FROM tracks t
		LEFT JOIN artists ar ON ar.id=t.artist_id
		LEFT JOIN albums al ON al.id=t.album_id
		LEFT JOIN user_profiles up ON up.user_sub=t.owner_sub
		WHERE t.id=$1::uuid
	`, strings.TrimSpace(trackID)).Scan(
		&item.ID,
		&item.Title,
		&item.Artist,
		&item.Album,
		&item.AlbumID,
		&item.Genre,
		&item.DurationSeconds,
		&item.OwnerSub,
		&item.UploaderName,
		&item.Rating,
		&item.Plays,
		&item.RecentScore,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return item, errors.New("track not found")
		}
		return item, err
	}
	return item, nil
}

func (s *Service) albumGenre(ctx context.Context, albumID int64) (string, error) {
	var genre string
	err := s.db.QueryRow(ctx, `SELECT COALESCE(genre,'') FROM albums WHERE id=$1`, albumID).Scan(&genre)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(genre), nil
}

type scoredTrack struct {
	candidateTrack
	Score  float64
	Reason string
}

func rankCandidates(candidates []candidateTrack, state sessionState, profile tasteProfile, policy repeatPolicy, recent recentConstraints) []scoredTrack {
	out := make([]scoredTrack, 0, len(candidates))
	for _, c := range candidates {
		if recent.SessionTrackID[c.ID] || recent.TrackCounts[c.ID] >= policy.MaxTrackPlaysPerHour {
			continue
		}
		if c.OwnerSub != "" && recent.CreatorCounts[c.OwnerSub] >= policy.MaxCreatorTracksPerHour {
			continue
		}
		score, reason, ok := scoreCandidate(c, state, profile, recent)
		if !ok {
			continue
		}
		out = append(out, scoredTrack{
			candidateTrack: c,
			Score:          score,
			Reason:         reason,
		})
	}
	slices.SortFunc(out, func(a, b scoredTrack) int {
		if a.Score == b.Score {
			return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
		}
		if a.Score > b.Score {
			return -1
		}
		return 1
	})
	return out
}

func diversifyTracks(scored []scoredTrack, needed int, mode string) []scoredTrack {
	if needed <= 0 || len(scored) == 0 {
		return nil
	}
	pool := append([]scoredTrack(nil), scored...)
	picks := make([]scoredTrack, 0, min(len(pool), needed))
	recentAlbums := make([]int64, 0, 4)
	recentGenres := make([]string, 0, 4)
	albumPickCounts := map[int64]int{}
	maxPerAlbum := 2
	if mode == "album" {
		maxPerAlbum = 3
	}
	for len(picks) < needed && len(pool) > 0 {
		bestIdx := pickDiversifiedTrack(pool, recentAlbums, recentGenres, albumPickCounts, mode, maxPerAlbum, true)
		if bestIdx < 0 {
			bestIdx = pickDiversifiedTrack(pool, recentAlbums, recentGenres, albumPickCounts, mode, maxPerAlbum, false)
		}
		if bestIdx < 0 {
			bestIdx = 0
		}
		pick := pool[bestIdx]
		picks = append(picks, pick)
		if pick.AlbumID > 0 {
			albumPickCounts[pick.AlbumID]++
		}
		recentAlbums = appendWindowInt64(recentAlbums, pick.AlbumID, 4)
		recentGenres = appendWindowString(recentGenres, pick.Genre, 4)
		pool = append(pool[:bestIdx], pool[bestIdx+1:]...)
	}
	return picks
}

func pickDiversifiedTrack(pool []scoredTrack, recentAlbums []int64, recentGenres []string, albumPickCounts map[int64]int, mode string, maxPerAlbum int, enforceCap bool) int {
	bestIdx := -1
	bestScore := -math.MaxFloat64
	window := min(len(pool), 20)
	lastAlbumID := int64(0)
	if len(recentAlbums) > 0 {
		lastAlbumID = recentAlbums[len(recentAlbums)-1]
	}
	for i := 0; i < window; i++ {
		item := pool[i]
		if item.AlbumID > 0 && enforceCap {
			if albumPickCounts[item.AlbumID] >= maxPerAlbum {
				continue
			}
			if lastAlbumID > 0 && item.AlbumID == lastAlbumID && len(pool) > 1 {
				continue
			}
		}
		adjusted := item.Score
		albumRepeats := countRecentAlbum(recentAlbums, item.AlbumID)
		genreRepeats := countRecentGenre(recentGenres, item.Genre)
		if item.AlbumID > 0 {
			if lastAlbumID > 0 && item.AlbumID == lastAlbumID {
				adjusted -= 18
			}
			switch albumRepeats {
			case 1:
				adjusted -= 10
			case 2:
				adjusted -= 18
			default:
				if albumRepeats >= 3 {
					adjusted -= 30
				}
			}
			if albumPickCounts[item.AlbumID] >= 1 {
				adjusted -= 8 * float64(albumPickCounts[item.AlbumID])
			}
		}
		if genreRepeats >= 1 {
			adjusted -= 2.8 * float64(genreRepeats)
		}
		if mode == "album" && item.AlbumID > 0 && lastAlbumID > 0 && item.AlbumID == lastAlbumID {
			adjusted -= 14
		}
		if adjusted > bestScore {
			bestScore = adjusted
			bestIdx = i
		}
	}
	return bestIdx
}

func countRecentAlbum(items []int64, albumID int64) int {
	if albumID <= 0 {
		return 0
	}
	total := 0
	for _, item := range items {
		if item == albumID {
			total++
		}
	}
	return total
}

func countRecentGenre(items []string, genre string) int {
	key := strings.ToLower(strings.TrimSpace(genre))
	if key == "" {
		return 0
	}
	total := 0
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(item)) == key {
			total++
		}
	}
	return total
}

func appendWindowInt64(items []int64, value int64, size int) []int64 {
	items = append(items, value)
	if len(items) > size {
		items = items[len(items)-size:]
	}
	return items
}

func appendWindowString(items []string, value string, size int) []string {
	items = append(items, value)
	if len(items) > size {
		items = items[len(items)-size:]
	}
	return items
}

func scoreCandidate(c candidateTrack, state sessionState, profile tasteProfile, recent recentConstraints) (float64, string, bool) {
	genreKey := strings.ToLower(strings.TrimSpace(c.Genre))
	base := (float64(minInt64(c.Plays, 80)) * 0.05) + (c.Rating * 0.75) + (c.RecentScore * 0.18)
	preference := profile.PreferredGenres[genreKey] + (profile.GenreSignals[genreKey] * 0.18) + (profile.CreatorSignals[c.OwnerSub] * 0.08) + (profile.AlbumSignals[c.AlbumID] * 0.12)
	modeBonus := 0.0
	reason := "Taste-aligned rotation"
	if state.Mode == "radio" {
		preference = 0
	}

	switch state.Mode {
	case "for_you":
		reason = "Based on your listening"
	case "radio":
		base = (float64(minInt64(c.Plays, 200)) * 0.11) + (c.Rating * 1.1) + (c.RecentScore * 1.45)
		modeBonus += 4
		reason = "Radio rotation: most played now"
	case "genre":
		match := genreMatchStrength(state.SeedGenre, c.Genre)
		if match <= 0 {
			return 0, "", false
		}
		if strings.EqualFold(strings.TrimSpace(state.SeedGenre), strings.TrimSpace(c.Genre)) {
			modeBonus += 7
			reason = fmt.Sprintf("Genre radio core: %s", strings.TrimSpace(c.Genre))
		} else {
			modeBonus += 3.5 * match
			reason = fmt.Sprintf("Genre-adjacent: %s", strings.TrimSpace(c.Genre))
		}
	case "creator":
		if state.SeedCreatorSub == "" {
			return 0, "", false
		}
		if c.OwnerSub == state.SeedCreatorSub {
			modeBonus += 8
			reason = "From the selected creator"
		} else {
			creatorAdjacency := profile.CreatorSignals[c.OwnerSub] * 0.06
			genreAdjacency := genreMatchStrength(state.SeedGenre, c.Genre) * 2.5
			modeBonus += creatorAdjacency + genreAdjacency
			reason = "Creator-adjacent bridge"
		}
	case "album":
		if state.SeedAlbumID <= 0 {
			return 0, "", false
		}
		if c.AlbumID == state.SeedAlbumID {
			modeBonus += 9
			reason = "Album radio core selection"
		} else if state.SeedGenre != "" && genreMatchStrength(state.SeedGenre, c.Genre) > 0 {
			modeBonus += 3
			reason = "Album-adjacent genre bridge"
		} else {
			return 0, "", false
		}
	case "try_me":
		novelty := math.Max(0, 16-(float64(c.Plays)*0.14))
		qualityGate := (c.Rating * 0.4) + math.Max(0, c.RecentScore*0.08)
		modeBonus += novelty + qualityGate
		reason = "Try me: low-play discovery"
	}

	if state.Options.FixedGenre != "" {
		if !strings.EqualFold(strings.TrimSpace(state.Options.FixedGenre), strings.TrimSpace(c.Genre)) {
			return 0, "", false
		}
		modeBonus += 2
		reason = fmt.Sprintf("Locked to %s", strings.TrimSpace(c.Genre))
	}
	if genreKey != "" && slices.Contains(state.Options.BoostedGenres, genreKey) {
		modeBonus += 3
		reason = fmt.Sprintf("More like %s", c.Genre)
	}
	if genreKey != "" && slices.Contains(state.Options.MutedGenres, genreKey) {
		modeBonus -= 4
	}
	if c.OwnerSub != "" && slices.Contains(state.Options.BoostedCreators, c.OwnerSub) {
		modeBonus += 2
	}
	if c.OwnerSub != "" && slices.Contains(state.Options.MutedCreators, c.OwnerSub) {
		modeBonus -= 3
	}
	if c.ID != "" && slices.Contains(state.Options.BannedTrackIDs, c.ID) {
		return 0, "", false
	}
	if state.Options.SurpriseBias > 0 {
		modeBonus += math.Max(0, 10-(float64(c.Plays)*0.1)) * float64(state.Options.SurpriseBias) * 0.35
		if state.Mode != "try_me" && reason == "Taste-aligned rotation" {
			reason = "Surprise bias within your taste lane"
		}
	}

	jitter := rand.Float64() * 0.35
	score := base + preference + modeBonus + jitter
	if score <= -2 {
		return 0, "", false
	}
	return score, reason, true
}

func applyFeedbackToOptions(opts *sessionOptions, track candidateTrack, action string) {
	genreKey := strings.ToLower(strings.TrimSpace(track.Genre))
	switch action {
	case "more_like_this":
		if genreKey != "" && !slices.Contains(opts.BoostedGenres, genreKey) {
			opts.BoostedGenres = append(opts.BoostedGenres, genreKey)
		}
		if track.OwnerSub != "" && !slices.Contains(opts.BoostedCreators, track.OwnerSub) {
			opts.BoostedCreators = append(opts.BoostedCreators, track.OwnerSub)
		}
	case "less_like_this":
		if genreKey != "" && !slices.Contains(opts.MutedGenres, genreKey) {
			opts.MutedGenres = append(opts.MutedGenres, genreKey)
		}
		if track.OwnerSub != "" && !slices.Contains(opts.MutedCreators, track.OwnerSub) {
			opts.MutedCreators = append(opts.MutedCreators, track.OwnerSub)
		}
		if track.ID != "" && !slices.Contains(opts.BannedTrackIDs, track.ID) {
			opts.BannedTrackIDs = append(opts.BannedTrackIDs, track.ID)
		}
	case "stay_in_genre":
		if genreKey != "" {
			opts.FixedGenre = genreKey
		}
	case "surprise_me":
		opts.SurpriseBias++
	case "skip":
		if track.ID != "" && !slices.Contains(opts.BannedTrackIDs, track.ID) {
			opts.BannedTrackIDs = append(opts.BannedTrackIDs, track.ID)
		}
	}
}

func normalizeMode(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "for_you", "radio", "try_me":
		return strings.TrimSpace(strings.ToLower(raw))
	case "genre", "creator", "album":
		return "radio"
	default:
		return ""
	}
}

func normalizeFeedbackAction(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "more_like_this", "less_like_this", "stay_in_genre", "surprise_me", "skip":
		return strings.TrimSpace(strings.ToLower(raw))
	default:
		return ""
	}
}

func buildSummary(state sessionState) string {
	switch state.Mode {
	case "for_you":
		return "Auto-built around your listening profile and preferred genres."
	case "radio":
		return "A mixed auto-DJ lane built from the platform's strongest public tracks."
	case "genre":
		return fmt.Sprintf("Locked to public tracks in %s.", state.SeedGenre)
	case "creator":
		return "Focused on the selected creator and nearby taste signals."
	case "album":
		return "Starts from the selected album and stays close to its lane."
	case "try_me":
		return "Pushes into low-play and underexposed public tracks."
	default:
		return "Adaptive jukebox session."
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
