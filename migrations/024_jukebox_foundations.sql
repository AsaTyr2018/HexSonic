ALTER TABLE user_profiles
  ADD COLUMN IF NOT EXISTS jukebox_preferred_genres TEXT[] NOT NULL DEFAULT '{}'::TEXT[];

CREATE TABLE IF NOT EXISTS jukebox_sessions (
  id UUID PRIMARY KEY,
  user_sub TEXT NOT NULL,
  mode TEXT NOT NULL,
  seed_genre TEXT NOT NULL DEFAULT '',
  seed_creator_sub TEXT NOT NULL DEFAULT '',
  seed_album_id BIGINT REFERENCES albums(id) ON DELETE SET NULL,
  options_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_activity_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (mode IN ('for_you', 'genre', 'creator', 'album', 'try_me')),
  CHECK (status IN ('active', 'ended'))
);

CREATE INDEX IF NOT EXISTS idx_jukebox_sessions_user_activity
  ON jukebox_sessions(user_sub, last_activity_at DESC);

CREATE TABLE IF NOT EXISTS jukebox_session_tracks (
  id BIGSERIAL PRIMARY KEY,
  session_id UUID NOT NULL REFERENCES jukebox_sessions(id) ON DELETE CASCADE,
  position INT NOT NULL,
  track_id UUID NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
  score DOUBLE PRECISION NOT NULL DEFAULT 0,
  reason TEXT NOT NULL DEFAULT '',
  played_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(session_id, position),
  UNIQUE(session_id, track_id)
);

CREATE INDEX IF NOT EXISTS idx_jukebox_session_tracks_session_position
  ON jukebox_session_tracks(session_id, position);

CREATE INDEX IF NOT EXISTS idx_jukebox_session_tracks_track_created
  ON jukebox_session_tracks(track_id, created_at DESC);

CREATE TABLE IF NOT EXISTS jukebox_feedback_events (
  id BIGSERIAL PRIMARY KEY,
  session_id UUID NOT NULL REFERENCES jukebox_sessions(id) ON DELETE CASCADE,
  user_sub TEXT NOT NULL,
  track_id UUID REFERENCES tracks(id) ON DELETE SET NULL,
  action TEXT NOT NULL,
  payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (action IN ('more_like_this', 'less_like_this', 'stay_in_genre', 'surprise_me', 'skip'))
);

CREATE INDEX IF NOT EXISTS idx_jukebox_feedback_session_created
  ON jukebox_feedback_events(session_id, created_at DESC);

INSERT INTO app_settings(key, value_text)
VALUES
  ('jukebox_max_track_plays_per_hour', '1'),
  ('jukebox_max_creator_tracks_per_hour', '12')
ON CONFLICT (key) DO NOTHING;
