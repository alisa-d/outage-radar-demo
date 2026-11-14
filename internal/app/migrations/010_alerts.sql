-- Adopts the minimal alerts table from 009: the new columns are nullable so statistics rows without them still work.
ALTER TABLE alerts ADD COLUMN dedup_key         text UNIQUE;
ALTER TABLE alerts ADD COLUMN subscriber_id     bigint REFERENCES subscribers ON DELETE CASCADE;
ALTER TABLE alerts ADD COLUMN watched_address_id bigint REFERENCES watched_addresses ON DELETE CASCADE;
ALTER TABLE alerts ADD COLUMN notice_id         bigint REFERENCES notices ON DELETE CASCADE;
ALTER TABLE alerts ADD COLUMN ordinal           int;  -- the Outage's position within its Notice, its identity across re-ingests
ALTER TABLE alerts ADD COLUMN match_result      text;
ALTER TABLE alerts ADD COLUMN severity          text;
ALTER TABLE alerts ADD COLUMN start_at          timestamptz;
ALTER TABLE alerts ADD COLUMN end_at            timestamptz;
CREATE INDEX alerts_latest ON alerts (notice_id, ordinal, watched_address_id, id);

-- One row per try, never the message body.
CREATE TABLE delivery_attempts (
    id         bigserial PRIMARY KEY,
    alert_id   bigint NOT NULL REFERENCES alerts ON DELETE CASCADE,
    channel_id bigint NOT NULL REFERENCES contact_channels ON DELETE CASCADE,
    at         timestamptz NOT NULL,
    outcome    text NOT NULL CHECK (outcome IN ('sent', 'failed'))
);
