CREATE TABLE IF NOT EXISTS stream_play_tokens (
  token_hash TEXT PRIMARY KEY,
  track_id UUID NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
  user_sub TEXT NOT NULL DEFAULT '',
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  played_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_stream_play_tokens_expires ON stream_play_tokens(expires_at);
CREATE INDEX IF NOT EXISTS idx_stream_play_tokens_track ON stream_play_tokens(track_id);

CREATE TABLE IF NOT EXISTS play_events (
  id BIGSERIAL PRIMARY KEY,
  track_id UUID NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
  album_id BIGINT REFERENCES albums(id) ON DELETE SET NULL,
  user_sub TEXT NOT NULL DEFAULT '',
  played_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_play_events_track ON play_events(track_id, played_at DESC);
CREATE INDEX IF NOT EXISTS idx_play_events_album ON play_events(album_id, played_at DESC);
CREATE INDEX IF NOT EXISTS idx_play_events_user ON play_events(user_sub, played_at DESC);
