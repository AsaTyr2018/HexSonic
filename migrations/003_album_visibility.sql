ALTER TABLE albums ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'private';
ALTER TABLE albums ADD COLUMN IF NOT EXISTS owner_sub TEXT NOT NULL DEFAULT 'local-dev';

DO $$ BEGIN
  ALTER TABLE albums ADD CONSTRAINT albums_visibility_check CHECK (visibility IN ('private', 'public'));
EXCEPTION
  WHEN duplicate_object THEN NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_albums_visibility ON albums(visibility);
CREATE INDEX IF NOT EXISTS idx_albums_owner_sub ON albums(owner_sub);

UPDATE albums a
SET visibility = 'public'
WHERE EXISTS (
  SELECT 1 FROM tracks t
  WHERE t.album_id = a.id AND t.visibility = 'public'
);
