CREATE TABLE IF NOT EXISTS listening_events (
  id BIGSERIAL PRIMARY KEY,
  track_id UUID NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
  album_id BIGINT REFERENCES albums(id) ON DELETE SET NULL,
  user_sub TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  event_type TEXT NOT NULL,
  source_context TEXT NOT NULL DEFAULT '',
  playback_seconds DOUBLE PRECISION NOT NULL DEFAULT 0,
  duration_seconds DOUBLE PRECISION NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (event_type IN ('play_start', 'play_30s', 'play_50_percent', 'play_complete', 'skip_early', 'seek', 'rating', 'playlist_add'))
);

CREATE INDEX IF NOT EXISTS idx_listening_events_track_created
  ON listening_events(track_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_listening_events_user_created
  ON listening_events(user_sub, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_listening_events_type_created
  ON listening_events(event_type, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_listening_events_source_created
  ON listening_events(source_context, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_listening_events_session_type
  ON listening_events(session_id, event_type, created_at DESC);
