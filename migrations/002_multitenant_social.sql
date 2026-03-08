ALTER TABLE tracks ADD COLUMN IF NOT EXISTS owner_sub TEXT NOT NULL DEFAULT 'local-dev';

CREATE INDEX IF NOT EXISTS idx_tracks_owner_sub ON tracks(owner_sub);

CREATE TABLE IF NOT EXISTS follows (
  follower_sub TEXT NOT NULL,
  followed_sub TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(follower_sub, followed_sub),
  CHECK (follower_sub <> followed_sub)
);

CREATE TABLE IF NOT EXISTS comments (
  id BIGSERIAL PRIMARY KEY,
  track_id UUID NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
  author_sub TEXT NOT NULL,
  author_name TEXT,
  content TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_comments_track_created ON comments(track_id, created_at DESC);

CREATE TABLE IF NOT EXISTS ratings (
  track_id UUID NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
  author_sub TEXT NOT NULL,
  rating SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(track_id, author_sub)
);

CREATE TABLE IF NOT EXISTS audit_logs (
  id BIGSERIAL PRIMARY KEY,
  actor_sub TEXT NOT NULL,
  action TEXT NOT NULL,
  target_type TEXT NOT NULL,
  target_id TEXT NOT NULL,
  details JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs(created_at DESC);
