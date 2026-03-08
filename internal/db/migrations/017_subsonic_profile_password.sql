ALTER TABLE user_profiles
  ADD COLUMN IF NOT EXISTS subsonic_password TEXT NOT NULL DEFAULT '';
