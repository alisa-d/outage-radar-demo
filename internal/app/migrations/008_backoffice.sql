-- Alerts are produced by a later ticket; the backoffice statistics read their count and delivery outcome.
CREATE TABLE alerts (
    id         bigserial PRIMARY KEY,
    created_at timestamptz NOT NULL,
    delivery   text NOT NULL CHECK (delivery IN ('pending', 'delivered', 'failed'))
);

-- Backoffice actions. No subscriber data: actor is the Operator's OIDC username.
CREATE TABLE audit_log (
    id     bigserial PRIMARY KEY,
    at     timestamptz NOT NULL,
    actor  text NOT NULL,
    action text NOT NULL
);
