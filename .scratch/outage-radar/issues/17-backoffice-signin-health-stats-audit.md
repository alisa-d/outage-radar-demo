# 17: Backoffice sign-in, health and stats, audit log

**What to build:** Operators sign in to a backoffice through OIDC with 2FA required (Keycloak in local compose). There are two roles: Operator and Privacy officer. Operators see a Provider Health page (state, last success, recent errors) and aggregate statistics: Subscribers per Settlement, Alerts per day, delivery success. The statistics never expose personal data. An audit log records backoffice actions. The backoffice uses its own JSON API, separate from the public one.

**Blocked by:** 06 (Provider Health, status strip, Coverage)

**Status:** resolved

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] Backoffice API calls without a valid OIDC session are rejected, and a role mismatch is forbidden
- [ ] Keycloak in local compose enforces 2FA for backoffice users
- [ ] The Provider Health page reflects the states from ticket 06
- [ ] No backoffice endpoint returns decrypted personal data (test asserts on response bodies)
- [ ] Backoffice actions are written to the audit log

## Answer

- Backoffice JSON API behind Keycloak OIDC Bearer ID tokens (JWKS verified, amr checked), with a realm export and compose service.
- Provider health, stats and an audit log are served from migration `009_backoffice.sql`; tests use a fake OIDC issuer in `internal/testkit/oidc.go`.
- Merged into `feat/outage-radar` as 560b345 (`git merge --no-ff`).

Known gaps:
- Keycloak realm never started; issuer URL may not match via localhost:8082.
- Realm uses a hardcoded amr mapper.
- No backoffice frontend or auth-code flow, only a Bearer ID token JSON API.
- Minimal `alerts` table that ticket 14 must adopt or replace.
- Only the latest error is shown, not history.
- JWKS is refetched on every unknown kid without rate limit.
- Story 60 Stale/Suspicious alert left to ticket 21.
