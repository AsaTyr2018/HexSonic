CREATE TABLE IF NOT EXISTS artists (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS albums (
  id BIGSERIAL PRIMARY KEY,
  artist_id BIGINT REFERENCES artists(id) ON DELETE SET NULL,
  title TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(artist_id, title)
);

CREATE TABLE IF NOT EXISTS tracks (
  id UUID PRIMARY KEY,
  title TEXT NOT NULL,
  artist_id BIGINT REFERENCES artists(id) ON DELETE SET NULL,
  album_id BIGINT REFERENCES albums(id) ON DELETE SET NULL,
  duration_seconds DOUBLE PRECISION NOT NULL DEFAULT 0,
  fingerprint TEXT,
  visibility TEXT NOT NULL DEFAULT 'private',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (visibility IN ('private', 'unlisted', 'followers_only', 'public'))
);

CREATE TABLE IF NOT EXISTS track_files (
  id UUID PRIMARY KEY,
  track_id UUID NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
  file_hash TEXT NOT NULL UNIQUE,
  file_path TEXT NOT NULL,
  codec TEXT,
  bitrate INTEGER,
  sample_rate INTEGER,
  channels INTEGER,
  size_bytes BIGINT NOT NULL,
  is_original BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_track_files_track_id ON track_files(track_id);
CREATE INDEX IF NOT EXISTS idx_tracks_visibility ON tracks(visibility);

CREATE TABLE IF NOT EXISTS transcode_jobs (
  id BIGSERIAL PRIMARY KEY,
  track_id UUID NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
  source_file_id UUID NOT NULL REFERENCES track_files(id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'queued',
  error_text TEXT,
  attempts INTEGER NOT NULL DEFAULT 0,
  locked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (status IN ('queued', 'processing', 'done', 'failed')),
  UNIQUE(track_id, source_file_id)
);

CREATE INDEX IF NOT EXISTS idx_transcode_jobs_status_created ON transcode_jobs(status, created_at);
