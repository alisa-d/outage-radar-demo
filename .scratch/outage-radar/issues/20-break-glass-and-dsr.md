# 20: Break-glass and data-subject requests

**What to build:** A Privacy officer can open one Subscriber's personal data through break-glass access, which requires a stated reason and expires after a time limit. Every use is written to the audit log. A Privacy officer can also run an export or an erasure on a Subscriber's behalf. Operators can do neither.

**Blocked by:** 13 (Self-service management, export, erasure), 17 (Backoffice sign-in, health and stats, audit log)

**Status:** resolved

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [x] Break-glass access requires a reason, and access ends when the time limit passes (tested with the clock)
- [x] Each break-glass access and data-subject action is audited with the Privacy officer, reason and time
- [x] An Operator cannot perform break-glass access or data-subject requests
- [x] An export or erasure run on someone's behalf gives the same result as the self-service path

## Answer

- A Privacy officer opens break-glass with a stated reason; the grant expires after 15 minutes (clock-tested) and each open and view is audited with officer, reason and time (`TestBreakGlassNeedsAReasonExpiresAndIsAudited`).
- A Privacy officer can export or erase a Subscriber on their behalf with a reason; the export equals the self-service export, erasure leaves nothing behind, and both are audited (`TestPrivacyOfficerExportAndErasureMatchSelfServiceAndAreAudited`). Operators get 403 on all of it.
- Migration `014_break_glass.sql`; code in `internal/app/privacy.go` and `internal/app/backoffice.go`. Merged in `ac365d1`.

Known gaps:
- Erasure still leaves a pending queued verify-email job, because jobs are not linked to a Subscriber.
- No log-leak test for the break-glass or data-subject flows.
- Nothing stops one officer's grant being used by another beyond per-officer-name checks, and there is no rate limit on these endpoints.
- DSR export and erase need only a reason, not an open grant.
- No frontend.
- Never run against Keycloak, only the stub OIDC provider.
- The DSR code was written together with break-glass, not strictly one test at a time.
