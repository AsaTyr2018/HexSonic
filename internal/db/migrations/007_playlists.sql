CREATE TABLE IF NOT EXISTS playlists (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('public', 'private')),
  owner_sub TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_playlists_owner_sub ON playlists(owner_sub);
CREATE INDEX IF NOT EXISTS idx_playlists_visibility ON playlists(visibility);

CREATE TABLE IF NOT EXISTS playlist_tracks (
  playlist_id BIGINT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
  track_id UUID NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
  position INTEGER NOT NULL DEFAULT 1,
  added_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (playlist_id, track_id)
);

CREATE INDEX IF NOT EXISTS idx_playlist_tracks_playlist_pos ON playlist_tracks(playlist_id, position, added_at);
