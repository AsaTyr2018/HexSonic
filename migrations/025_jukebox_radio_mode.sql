ALTER TABLE jukebox_sessions
  DROP CONSTRAINT IF EXISTS jukebox_sessions_mode_check;

ALTER TABLE jukebox_sessions
  ADD CONSTRAINT jukebox_sessions_mode_check
  CHECK (mode IN ('for_you', 'radio', 'genre', 'creator', 'album', 'try_me'));
