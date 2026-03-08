package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Store struct {
	Root string
}

func New(root string) (*Store, error) {
	s := &Store{Root: root}
	for _, p := range []string{
		s.OriginalsDir(),
		s.DerivedDir(),
		s.CoversDir(),
		s.UserAvatarsDir(),
		s.TempDir(),
	} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", p, err)
		}
	}
	return s, nil
}

func (s *Store) OriginalsDir() string { return filepath.Join(s.Root, "originals") }
func (s *Store) DerivedDir() string   { return filepath.Join(s.Root, "derived") }
func (s *Store) CoversDir() string    { return filepath.Join(s.Root, "covers") }
func (s *Store) UserAvatarsDir() string {
	return filepath.Join(s.Root, "users")
}
func (s *Store) TempDir() string      { return filepath.Join(s.Root, "temp") }

func (s *Store) OriginalsPath(hash, ext string) string {
	h := strings.ToLower(hash)
	if len(h) < 4 {
		h = h + strings.Repeat("0", 4-len(h))
	}
	cleanExt := strings.TrimPrefix(ext, ".")
	if cleanExt == "" {
		cleanExt = "bin"
	}
	return filepath.Join(s.OriginalsDir(), h[0:2], h[2:4], h+"."+cleanExt)
}

func (s *Store) DerivedTrackDir(trackID string) string {
	return filepath.Join(s.DerivedDir(), trackID)
}

func (s *Store) AlbumCoverPath(albumID int64) string {
	return filepath.Join(s.CoversDir(), fmt.Sprintf("album-%d.jpg", albumID))
}

func (s *Store) AlbumCoverPathWithExt(albumID int64, ext string) string {
	cleanExt := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
	if cleanExt == "" {
		cleanExt = "jpg"
	}
	return filepath.Join(s.CoversDir(), fmt.Sprintf("album-%d.%s", albumID, cleanExt))
}

func (s *Store) UserAvatarPathWithExt(userSub, ext string) string {
	cleanExt := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
	if cleanExt == "" {
		cleanExt = "jpg"
	}
	safeSub := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_").Replace(strings.TrimSpace(userSub))
	if safeSub == "" {
		safeSub = "unknown"
	}
	return filepath.Join(s.UserAvatarsDir(), fmt.Sprintf("avatar-%s.%s", safeSub, cleanExt))
}
