ALTER TABLE tracks
  ADD COLUMN IF NOT EXISTS track_number INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_tracks_album_track_number
  ON tracks(album_id, track_number, title);
