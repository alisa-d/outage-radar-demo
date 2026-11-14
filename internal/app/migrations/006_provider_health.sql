-- One row per Provider with an adapter; the state (healthy/stale/suspicious) is derived at read time from the clock.
CREATE TABLE provider_health (
    provider_id     int PRIMARY KEY REFERENCES providers,
    interval_s      int NOT NULL,
    first_seen_at   timestamptz NOT NULL,
    last_success_at timestamptz,
    last_error      text,
    suspicious      boolean NOT NULL DEFAULT false,
    seen_notices    boolean NOT NULL DEFAULT false -- ever returned any Notice, so an empty response is abnormal
);
