CREATE TABLE IF NOT EXISTS registration_invites (
  id BIGSERIAL PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  created_by TEXT NOT NULL,
  used_by TEXT NOT NULL DEFAULT '',
  expires_at TIMESTAMPTZ NOT NULL,
  used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_registration_invites_active
  ON registration_invites(expires_at, used_at);
