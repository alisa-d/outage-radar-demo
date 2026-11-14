-- A held Alert whose Outage ended or was cancelled before release is recorded as skipped, not sent.
ALTER TABLE delivery_attempts DROP CONSTRAINT delivery_attempts_outcome_check;
ALTER TABLE delivery_attempts ADD CONSTRAINT delivery_attempts_outcome_check CHECK (outcome IN ('sent', 'failed', 'skipped'));
