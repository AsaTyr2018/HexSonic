ALTER TABLE user_profiles
  ADD COLUMN IF NOT EXISTS display_name_changed_at TIMESTAMPTZ;
