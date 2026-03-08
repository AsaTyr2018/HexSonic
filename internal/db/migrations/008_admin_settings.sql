CREATE TABLE IF NOT EXISTS app_settings (
  key TEXT PRIMARY KEY,
  value_text TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO app_settings(key, value_text)
VALUES('registration_enabled', 'true')
ON CONFLICT (key) DO NOTHING;
