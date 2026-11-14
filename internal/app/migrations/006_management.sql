-- Management links share the magic_links table; a link may verify one added channel.
ALTER TABLE magic_links ADD COLUMN purpose text NOT NULL DEFAULT 'verify' CHECK (purpose IN ('verify', 'manage'));
ALTER TABLE magic_links ADD COLUMN channel_id bigint REFERENCES contact_channels ON DELETE CASCADE;
-- Set only for channels added through management, so unverified ones can be purged after 24h.
ALTER TABLE contact_channels ADD COLUMN created_at timestamptz;
