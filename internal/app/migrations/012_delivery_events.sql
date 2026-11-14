-- Provider delivery-failure events (bounces, complaints) for metrics. No address, no reason text: counts only.
CREATE TABLE delivery_events (
    id      bigserial PRIMARY KEY,
    at      timestamptz NOT NULL,
    channel text NOT NULL,
    event   text NOT NULL CHECK (event IN ('hard_bounce', 'soft_bounce', 'complaint'))
);
