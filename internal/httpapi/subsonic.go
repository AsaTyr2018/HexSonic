package httpapi

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"hexsonic/internal/auth"
)

const (
	subsonicAPIVersion    = "1.16.1"
	subsonicType          = "HEXSONIC"
	subsonicServerVersion = "0.1"
	subsonicXMLNS         = "http://subsonic.org/restapi"
)

type subsonicResponse struct {
	XMLName        xml.Name               `xml:"subsonic-response" json:"subsonic-response"`
	XMLNS          string                 `xml:"xmlns,attr,omitempty" json:"-"`
	Status         string                 `xml:"status,attr" json:"status"`
	Version        string                 `xml:"version,attr" json:"version"`
	Type           string                 `xml:"type,attr,omitempty" json:"type,omitempty"`
	ServerVersion  string                 `xml:"serverVersion,attr,omitempty" json:"serverVersion,omitempty"`
	OpenSubsonic   bool                   `xml:"openSubsonic,attr,omitempty" json:"openSubsonic,omitempty"`
	Error          *subsonicError         `xml:"error,omitempty" json:"error,omitempty"`
	License        *subsonicLicense       `xml:"license,omitempty" json:"license,omitempty"`
	MusicFolders   *subsonicMusicFolders  `xml:"musicFolders,omitempty" json:"musicFolders,omitempty"`
	Indexes        *subsonicIndexes       `xml:"indexes,omitempty" json:"indexes,omitempty"`
	Artists        *subsonicArtists       `xml:"artists,omitempty" json:"artists,omitempty"`
	Artist         *subsonicArtist        `xml:"artist,omitempty" json:"artist,omitempty"`
	Album          *subsonicAlbum         `xml:"album,omitempty" json:"album,omitempty"`
	AlbumList2     *subsonicAlbumList2    `xml:"albumList2,omitempty" json:"albumList2,omitempty"`
	Song           *subsonicSong          `xml:"song,omitempty" json:"song,omitempty"`
	Genres         *subsonicGenres        `xml:"genres,omitempty" json:"genres,omitempty"`
	SearchResult3  *subsonicSearchResult3 `xml:"searchResult3,omitempty" json:"searchResult3,omitempty"`
	Playlists      *subsonicPlaylists     `xml:"playlists,omitempty" json:"playlists,omitempty"`
	Playlist       *subsonicPlaylist      `xml:"playlist,omitempty" json:"playlist,omitempty"`
	NowPlaying     *subsonicNowPlaying    `xml:"nowPlaying,omitempty" json:"nowPlaying,omitempty"`
	User           *subsonicUser          `xml:"user,omitempty" json:"user,omitempty"`
	OpenExtensions *subsonicExtensions    `xml:"openSubsonicExtensions,omitempty" json:"openSubsonicExtensions,omitempty"`
	Directory      *subsonicDirectory     `xml:"directory,omitempty" json:"directory,omitempty"`
	ScanStatus     *subsonicScanStatus    `xml:"scanStatus,omitempty" json:"scanStatus,omitempty"`
}

type subsonicError struct {
	Code    int    `xml:"code,attr" json:"code"`
	Message string `xml:"message,attr" json:"message"`
}

type subsonicLicense struct {
	Valid bool `xml:"valid,attr" json:"valid"`
}

type subsonicMusicFolders struct {
	Folders []subsonicMusicFolder `xml:"musicFolder" json:"musicFolder"`
}

type subsonicMusicFolder struct {
	ID   string `xml:"id,attr" json:"id"`
	Name string `xml:"name,attr" json:"name"`
}

type subsonicIndexes struct {
	IgnoredArticles string           `xml:"ignoredArticles,attr,omitempty" json:"ignoredArticles,omitempty"`
	LastModified    int64            `xml:"lastModified,attr" json:"lastModified"`
	Index           []subsonicIndex  `xml:"index" json:"index"`
	Shortcuts       []subsonicArtist `xml:"shortcut,omitempty" json:"shortcut,omitempty"`
}

type subsonicArtists struct {
	IgnoredArticles string          `xml:"ignoredArticles,attr,omitempty" json:"ignoredArticles,omitempty"`
	Index           []subsonicIndex `xml:"index" json:"index"`
}

type subsonicIndex struct {
	Name   string           `xml:"name,attr" json:"name"`
	Artist []subsonicArtist `xml:"artist" json:"artist"`
}

type subsonicArtist struct {
	ID         string          `xml:"id,attr" json:"id"`
	Name       string          `xml:"name,attr" json:"name"`
	AlbumCount int             `xml:"albumCount,attr,omitempty" json:"albumCount,omitempty"`
	Album      []subsonicAlbum `xml:"album,omitempty" json:"album,omitempty"`
}

type subsonicAlbumList2 struct {
	Album []subsonicAlbum `xml:"album" json:"album"`
}

type subsonicAlbum struct {
	ID        string         `xml:"id,attr" json:"id"`
	Name      string         `xml:"name,attr" json:"name"`
	Title     string         `xml:"title,attr,omitempty" json:"title,omitempty"`
	Artist    string         `xml:"artist,attr,omitempty" json:"artist,omitempty"`
	ArtistID  string         `xml:"artistId,attr,omitempty" json:"artistId,omitempty"`
	CoverArt  string         `xml:"coverArt,attr,omitempty" json:"coverArt,omitempty"`
	SongCount int            `xml:"songCount,attr,omitempty" json:"songCount,omitempty"`
	Duration  int            `xml:"duration,attr,omitempty" json:"duration,omitempty"`
	Created   string         `xml:"created,attr,omitempty" json:"created,omitempty"`
	Genre     string         `xml:"genre,attr,omitempty" json:"genre,omitempty"`
	Song      []subsonicSong `xml:"song,omitempty" json:"song,omitempty"`
}

type subsonicSong struct {
	ID          string `xml:"id,attr" json:"id"`
	Parent      string `xml:"parent,attr,omitempty" json:"parent,omitempty"`
	IsDir       bool   `xml:"isDir,attr" json:"isDir"`
	Title       string `xml:"title,attr" json:"title"`
	Album       string `xml:"album,attr,omitempty" json:"album,omitempty"`
	Artist      string `xml:"artist,attr,omitempty" json:"artist,omitempty"`
	Track       int    `xml:"track,attr,omitempty" json:"track,omitempty"`
	Duration    int    `xml:"duration,attr,omitempty" json:"duration,omitempty"`
	Genre       string `xml:"genre,attr,omitempty" json:"genre,omitempty"`
	Created     string `xml:"created,attr,omitempty" json:"created,omitempty"`
	CoverArt    string `xml:"coverArt,attr,omitempty" json:"coverArt,omitempty"`
	Size        int64  `xml:"size,attr,omitempty" json:"size,omitempty"`
	ContentType string `xml:"contentType,attr,omitempty" json:"contentType,omitempty"`
	Suffix      string `xml:"suffix,attr,omitempty" json:"suffix,omitempty"`
}

type subsonicSearchResult3 struct {
	Artist []subsonicArtist `xml:"artist" json:"artist"`
	Album  []subsonicAlbum  `xml:"album" json:"album"`
	Song   []subsonicSong   `xml:"song" json:"song"`
}

type subsonicGenres struct {
	Genre []subsonicGenre `xml:"genre" json:"genre"`
}

type subsonicGenre struct {
	SongCount  int    `xml:"songCount,attr" json:"songCount"`
	AlbumCount int    `xml:"albumCount,attr" json:"albumCount"`
	Value      string `xml:"value,attr" json:"value"`
}

type subsonicPlaylists struct {
	Playlist []subsonicPlaylist `xml:"playlist" json:"playlist"`
}

type subsonicPlaylist struct {
	ID        string         `xml:"id,attr" json:"id"`
	Name      string         `xml:"name,attr" json:"name"`
	Owner     string         `xml:"owner,attr,omitempty" json:"owner,omitempty"`
	Public    bool           `xml:"public,attr" json:"public"`
	SongCount int            `xml:"songCount,attr,omitempty" json:"songCount,omitempty"`
	Duration  int            `xml:"duration,attr,omitempty" json:"duration,omitempty"`
	Created   string         `xml:"created,attr,omitempty" json:"created,omitempty"`
	Entry     []subsonicSong `xml:"entry,omitempty" json:"entry,omitempty"`
}

type subsonicNowPlaying struct {
	Entry []subsonicSong `xml:"entry,omitempty" json:"entry,omitempty"`
}

type subsonicUser struct {
	Username          string `xml:"username,attr" json:"username"`
	Email             string `xml:"email,attr,omitempty" json:"email,omitempty"`
	AdminRole         bool   `xml:"adminRole,attr" json:"adminRole"`
	SettingsRole      bool   `xml:"settingsRole,attr" json:"settingsRole"`
	DownloadRole      bool   `xml:"downloadRole,attr" json:"downloadRole"`
	UploadRole        bool   `xml:"uploadRole,attr" json:"uploadRole"`
	PlaylistRole      bool   `xml:"playlistRole,attr" json:"playlistRole"`
	CoverArtRole      bool   `xml:"coverArtRole,attr" json:"coverArtRole"`
	CommentRole       bool   `xml:"commentRole,attr" json:"commentRole"`
	StreamRole        bool   `xml:"streamRole,attr" json:"streamRole"`
	JukeboxRole       bool   `xml:"jukeboxRole,attr" json:"jukeboxRole"`
	ShareRole         bool   `xml:"shareRole,attr" json:"shareRole"`
	PodcastRole       bool   `xml:"podcastRole,attr" json:"podcastRole"`
	VideoConvRole     bool   `xml:"videoConversionRole,attr" json:"videoConversionRole"`
	ScrobblingEnabled bool   `xml:"scrobblingEnabled,attr" json:"scrobblingEnabled"`
}

type subsonicExtensions struct {
	Extension []subsonicExtension `xml:"extension" json:"extension"`
}

type subsonicExtension struct {
	Name    string `xml:"name,attr" json:"name"`
	Version int    `xml:"version,attr" json:"version"`
}

type subsonicDirectory struct {
	ID     string        `xml:"id,attr" json:"id"`
	Name   string        `xml:"name,attr,omitempty" json:"name,omitempty"`
	Parent string        `xml:"parent,attr,omitempty" json:"parent,omitempty"`
	Child  []subsonicSong `xml:"child,omitempty" json:"child,omitempty"`
}

type subsonicScanStatus struct {
	Scanning bool `xml:"scanning,attr" json:"scanning"`
	Count    int  `xml:"count,attr,omitempty" json:"count,omitempty"`
}

type subsonicStatusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *subsonicStatusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *subsonicStatusRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(p)
}

func subsonicParam(r *http.Request, key string) string {
	return strings.TrimSpace(r.FormValue(strings.TrimSpace(key)))
}

func (s *Server) subsonicHandler(w http.ResponseWriter, r *http.Request) {
	rec := &subsonicStatusRecorder{ResponseWriter: w}
	w = rec
	method := "unknown"
	actorSub := ""
	start := time.Now()
	defer func() {
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		_ = s.writeDebugLogEvent(r.Context(), "subsonic.request", method, r.Method, status, actorSub, clientIPFromRequest(r), r.UserAgent(), map[string]any{
			"path":        r.URL.Path,
			"duration_ms": time.Since(start).Milliseconds(),
		})
	}()

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, Origin, X-Requested-With")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	_ = r.ParseForm()

	method = strings.TrimSpace(chi.URLParam(r, "method"))
	if method == "" {
		method = strings.TrimSpace(chi.URLParam(r, "*"))
	}
	if method == "" {
		method = strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/rest/")
	}
	if idx := strings.LastIndex(strings.ToLower(method), "/rest/"); idx >= 0 {
		method = method[idx+len("/rest/"):]
	}
	method = strings.TrimPrefix(method, "/")
	method = strings.TrimSpace(method)
	for strings.HasPrefix(strings.ToLower(method), "rest/") {
		method = strings.TrimPrefix(method, "rest/")
		method = strings.TrimPrefix(method, "REST/")
	}
	if idx := strings.Index(method, "/"); idx >= 0 {
		method = method[:idx]
	}
	method = strings.TrimSuffix(method, ".view")
	method = strings.TrimSpace(method)
	if method == "" {
		log.Printf("subsonic: method=ping-empty http_method=%s path=%q ua=%q", r.Method, r.URL.Path, r.UserAgent())
		s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{
			Status:        "ok",
			Version:       subsonicAPIVersion,
			Type:          subsonicType,
			ServerVersion: subsonicServerVersion,
			OpenSubsonic:  true,
		})
		return
	}
	log.Printf("subsonic: method=%s http_method=%s path=%q ua=%q", method, r.Method, r.URL.Path, r.UserAgent())

	// A few clients probe server reachability before sending credentials.
	switch method {
	case "ping":
		if subsonicParam(r, "f") == "" && strings.Contains(strings.ToLower(strings.TrimSpace(r.Header.Get("Accept"))), "json") {
			q := r.Form
			q.Set("f", "json")
			r.URL.RawQuery = q.Encode()
		}
		s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{
			Status:        "ok",
			Version:       subsonicAPIVersion,
			Type:          subsonicType,
			ServerVersion: subsonicServerVersion,
			OpenSubsonic:  true,
		})
		return
	case "getLicense":
		s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{Status: "ok", Version: subsonicAPIVersion, Type: subsonicType, ServerVersion: subsonicServerVersion, OpenSubsonic: true, License: &subsonicLicense{Valid: true}})
		return
	case "getOpenSubsonicExtensions":
		s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{Status: "ok", Version: subsonicAPIVersion, Type: subsonicType, ServerVersion: subsonicServerVersion, OpenSubsonic: true, OpenExtensions: &subsonicExtensions{Extension: []subsonicExtension{{Name: "transcodeOffset", Version: 1}}}})
		return
	case "getScanStatus":
		s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{
			Status:        "ok",
			Version:       subsonicAPIVersion,
			Type:          subsonicType,
			ServerVersion: subsonicServerVersion,
			OpenSubsonic:  true,
			ScanStatus:    &subsonicScanStatus{Scanning: false, Count: 0},
		})
		return
	}

	claims, ok := s.subsonicAuthenticate(r)
	if !ok {
		log.Printf("subsonic: auth_failed method=%s path=%q", method, r.URL.Path)
		s.subsonicWriteError(w, r, http.StatusUnauthorized, 40, "authentication failed")
		return
	}
	actorSub = claims.Subject

	switch method {
	case "getMusicFolders":
		s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{Status: "ok", Version: subsonicAPIVersion, Type: subsonicType, ServerVersion: subsonicServerVersion, OpenSubsonic: true, MusicFolders: &subsonicMusicFolders{Folders: []subsonicMusicFolder{{ID: "1", Name: "HEXSONIC"}}}})
	case "getIndexes":
		s.subsonicGetIndexes(w, r, claims)
	case "getArtists":
		s.subsonicGetArtists(w, r, claims)
	case "getArtist":
		s.subsonicGetArtist(w, r, claims)
	case "getMusicDirectory":
		s.subsonicGetMusicDirectory(w, r, claims)
	case "getAlbumList2":
		s.subsonicGetAlbumList2(w, r, claims)
	case "getAlbum":
		s.subsonicGetAlbum(w, r, claims)
	case "getSong":
		s.subsonicGetSong(w, r, claims)
	case "getCoverArt":
		s.subsonicGetCoverArt(w, r, claims)
	case "stream":
		s.subsonicStream(w, r, claims)
	case "download":
		s.subsonicStream(w, r, claims)
	case "search3":
		s.subsonicSearch3(w, r, claims)
	case "getGenres":
		s.subsonicGetGenres(w, r, claims)
	case "getPlaylists":
		s.subsonicGetPlaylists(w, r, claims)
	case "getPlaylist":
		s.subsonicGetPlaylist(w, r, claims)
	case "getUser":
		s.subsonicGetUser(w, r, claims)
	case "scrobble":
		s.subsonicScrobble(w, r, claims)
	case "getNowPlaying":
		s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{Status: "ok", Version: subsonicAPIVersion, Type: subsonicType, ServerVersion: subsonicServerVersion, OpenSubsonic: true, NowPlaying: &subsonicNowPlaying{Entry: []subsonicSong{}}})
	default:
		log.Printf("subsonic: unknown_endpoint method=%s path=%q", method, r.URL.Path)
		s.subsonicWriteError(w, r, http.StatusNotFound, 70, "unknown endpoint")
	}
}

func (s *Server) subsonicGetMusicDirectory(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	id := subsonicParam(r, "id")
	if id == "" {
		s.subsonicWriteError(w, r, http.StatusBadRequest, 10, "id required")
		return
	}
	if id == "1" {
		albums, err := s.subsonicListAlbums(r.Context(), claims, "ORDER BY lower(a.title), a.id", 1000, 0)
		if err != nil {
			s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "directory lookup failed")
			return
		}
		children := make([]subsonicSong, 0, len(albums))
		for _, a := range albums {
			children = append(children, subsonicSong{
				ID:       a.ID,
				Title:    a.Title,
				Album:    a.Title,
				Artist:   a.Artist,
				IsDir:    true,
				CoverArt: a.ID,
				Parent:   "1",
			})
		}
		s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{
			Status:        "ok",
			Version:       subsonicAPIVersion,
			Type:          subsonicType,
			ServerVersion: subsonicServerVersion,
			OpenSubsonic:  true,
			Directory:     &subsonicDirectory{ID: "1", Name: "HEXSONIC", Child: children},
		})
		return
	}

	albumID, err := strconv.ParseInt(id, 10, 64)
	if err != nil || albumID <= 0 {
		s.subsonicWriteError(w, r, http.StatusNotFound, 70, "directory not found")
		return
	}
	allowed, err := s.canAccessAlbum(r.Context(), claims.Subject, auth.HasRole(claims, "admin"), albumID)
	if err != nil {
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "directory lookup failed")
		return
	}
	if !allowed {
		s.subsonicWriteError(w, r, http.StatusNotFound, 70, "directory not found")
		return
	}
	var title string
	if err := s.db.QueryRow(r.Context(), `SELECT COALESCE(title,'') FROM albums WHERE id=$1`, albumID).Scan(&title); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.subsonicWriteError(w, r, http.StatusNotFound, 70, "directory not found")
			return
		}
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "directory lookup failed")
		return
	}
	songs, err := s.subsonicListSongsByAlbum(r.Context(), claims, strconv.FormatInt(albumID, 10))
	if err != nil {
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "directory lookup failed")
		return
	}
	s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{
		Status:        "ok",
		Version:       subsonicAPIVersion,
		Type:          subsonicType,
		ServerVersion: subsonicServerVersion,
		OpenSubsonic:  true,
		Directory:     &subsonicDirectory{ID: strconv.FormatInt(albumID, 10), Name: title, Parent: "1", Child: songs},
	})
}

func (s *Server) subsonicAuthenticate(r *http.Request) (auth.Claims, bool) {
	if claims, ok := auth.FromContext(r.Context()); ok {
		return claims, true
	}

	if username, password, ok := r.BasicAuth(); ok {
		claims, err := s.subsonicPasswordLogin(r.Context(), strings.TrimSpace(username), password)
		if err == nil {
			return claims, true
		}
	}

	u := subsonicParam(r, "u")
	if u == "" {
		return auth.Claims{}, false
	}

	t := subsonicParam(r, "t")
	salt := subsonicParam(r, "s")
	if t != "" && salt != "" {
		claims, err := s.subsonicTokenLogin(r.Context(), u, t, salt)
		if err == nil {
			return claims, true
		}
	}

	p := subsonicParam(r, "p")
	if p == "" {
		return auth.Claims{}, false
	}
	if strings.HasPrefix(p, "enc:") {
		decoded, err := hex.DecodeString(strings.TrimPrefix(p, "enc:"))
		if err != nil {
			return auth.Claims{}, false
		}
		p = string(decoded)
	}
	claims, err := s.subsonicPasswordLogin(r.Context(), u, p)
	if err != nil {
		return auth.Claims{}, false
	}
	return claims, true
}

func (s *Server) subsonicTokenLogin(ctx context.Context, username, providedToken, salt string) (auth.Claims, error) {
	username = strings.TrimSpace(username)
	if username == "" || strings.TrimSpace(providedToken) == "" || strings.TrimSpace(salt) == "" {
		return auth.Claims{}, errors.New("missing token auth values")
	}
	userSub, err := s.keycloakUserSubByUsername(ctx, username)
	if err != nil {
		return auth.Claims{}, err
	}
	secret, err := s.userSubsonicPasswordBySub(ctx, userSub)
	if err != nil {
		return auth.Claims{}, err
	}
	if secret == "" {
		return auth.Claims{}, errors.New("subsonic password not configured")
	}
	sum := md5.Sum([]byte(secret + salt))
	expected := hex.EncodeToString(sum[:])
	if !strings.EqualFold(expected, strings.TrimSpace(providedToken)) {
		return auth.Claims{}, errors.New("invalid token")
	}
	return auth.Claims{
		Subject:  userSub,
		Username: username,
	}, nil
}

func (s *Server) subsonicPasswordLogin(ctx context.Context, username, password string) (auth.Claims, error) {
	if s.verifier == nil {
		return auth.Claims{}, errors.New("oidc verifier unavailable")
	}
	tokenURL := strings.TrimRight(s.cfg.OIDCIssuerURL, "/") + "/protocol/openid-connect/token"
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", s.cfg.OIDCClientID)
	if strings.TrimSpace(s.cfg.OIDCClientSecret) != "" {
		form.Set("client_secret", s.cfg.OIDCClientSecret)
	}
	form.Set("username", strings.TrimSpace(username))
	form.Set("password", password)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return auth.Claims{}, err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return auth.Claims{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return auth.Claims{}, errors.New("bad credentials")
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2*1024*1024)).Decode(&body); err != nil {
		return auth.Claims{}, err
	}
	if strings.TrimSpace(body.AccessToken) == "" {
		return auth.Claims{}, errors.New("missing access token")
	}
	return s.verifier.Verify(ctx, body.AccessToken)
}

func (s *Server) keycloakUserSubByUsername(ctx context.Context, username string) (string, error) {
	baseURL, realm, err := keycloakBaseAndRealm(s.cfg.OIDCIssuerURL)
	if err != nil {
		return "", err
	}
	adminToken, err := s.keycloakAdminToken(ctx, baseURL)
	if err != nil {
		return "", err
	}
	return s.keycloakFindUserID(ctx, baseURL, realm, adminToken, strings.TrimSpace(username))
}

func (s *Server) subsonicWriteError(w http.ResponseWriter, r *http.Request, status, code int, message string) {
	_ = status // Subsonic clients expect protocol-level errors with HTTP 200.
	s.subsonicWriteResponse(w, r, status, subsonicResponse{
		XMLNS:         subsonicXMLNS,
		Status:        "failed",
		Version:       subsonicAPIVersion,
		Type:          subsonicType,
		ServerVersion: subsonicServerVersion,
		OpenSubsonic:  true,
		Error:         &subsonicError{Code: code, Message: message},
	})
}

func (s *Server) subsonicWriteResponse(w http.ResponseWriter, r *http.Request, status int, payload subsonicResponse) {
	if payload.Status == "failed" {
		status = http.StatusOK
	}
	if strings.EqualFold(subsonicParam(r, "f"), "json") {
		raw, _ := json.Marshal(payload)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		delete(body, "subsonic-response")
		wrapped := map[string]any{
			"subsonic-response": body,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(wrapped)
		return
	}
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, xml.Header)
	enc := xml.NewEncoder(w)
	_ = enc.Encode(payload)
}

func subsonicArtistInitial(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "#"
	}
	r, _ := utf8.DecodeRuneInString(name)
	if r == utf8.RuneError {
		return "#"
	}
	r = unicode.ToUpper(r)
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return string(r)
	}
	return "#"
}

func (s *Server) subsonicGetIndexes(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	artists, err := s.subsonicListArtists(r.Context(), claims)
	if err != nil {
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "load artists failed")
		return
	}
	indexes := make(map[string][]subsonicArtist)
	keys := make([]string, 0, 32)
	for _, a := range artists {
		k := subsonicArtistInitial(a.Name)
		if _, ok := indexes[k]; !ok {
			keys = append(keys, k)
		}
		indexes[k] = append(indexes[k], a)
	}
	sort.Strings(keys)
	list := make([]subsonicIndex, 0, len(keys))
	for _, k := range keys {
		sort.Slice(indexes[k], func(i, j int) bool { return strings.ToLower(indexes[k][i].Name) < strings.ToLower(indexes[k][j].Name) })
		list = append(list, subsonicIndex{Name: k, Artist: indexes[k]})
	}
	s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{
		Status:        "ok",
		Version:       subsonicAPIVersion,
		Type:          subsonicType,
		ServerVersion: subsonicServerVersion,
		OpenSubsonic:  true,
		Indexes:       &subsonicIndexes{LastModified: time.Now().UnixMilli(), Index: list},
	})
}

func (s *Server) subsonicGetArtists(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	artists, err := s.subsonicListArtists(r.Context(), claims)
	if err != nil {
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "load artists failed")
		return
	}
	indexes := make(map[string][]subsonicArtist)
	keys := make([]string, 0, 32)
	for _, a := range artists {
		k := subsonicArtistInitial(a.Name)
		if _, ok := indexes[k]; !ok {
			keys = append(keys, k)
		}
		indexes[k] = append(indexes[k], a)
	}
	sort.Strings(keys)
	list := make([]subsonicIndex, 0, len(keys))
	for _, k := range keys {
		sort.Slice(indexes[k], func(i, j int) bool { return strings.ToLower(indexes[k][i].Name) < strings.ToLower(indexes[k][j].Name) })
		list = append(list, subsonicIndex{Name: k, Artist: indexes[k]})
	}
	s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{Status: "ok", Version: subsonicAPIVersion, Type: subsonicType, ServerVersion: subsonicServerVersion, OpenSubsonic: true, Artists: &subsonicArtists{Index: list}})
}

func (s *Server) subsonicListArtists(ctx context.Context, claims auth.Claims) ([]subsonicArtist, error) {
	isAdmin := auth.HasRole(claims, "admin")
	if isAdmin {
		rows, err := s.db.Query(ctx, `
			SELECT ar.id::text, ar.name, COUNT(DISTINCT t.album_id)::int
			FROM artists ar
			JOIN tracks t ON t.artist_id=ar.id
			GROUP BY ar.id, ar.name
			ORDER BY lower(ar.name)
			LIMIT 5000
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := make([]subsonicArtist, 0, 128)
		for rows.Next() {
			var a subsonicArtist
			if err := rows.Scan(&a.ID, &a.Name, &a.AlbumCount); err != nil {
				return nil, err
			}
			out = append(out, a)
		}
		return out, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT ar.id::text, ar.name, COUNT(DISTINCT t.album_id)::int
		FROM artists ar
		JOIN tracks t ON t.artist_id=ar.id
		WHERE
			t.owner_sub=$1
			OR t.visibility IN ('public','unlisted')
			OR (
				t.visibility='followers_only'
				AND EXISTS (SELECT 1 FROM follows f WHERE f.follower_sub=$1 AND f.followed_sub=t.owner_sub)
			)
		GROUP BY ar.id, ar.name
		ORDER BY lower(ar.name)
		LIMIT 5000
	`, claims.Subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]subsonicArtist, 0, 128)
	for rows.Next() {
		var a subsonicArtist
		if err := rows.Scan(&a.ID, &a.Name, &a.AlbumCount); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (s *Server) subsonicGetArtist(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	artistID := subsonicParam(r, "id")
	if artistID == "" {
		s.subsonicWriteError(w, r, http.StatusBadRequest, 10, "id required")
		return
	}
	var artist subsonicArtist
	if err := s.db.QueryRow(r.Context(), `SELECT id::text, name FROM artists WHERE id::text=$1`, artistID).Scan(&artist.ID, &artist.Name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.subsonicWriteError(w, r, http.StatusNotFound, 70, "artist not found")
			return
		}
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "artist lookup failed")
		return
	}
	albums, err := s.subsonicListAlbumsByArtist(r.Context(), claims, artistID, 500, 0)
	if err != nil {
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "album lookup failed")
		return
	}
	artist.Album = albums
	artist.AlbumCount = len(albums)
	s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{Status: "ok", Version: subsonicAPIVersion, Type: subsonicType, ServerVersion: subsonicServerVersion, OpenSubsonic: true, Artist: &artist})
}

func (s *Server) subsonicListAlbumsByArtist(ctx context.Context, claims auth.Claims, artistID string, size, from int) ([]subsonicAlbum, error) {
	isAdmin := auth.HasRole(claims, "admin")
	if isAdmin {
		rows, err := s.db.Query(ctx, `
			SELECT a.id::text, a.title, COALESCE(ar.name,''), COALESCE(a.artist_id::text,''), COALESCE(a.cover_path,''), a.created_at::text, COALESCE(a.genre,''),
				(SELECT COUNT(*)::int FROM tracks t WHERE t.album_id=a.id),
				(SELECT COALESCE(SUM(t.duration_seconds),0)::int FROM tracks t WHERE t.album_id=a.id)
			FROM albums a
			LEFT JOIN artists ar ON ar.id=a.artist_id
			WHERE a.artist_id::text=$1
			ORDER BY lower(a.title)
			LIMIT $2 OFFSET $3
		`, artistID, size, from)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return subsonicAlbumsFromRows(rows)
	}
	rows, err := s.db.Query(ctx, `
		SELECT a.id::text, a.title, COALESCE(ar.name,''), COALESCE(a.artist_id::text,''), COALESCE(a.cover_path,''), a.created_at::text, COALESCE(a.genre,''),
			(SELECT COUNT(*)::int FROM tracks t WHERE t.album_id=a.id AND (t.owner_sub=$1 OR t.visibility IN ('public','unlisted') OR (t.visibility='followers_only' AND EXISTS (SELECT 1 FROM follows f WHERE f.follower_sub=$1 AND f.followed_sub=t.owner_sub)))),
			(SELECT COALESCE(SUM(t.duration_seconds),0)::int FROM tracks t WHERE t.album_id=a.id AND (t.owner_sub=$1 OR t.visibility IN ('public','unlisted') OR (t.visibility='followers_only' AND EXISTS (SELECT 1 FROM follows f WHERE f.follower_sub=$1 AND f.followed_sub=t.owner_sub))))
		FROM albums a
		LEFT JOIN artists ar ON ar.id=a.artist_id
		WHERE a.artist_id::text=$2
		AND (a.owner_sub=$1 OR a.visibility='public')
		ORDER BY lower(a.title)
		LIMIT $3 OFFSET $4
	`, claims.Subject, artistID, size, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return subsonicAlbumsFromRows(rows)
}

func subsonicAlbumsFromRows(rows pgx.Rows) ([]subsonicAlbum, error) {
	out := make([]subsonicAlbum, 0, 64)
	for rows.Next() {
		var a subsonicAlbum
		var coverPath string
		if err := rows.Scan(&a.ID, &a.Name, &a.Artist, &a.ArtistID, &coverPath, &a.Created, &a.Genre, &a.SongCount, &a.Duration); err != nil {
			return nil, err
		}
		a.Title = a.Name
		if strings.TrimSpace(coverPath) != "" {
			a.CoverArt = a.ID
		}
		out = append(out, a)
	}
	return out, nil
}

func (s *Server) subsonicGetAlbumList2(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	typ := subsonicParam(r, "type")
	if typ == "" {
		typ = "alphabeticalByName"
	}
	size := 50
	if v, err := strconv.Atoi(subsonicParam(r, "size")); err == nil && v > 0 && v <= 500 {
		size = v
	}
	from := 0
	if v, err := strconv.Atoi(subsonicParam(r, "offset")); err == nil && v >= 0 {
		from = v
	}

	orderBy := "lower(a.title)"
	switch typ {
	case "newest", "recent":
		orderBy = "a.created_at DESC"
	case "random":
		orderBy = "random()"
	case "alphabeticalByName", "alphabeticalByArtist", "frequent":
		orderBy = "lower(a.title)"
	}
	albums, err := s.subsonicListAlbums(r.Context(), claims, orderBy, size, from)
	if err != nil {
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "load albums failed")
		return
	}
	s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{Status: "ok", Version: subsonicAPIVersion, Type: subsonicType, ServerVersion: subsonicServerVersion, OpenSubsonic: true, AlbumList2: &subsonicAlbumList2{Album: albums}})
}

func (s *Server) subsonicListAlbums(ctx context.Context, claims auth.Claims, orderBy string, size, from int) ([]subsonicAlbum, error) {
	isAdmin := auth.HasRole(claims, "admin")
	query := `
		SELECT a.id::text, a.title, COALESCE(ar.name,''), COALESCE(a.artist_id::text,''), COALESCE(a.cover_path,''), a.created_at::text, COALESCE(a.genre,''),
			(SELECT COUNT(*)::int FROM tracks t WHERE t.album_id=a.id),
			(SELECT COALESCE(SUM(t.duration_seconds),0)::int FROM tracks t WHERE t.album_id=a.id)
		FROM albums a
		LEFT JOIN artists ar ON ar.id=a.artist_id
		ORDER BY ` + orderBy + `
		LIMIT $1 OFFSET $2`
	var rows pgx.Rows
	var err error
	if isAdmin {
		rows, err = s.db.Query(ctx, query, size, from)
	} else {
		query = `
		SELECT a.id::text, a.title, COALESCE(ar.name,''), COALESCE(a.artist_id::text,''), COALESCE(a.cover_path,''), a.created_at::text, COALESCE(a.genre,''),
			(SELECT COUNT(*)::int FROM tracks t WHERE t.album_id=a.id AND (t.owner_sub=$1 OR t.visibility IN ('public','unlisted') OR (t.visibility='followers_only' AND EXISTS (SELECT 1 FROM follows f WHERE f.follower_sub=$1 AND f.followed_sub=t.owner_sub)))),
			(SELECT COALESCE(SUM(t.duration_seconds),0)::int FROM tracks t WHERE t.album_id=a.id AND (t.owner_sub=$1 OR t.visibility IN ('public','unlisted') OR (t.visibility='followers_only' AND EXISTS (SELECT 1 FROM follows f WHERE f.follower_sub=$1 AND f.followed_sub=t.owner_sub))))
		FROM albums a
		LEFT JOIN artists ar ON ar.id=a.artist_id
		WHERE a.owner_sub=$1 OR a.visibility='public'
		ORDER BY ` + orderBy + `
		LIMIT $2 OFFSET $3`
		rows, err = s.db.Query(ctx, query, claims.Subject, size, from)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return subsonicAlbumsFromRows(rows)
}

func (s *Server) subsonicGetAlbum(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	albumIDStr := subsonicParam(r, "id")
	albumID, err := strconv.ParseInt(albumIDStr, 10, 64)
	if err != nil || albumID <= 0 {
		s.subsonicWriteError(w, r, http.StatusBadRequest, 10, "invalid id")
		return
	}
	allowed, err := s.canAccessAlbum(r.Context(), claims.Subject, auth.HasRole(claims, "admin"), albumID)
	if err != nil {
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "album lookup failed")
		return
	}
	if !allowed {
		s.subsonicWriteError(w, r, http.StatusNotFound, 70, "album not found")
		return
	}
	var album subsonicAlbum
	var coverPath string
	err = s.db.QueryRow(r.Context(), `
		SELECT a.id::text, a.title, COALESCE(ar.name,''), COALESCE(a.artist_id::text,''), COALESCE(a.cover_path,''), a.created_at::text, COALESCE(a.genre,'')
		FROM albums a
		LEFT JOIN artists ar ON ar.id=a.artist_id
		WHERE a.id=$1
	`, albumID).Scan(&album.ID, &album.Name, &album.Artist, &album.ArtistID, &coverPath, &album.Created, &album.Genre)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.subsonicWriteError(w, r, http.StatusNotFound, 70, "album not found")
			return
		}
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "album lookup failed")
		return
	}
	album.Title = album.Name
	if strings.TrimSpace(coverPath) != "" {
		album.CoverArt = album.ID
	}
	songs, err := s.subsonicListSongsByAlbum(r.Context(), claims, albumIDStr)
	if err != nil {
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "track lookup failed")
		return
	}
	album.Song = songs
	album.SongCount = len(songs)
	for _, sng := range songs {
		album.Duration += sng.Duration
	}
	s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{Status: "ok", Version: subsonicAPIVersion, Type: subsonicType, ServerVersion: subsonicServerVersion, OpenSubsonic: true, Album: &album})
}

func (s *Server) subsonicListSongsByAlbum(ctx context.Context, claims auth.Claims, albumID string) ([]subsonicSong, error) {
	isAdmin := auth.HasRole(claims, "admin")
	query := `
		SELECT t.id::text, t.title, COALESCE(al.id::text,''), COALESCE(al.title,''), COALESCE(ar.name,''), COALESCE(t.track_number,0), COALESCE(t.duration_seconds,0)::int,
			COALESCE(t.genre,''), t.created_at::text, COALESCE(tf.file_path,''), COALESCE(tf.size_bytes,0), COALESCE(al.cover_path,'')
		FROM tracks t
		LEFT JOIN albums al ON al.id=t.album_id
		LEFT JOIN artists ar ON ar.id=t.artist_id
		LEFT JOIN LATERAL (
			SELECT file_path, size_bytes FROM track_files tf WHERE tf.track_id=t.id AND tf.is_original=true ORDER BY tf.created_at DESC LIMIT 1
		) tf ON true
		WHERE t.album_id::text=$1
		ORDER BY COALESCE(t.track_number,0), lower(t.title)
	`
	var rows pgx.Rows
	var err error
	if isAdmin {
		rows, err = s.db.Query(ctx, query, albumID)
	} else {
		query = `
		SELECT t.id::text, t.title, COALESCE(al.id::text,''), COALESCE(al.title,''), COALESCE(ar.name,''), COALESCE(t.track_number,0), COALESCE(t.duration_seconds,0)::int,
			COALESCE(t.genre,''), t.created_at::text, COALESCE(tf.file_path,''), COALESCE(tf.size_bytes,0), COALESCE(al.cover_path,'')
		FROM tracks t
		LEFT JOIN albums al ON al.id=t.album_id
		LEFT JOIN artists ar ON ar.id=t.artist_id
		LEFT JOIN LATERAL (
			SELECT file_path, size_bytes FROM track_files tf WHERE tf.track_id=t.id AND tf.is_original=true ORDER BY tf.created_at DESC LIMIT 1
		) tf ON true
		WHERE t.album_id::text=$1
		AND (
			t.owner_sub=$2 OR t.visibility IN ('public','unlisted') OR
			(t.visibility='followers_only' AND EXISTS (SELECT 1 FROM follows f WHERE f.follower_sub=$2 AND f.followed_sub=t.owner_sub))
		)
		ORDER BY COALESCE(t.track_number,0), lower(t.title)
		`
		rows, err = s.db.Query(ctx, query, albumID, claims.Subject)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]subsonicSong, 0, 64)
	for rows.Next() {
		var song subsonicSong
		var path, coverPath string
		if err := rows.Scan(&song.ID, &song.Title, &song.Parent, &song.Album, &song.Artist, &song.Track, &song.Duration, &song.Genre, &song.Created, &path, &song.Size, &coverPath); err != nil {
			return nil, err
		}
		song.IsDir = false
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
		song.Suffix = ext
		if ext != "" {
			song.ContentType = mime.TypeByExtension("." + ext)
		}
		if strings.TrimSpace(coverPath) != "" && strings.TrimSpace(song.Parent) != "" {
			song.CoverArt = song.Parent
		}
		out = append(out, song)
	}
	return out, nil
}

func (s *Server) subsonicGetSong(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	id := subsonicParam(r, "id")
	if id == "" {
		s.subsonicWriteError(w, r, http.StatusBadRequest, 10, "id required")
		return
	}
	allowed, err := s.canAccessTrack(r.Context(), claims.Subject, auth.HasRole(claims, "admin"), id)
	if err != nil {
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "lookup failed")
		return
	}
	if !allowed {
		s.subsonicWriteError(w, r, http.StatusNotFound, 70, "song not found")
		return
	}
	song, err := s.subsonicGetSongByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.subsonicWriteError(w, r, http.StatusNotFound, 70, "song not found")
			return
		}
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "lookup failed")
		return
	}
	s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{Status: "ok", Version: subsonicAPIVersion, Type: subsonicType, ServerVersion: subsonicServerVersion, OpenSubsonic: true, Song: &song})
}

func (s *Server) subsonicGetSongByID(ctx context.Context, id string) (subsonicSong, error) {
	var song subsonicSong
	var path, coverPath string
	err := s.db.QueryRow(ctx, `
		SELECT t.id::text, t.title, COALESCE(al.id::text,''), COALESCE(al.title,''), COALESCE(ar.name,''), COALESCE(t.track_number,0), COALESCE(t.duration_seconds,0)::int,
			COALESCE(t.genre,''), t.created_at::text, COALESCE(tf.file_path,''), COALESCE(tf.size_bytes,0), COALESCE(al.cover_path,'')
		FROM tracks t
		LEFT JOIN albums al ON al.id=t.album_id
		LEFT JOIN artists ar ON ar.id=t.artist_id
		LEFT JOIN LATERAL (
			SELECT file_path, size_bytes FROM track_files tf WHERE tf.track_id=t.id AND tf.is_original=true ORDER BY tf.created_at DESC LIMIT 1
		) tf ON true
		WHERE t.id=$1
	`, id).Scan(&song.ID, &song.Title, &song.Parent, &song.Album, &song.Artist, &song.Track, &song.Duration, &song.Genre, &song.Created, &path, &song.Size, &coverPath)
	if err != nil {
		return subsonicSong{}, err
	}
	song.IsDir = false
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	song.Suffix = ext
	if ext != "" {
		song.ContentType = mime.TypeByExtension("." + ext)
	}
	if strings.TrimSpace(coverPath) != "" && strings.TrimSpace(song.Parent) != "" {
		song.CoverArt = song.Parent
	}
	return song, nil
}

func (s *Server) subsonicStream(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	id := subsonicParam(r, "id")
	if id == "" {
		s.subsonicWriteError(w, r, http.StatusBadRequest, 10, "id required")
		return
	}
	allowed, err := s.canAccessTrack(r.Context(), claims.Subject, auth.HasRole(claims, "admin"), id)
	if err != nil {
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "lookup failed")
		return
	}
	if !allowed {
		s.subsonicWriteError(w, r, http.StatusNotFound, 70, "song not found")
		return
	}
	format := subsonicParam(r, "format")
	if format == "" {
		ua := strings.ToLower(r.UserAgent())
		if strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad") || strings.Contains(ua, "android") || strings.Contains(ua, "cfnetwork") {
			format = "m4a"
		} else {
			format = "mp3"
		}
	}
	path, mimeType, err := s.resolveStreamPath(r.Context(), id, format)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.subsonicWriteError(w, r, http.StatusNotFound, 70, "song not found")
			return
		}
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "stream resolve failed")
		return
	}
	f, err := os.Open(path)
	if err != nil {
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "stream open failed")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "stream stat failed")
		return
	}
	_, _ = s.db.Exec(r.Context(), `INSERT INTO play_events(track_id, user_sub) VALUES($1, $2)`, id, claims.Subject)
	w.Header().Set("Content-Type", mimeType)
	http.ServeContent(w, r, filepath.Base(path), st.ModTime(), f)
}

func (s *Server) subsonicGetCoverArt(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	id := subsonicParam(r, "id")
	if id == "" {
		s.subsonicWriteError(w, r, http.StatusBadRequest, 10, "id required")
		return
	}
	if albumID, err := strconv.ParseInt(id, 10, 64); err == nil && albumID > 0 {
		allowed, coverPath, err := s.canAccessAlbumCover(r.Context(), albumID, claims.Subject, auth.HasRole(claims, "admin"))
		if err != nil {
			s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "cover lookup failed")
			return
		}
		if !allowed || strings.TrimSpace(coverPath) == "" {
			s.subsonicWriteError(w, r, http.StatusNotFound, 70, "cover not found")
			return
		}
		s.subsonicServeImage(w, r, coverPath)
		return
	}
	var albumID int64
	if err := s.db.QueryRow(r.Context(), `SELECT COALESCE(album_id,0) FROM tracks WHERE id=$1`, id).Scan(&albumID); err != nil || albumID <= 0 {
		s.subsonicWriteError(w, r, http.StatusNotFound, 70, "cover not found")
		return
	}
	allowed, coverPath, err := s.canAccessAlbumCover(r.Context(), albumID, claims.Subject, auth.HasRole(claims, "admin"))
	if err != nil {
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "cover lookup failed")
		return
	}
	if !allowed || strings.TrimSpace(coverPath) == "" {
		s.subsonicWriteError(w, r, http.StatusNotFound, 70, "cover not found")
		return
	}
	s.subsonicServeImage(w, r, coverPath)
}

func (s *Server) subsonicServeImage(w http.ResponseWriter, r *http.Request, path string) {
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	_, _ = f.Seek(0, io.SeekStart)
	ctype := http.DetectContentType(buf[:n])
	if ctype == "" || ctype == "application/octet-stream" {
		ctype = mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
		if ctype == "" {
			ctype = "image/jpeg"
		}
	}
	w.Header().Set("Content-Type", ctype)
	http.ServeContent(w, r, filepath.Base(path), st.ModTime(), f)
}

func (s *Server) subsonicSearch3(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	query := subsonicParam(r, "query")
	if query == "" {
		s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{Status: "ok", Version: subsonicAPIVersion, Type: subsonicType, ServerVersion: subsonicServerVersion, OpenSubsonic: true, SearchResult3: &subsonicSearchResult3{}})
		return
	}
	q := "%" + strings.ToLower(query) + "%"

	artists := make([]subsonicArtist, 0, 20)
	albums := make([]subsonicAlbum, 0, 20)
	songs := make([]subsonicSong, 0, 20)

	arRows, err := s.db.Query(r.Context(), `SELECT id::text, name FROM artists WHERE lower(name) LIKE $1 ORDER BY lower(name) LIMIT 20`, q)
	if err == nil {
		defer arRows.Close()
		for arRows.Next() {
			var a subsonicArtist
			if arRows.Scan(&a.ID, &a.Name) == nil {
				artists = append(artists, a)
			}
		}
	}

	alRows, err := s.db.Query(r.Context(), `
		SELECT a.id::text, a.title, COALESCE(ar.name,''), COALESCE(a.artist_id::text,''), COALESCE(a.cover_path,''), a.created_at::text, COALESCE(a.genre,''),
			(SELECT COUNT(*)::int FROM tracks t WHERE t.album_id=a.id),
			(SELECT COALESCE(SUM(t.duration_seconds),0)::int FROM tracks t WHERE t.album_id=a.id)
		FROM albums a
		LEFT JOIN artists ar ON ar.id=a.artist_id
		WHERE lower(a.title) LIKE $1
		ORDER BY lower(a.title)
		LIMIT 20
	`, q)
	if err == nil {
		defer alRows.Close()
		if al, e := subsonicAlbumsFromRows(alRows); e == nil {
			albums = al
		}
	}

	tsRows, err := s.db.Query(r.Context(), `
		SELECT t.id::text
		FROM tracks t
		WHERE lower(t.title) LIKE $1
		ORDER BY lower(t.title)
		LIMIT 20
	`, q)
	if err == nil {
		defer tsRows.Close()
		for tsRows.Next() {
			var id string
			if tsRows.Scan(&id) == nil {
				allowed, e := s.canAccessTrack(r.Context(), claims.Subject, auth.HasRole(claims, "admin"), id)
				if e != nil || !allowed {
					continue
				}
				song, e := s.subsonicGetSongByID(r.Context(), id)
				if e == nil {
					songs = append(songs, song)
				}
			}
		}
	}

	s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{Status: "ok", Version: subsonicAPIVersion, Type: subsonicType, ServerVersion: subsonicServerVersion, OpenSubsonic: true, SearchResult3: &subsonicSearchResult3{Artist: artists, Album: albums, Song: songs}})
}

func (s *Server) subsonicGetGenres(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	isAdmin := auth.HasRole(claims, "admin")
	var rows pgx.Rows
	var err error
	if isAdmin {
		rows, err = s.db.Query(r.Context(), `
			SELECT COALESCE(NULLIF(trim(t.genre), ''), 'Unknown') AS genre,
				COUNT(*)::int AS song_count,
				COUNT(DISTINCT t.album_id)::int AS album_count
			FROM tracks t
			GROUP BY 1
			ORDER BY lower(genre)
		`)
	} else {
		rows, err = s.db.Query(r.Context(), `
			SELECT COALESCE(NULLIF(trim(t.genre), ''), 'Unknown') AS genre,
				COUNT(*)::int AS song_count,
				COUNT(DISTINCT t.album_id)::int AS album_count
			FROM tracks t
			WHERE
				t.owner_sub=$1
				OR t.visibility IN ('public','unlisted')
				OR (
					t.visibility='followers_only'
					AND EXISTS (
						SELECT 1 FROM follows f
						WHERE f.follower_sub=$1 AND f.followed_sub=t.owner_sub
					)
				)
			GROUP BY 1
			ORDER BY lower(genre)
		`, claims.Subject)
	}
	if err != nil {
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "genre lookup failed")
		return
	}
	defer rows.Close()

	out := make([]subsonicGenre, 0, 32)
	for rows.Next() {
		var g subsonicGenre
		if err := rows.Scan(&g.Value, &g.SongCount, &g.AlbumCount); err != nil {
			s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "genre lookup failed")
			return
		}
		out = append(out, g)
	}

	s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{
		Status:        "ok",
		Version:       subsonicAPIVersion,
		Type:          subsonicType,
		ServerVersion: subsonicServerVersion,
		OpenSubsonic:  true,
		Genres:        &subsonicGenres{Genre: out},
	})
}

func (s *Server) subsonicGetPlaylists(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	rows, err := s.db.Query(r.Context(), `
		SELECT p.id::text, p.name, p.owner_sub, p.visibility, p.created_at::text,
			(SELECT COUNT(*)::int FROM playlist_tracks pt WHERE pt.playlist_id=p.id)
		FROM playlists p
		WHERE p.owner_sub=$1 OR p.visibility='public'
		ORDER BY p.updated_at DESC
	`, claims.Subject)
	if err != nil {
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "playlist lookup failed")
		return
	}
	defer rows.Close()
	playlists := make([]subsonicPlaylist, 0, 32)
	for rows.Next() {
		var p subsonicPlaylist
		var ownerSub, vis string
		if err := rows.Scan(&p.ID, &p.Name, &ownerSub, &vis, &p.Created, &p.SongCount); err != nil {
			s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "playlist lookup failed")
			return
		}
		p.Owner = ownerSub
		p.Public = vis == "public"
		playlists = append(playlists, p)
	}
	s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{Status: "ok", Version: subsonicAPIVersion, Type: subsonicType, ServerVersion: subsonicServerVersion, OpenSubsonic: true, Playlists: &subsonicPlaylists{Playlist: playlists}})
}

func (s *Server) subsonicGetPlaylist(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	id := subsonicParam(r, "id")
	if id == "" {
		s.subsonicWriteError(w, r, http.StatusBadRequest, 10, "id required")
		return
	}
	var pl subsonicPlaylist
	var ownerSub, vis string
	err := s.db.QueryRow(r.Context(), `
		SELECT p.id::text, p.name, p.owner_sub, p.visibility, p.created_at::text
		FROM playlists p WHERE p.id::text=$1
	`, id).Scan(&pl.ID, &pl.Name, &ownerSub, &vis, &pl.Created)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.subsonicWriteError(w, r, http.StatusNotFound, 70, "playlist not found")
			return
		}
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "playlist lookup failed")
		return
	}
	if ownerSub != claims.Subject && vis != "public" && !auth.HasRole(claims, "admin") {
		s.subsonicWriteError(w, r, http.StatusNotFound, 70, "playlist not found")
		return
	}
	pl.Owner = ownerSub
	pl.Public = vis == "public"

	rows, err := s.db.Query(r.Context(), `
		SELECT t.id::text
		FROM playlist_tracks pt
		JOIN tracks t ON t.id=pt.track_id
		WHERE pt.playlist_id=$1
		ORDER BY pt.position, pt.added_at
	`, id)
	if err != nil {
		s.subsonicWriteError(w, r, http.StatusInternalServerError, 0, "playlist entries failed")
		return
	}
	defer rows.Close()
	entries := make([]subsonicSong, 0, 128)
	for rows.Next() {
		var trackID string
		if err := rows.Scan(&trackID); err != nil {
			continue
		}
		allowed, err := s.canAccessTrack(r.Context(), claims.Subject, auth.HasRole(claims, "admin"), trackID)
		if err != nil || !allowed {
			continue
		}
		song, err := s.subsonicGetSongByID(r.Context(), trackID)
		if err == nil {
			entries = append(entries, song)
		}
	}
	pl.Entry = entries
	pl.SongCount = len(entries)
	for _, e := range entries {
		pl.Duration += e.Duration
	}
	s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{Status: "ok", Version: subsonicAPIVersion, Type: subsonicType, ServerVersion: subsonicServerVersion, OpenSubsonic: true, Playlist: &pl})
}

func (s *Server) subsonicGetUser(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	reqUser := subsonicParam(r, "username")
	if reqUser == "" {
		reqUser = subsonicParam(r, "u")
	}
	if reqUser == "" {
		reqUser = claims.Username
	}
	isAdmin := auth.HasRole(claims, "admin")
	if !isAdmin && !strings.EqualFold(reqUser, claims.Username) {
		s.subsonicWriteError(w, r, http.StatusForbidden, 50, "forbidden")
		return
	}
	user := subsonicUser{
		Username:          claims.Username,
		Email:             claims.Email,
		AdminRole:         isAdmin,
		SettingsRole:      isAdmin,
		DownloadRole:      true,
		UploadRole:        isAdmin || auth.HasRole(claims, "member") || auth.HasRole(claims, "creator"),
		PlaylistRole:      true,
		CoverArtRole:      true,
		CommentRole:       true,
		StreamRole:        true,
		JukeboxRole:       false,
		ShareRole:         true,
		PodcastRole:       false,
		VideoConvRole:     false,
		ScrobblingEnabled: true,
	}
	s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{
		Status:        "ok",
		Version:       subsonicAPIVersion,
		Type:          subsonicType,
		ServerVersion: subsonicServerVersion,
		OpenSubsonic:  true,
		User:          &user,
	})
}

func (s *Server) subsonicScrobble(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	ids := r.URL.Query()["id"]
	for _, id := range ids {
		trackID := strings.TrimSpace(id)
		if trackID == "" {
			continue
		}
		allowed, err := s.canAccessTrack(r.Context(), claims.Subject, auth.HasRole(claims, "admin"), trackID)
		if err != nil || !allowed {
			continue
		}
		_, _ = s.db.Exec(r.Context(), `INSERT INTO play_events(track_id, user_sub) VALUES($1, $2)`, trackID, claims.Subject)
	}
	s.subsonicWriteResponse(w, r, http.StatusOK, subsonicResponse{Status: "ok", Version: subsonicAPIVersion, Type: subsonicType, ServerVersion: subsonicServerVersion, OpenSubsonic: true})
}
