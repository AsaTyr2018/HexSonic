ALTER TABLE user_profiles
  ADD COLUMN IF NOT EXISTS banner_path TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS featured_album_ids BIGINT[] NOT NULL DEFAULT '{}'::BIGINT[];

UPDATE user_profiles
SET featured_album_ids = CASE
  WHEN featured_album_id IS NOT NULL AND (featured_album_ids IS NULL OR cardinality(featured_album_ids)=0)
    THEN ARRAY[featured_album_id]
  WHEN featured_album_ids IS NULL
    THEN '{}'::BIGINT[]
  ELSE featured_album_ids
END;
