-- Personal-data tables (ADR 0001). *_enc columns are envelope-encrypted; blind_idx is an HMAC.
CREATE TABLE subscribers (
    id         bigserial PRIMARY KEY,
    language   text NOT NULL CHECK (language IN ('bg', 'en')),
    created_at timestamptz NOT NULL
);

CREATE TABLE contact_channels (
    id            bigserial PRIMARY KEY,
    subscriber_id bigint NOT NULL REFERENCES subscribers ON DELETE CASCADE,
    kind          text NOT NULL CHECK (kind IN ('email')),
    value_enc     bytea NOT NULL,
    blind_idx     bytea NOT NULL UNIQUE,
    verified_at   timestamptz,
    disabled_at   timestamptz
);

CREATE TABLE watched_addresses (
    id            bigserial PRIMARY KEY,
    subscriber_id bigint NOT NULL REFERENCES subscribers ON DELETE CASCADE,
    data_enc      bytea NOT NULL -- street id, number, coordinates
);

CREATE TABLE consents (
    id             bigserial PRIMARY KEY,
    subscriber_id  bigint NOT NULL REFERENCES subscribers ON DELETE CASCADE,
    at             timestamptz NOT NULL,
    notice_version text NOT NULL
);

CREATE TABLE magic_links (
    id            text PRIMARY KEY,
    subscriber_id bigint NOT NULL REFERENCES subscribers ON DELETE CASCADE,
    expires_at    timestamptz NOT NULL,
    used_at       timestamptz
);
