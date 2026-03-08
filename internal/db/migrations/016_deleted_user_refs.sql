CREATE TABLE IF NOT EXISTS deleted_user_refs (
  original_sub TEXT PRIMARY KEY,
  dummy_sub TEXT NOT NULL UNIQUE,
  deleted_by TEXT NOT NULL DEFAULT '',
  deleted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  restored_to_sub TEXT NOT NULL DEFAULT '',
  restored_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_deleted_user_refs_dummy_sub
  ON deleted_user_refs(dummy_sub);
