-- Break-glass: a Privacy officer's time-limited, reasoned access to one Subscriber. Erasing the Subscriber ends the grant.
CREATE TABLE break_glass_grants (
    id            text PRIMARY KEY,
    officer       text NOT NULL,
    subscriber_id bigint NOT NULL REFERENCES subscribers ON DELETE CASCADE,
    reason        text NOT NULL,
    expires_at    timestamptz NOT NULL
);
-- The stated reason for a break-glass access or data-subject request; empty for ordinary actions.
ALTER TABLE audit_log ADD COLUMN reason text NOT NULL DEFAULT '';
