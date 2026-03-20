ALTER TABLE user_profiles
  ADD COLUMN IF NOT EXISTS status_line TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS accent_color TEXT NOT NULL DEFAULT '#2d78dd',
  ADD COLUMN IF NOT EXISTS profile_role TEXT NOT NULL DEFAULT 'listener',
  ADD COLUMN IF NOT EXISTS featured_album_id BIGINT REFERENCES albums(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS featured_playlist_id BIGINT REFERENCES playlists(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS guest_show_followers BOOLEAN NOT NULL DEFAULT true,
  ADD COLUMN IF NOT EXISTS guest_show_playlists BOOLEAN NOT NULL DEFAULT true,
  ADD COLUMN IF NOT EXISTS guest_show_favorites BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS guest_show_stats BOOLEAN NOT NULL DEFAULT true,
  ADD COLUMN IF NOT EXISTS guest_show_uploads BOOLEAN NOT NULL DEFAULT true;

UPDATE user_profiles
SET profile_role = CASE WHEN creator_badge THEN 'creator' ELSE 'listener' END
WHERE COALESCE(profile_role, '') = '' OR profile_role NOT IN ('listener', 'creator');

CREATE TABLE IF NOT EXISTS favorites (
  id BIGSERIAL PRIMARY KEY,
  user_sub TEXT NOT NULL,
  target_kind TEXT NOT NULL,
  track_id UUID REFERENCES tracks(id) ON DELETE CASCADE,
  album_id BIGINT REFERENCES albums(id) ON DELETE CASCADE,
  playlist_id BIGINT REFERENCES playlists(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (target_kind IN ('track', 'album', 'playlist')),
  CHECK (
    (CASE WHEN track_id IS NULL THEN 0 ELSE 1 END) +
    (CASE WHEN album_id IS NULL THEN 0 ELSE 1 END) +
    (CASE WHEN playlist_id IS NULL THEN 0 ELSE 1 END) = 1
  )
);

CREATE INDEX IF NOT EXISTS idx_favorites_user_created ON favorites(user_sub, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_favorites_track ON favorites(user_sub, track_id) WHERE target_kind='track' AND track_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_favorites_album ON favorites(user_sub, album_id) WHERE target_kind='album' AND album_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_favorites_playlist ON favorites(user_sub, playlist_id) WHERE target_kind='playlist' AND playlist_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_favorites_track ON favorites(track_id) WHERE track_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_favorites_album ON favorites(album_id) WHERE album_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_favorites_playlist ON favorites(playlist_id) WHERE playlist_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS notifications (
  id BIGSERIAL PRIMARY KEY,
  user_sub TEXT NOT NULL,
  actor_sub TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL,
  album_id BIGINT REFERENCES albums(id) ON DELETE SET NULL,
  track_id UUID REFERENCES tracks(id) ON DELETE SET NULL,
  title TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL DEFAULT '',
  is_read BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (user_sub, kind, actor_sub, album_id)
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_created ON notifications(user_sub, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_user_unread ON notifications(user_sub, is_read, created_at DESC);
