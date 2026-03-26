ALTER TABLE jukebox_session_tracks
  DROP CONSTRAINT IF EXISTS jukebox_session_tracks_session_id_position_key;

DROP INDEX IF EXISTS idx_jukebox_session_tracks_session_position;

CREATE INDEX IF NOT EXISTS idx_jukebox_session_tracks_unplayed_order
  ON jukebox_session_tracks(session_id, played_at, created_at, id);
