ALTER TABLE registration_invites
  ADD COLUMN IF NOT EXISTS token_plain TEXT NOT NULL DEFAULT '';
