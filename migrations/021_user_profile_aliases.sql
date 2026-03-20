CREATE TABLE IF NOT EXISTS user_profile_aliases (
  id BIGSERIAL PRIMARY KEY,
  user_sub TEXT NOT NULL,
  alias TEXT NOT NULL,
  alias_lookup TEXT NOT NULL,
  is_primary BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_user_profile_aliases_lookup
  ON user_profile_aliases(alias_lookup);

CREATE INDEX IF NOT EXISTS idx_user_profile_aliases_user_sub
  ON user_profile_aliases(user_sub, updated_at DESC);

INSERT INTO user_profile_aliases(user_sub, alias, alias_lookup, is_primary, created_at, updated_at)
SELECT DISTINCT ON (lower(trim(display_name)))
  user_sub,
  trim(display_name) AS alias,
  lower(trim(display_name)) AS alias_lookup,
  true,
  now(),
  now()
FROM user_profiles
WHERE trim(display_name) <> ''
ORDER BY lower(trim(display_name)), updated_at DESC, user_sub
ON CONFLICT (alias_lookup) DO NOTHING;
