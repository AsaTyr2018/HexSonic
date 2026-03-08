CREATE TABLE IF NOT EXISTS album_comments (
  id BIGSERIAL PRIMARY KEY,
  album_id BIGINT NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
  author_sub TEXT NOT NULL,
  author_name TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_album_comments_album_created
  ON album_comments(album_id, created_at DESC);
