CREATE TABLE notices (
    id           bigserial PRIMARY KEY,
    provider_id  int NOT NULL REFERENCES providers,
    external_id  text NOT NULL,
    text         text NOT NULL,
    content_hash text NOT NULL,
    published_at timestamptz,
    UNIQUE (provider_id, external_id)
);

CREATE TABLE outages (
    id              bigserial PRIMARY KEY,
    notice_id       bigint NOT NULL REFERENCES notices ON DELETE CASCADE,
    provider_id     int NOT NULL REFERENCES providers,
    kind            text NOT NULL CHECK (kind IN ('planned', 'emergency')),
    severity        text NOT NULL CHECK (severity IN ('cut', 'disturbance')),
    start_at        timestamptz NOT NULL,
    end_at          timestamptz NOT NULL,
    end_approximate boolean NOT NULL DEFAULT false,
    settlement      text NOT NULL,
    places          text[] NOT NULL DEFAULT '{}'
);
CREATE INDEX outages_start ON outages (start_at);
CREATE INDEX outages_notice ON outages (notice_id);
