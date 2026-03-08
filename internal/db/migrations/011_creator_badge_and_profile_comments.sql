ALTER TABLE user_profiles
  ADD COLUMN IF NOT EXISTS creator_badge BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS user_profile_comments (
  id BIGSERIAL PRIMARY KEY,
  target_sub TEXT NOT NULL,
  author_sub TEXT NOT NULL,
  author_name TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_user_profile_comments_target_created
  ON user_profile_comments(target_sub, created_at DESC);
