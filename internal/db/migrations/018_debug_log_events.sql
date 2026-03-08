CREATE TABLE IF NOT EXISTS debug_log_events (
  id BIGSERIAL PRIMARY KEY,
  event_type TEXT NOT NULL DEFAULT '',
  endpoint TEXT NOT NULL DEFAULT '',
  http_method TEXT NOT NULL DEFAULT '',
  status_code INT NOT NULL DEFAULT 0,
  actor_sub TEXT NOT NULL DEFAULT '',
  client_ip TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  details JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_debug_log_events_created
  ON debug_log_events(created_at DESC);

INSERT INTO app_settings(key, value_text)
VALUES('debug_logging_enabled', 'false')
ON CONFLICT (key) DO NOTHING;
