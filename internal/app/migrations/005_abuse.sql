-- Rate-limit and budget counters. key is an HMAC, never a plaintext IP, contact or address.
CREATE TABLE rate_hits (
    key bytea NOT NULL,
    at  timestamptz NOT NULL
);
CREATE INDEX rate_hits_key_at ON rate_hits (key, at);
