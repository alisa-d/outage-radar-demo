# 12: Abuse protection

**What to build:** Bots gain nothing and cost nothing. Signup requires a valid Cloudflare Turnstile token, verified on the server and stubbed in tests. Rate limits apply per IP, per Contact Channel (by blind index) and per Address, and there is a daily cap on verification attempts per Contact Channel. Unverified channels never receive Alerts. A global per-channel daily budget circuit breaker pauses new verifications when it trips, while existing verified Subscribers keep receiving Alerts. The breaker is set high for email and is ready for SMS.

**Blocked by:** 11 (Subscribe and verify by email)

**Status:** resolved

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] A signup without a valid Turnstile token is rejected
- [ ] Exceeding each rate limit returns a limited response and sends no email
- [ ] The daily verification cap per Contact Channel is enforced, and the clock resets it
- [ ] A tripped circuit breaker blocks new verifications but not Alerts to verified channels, and emits a metric
- [ ] Rate-limit keys contain no plaintext personal data

## Answer

Signup now requires a Turnstile token (stubbed verifier), with per-IP, per-channel (blind index) and per-address rate limits.
A daily verification cap per channel and a global daily circuit breaker pause new verifications and bump a metric; keys hold no plaintext.
Merged as 3a2b758 on feat/outage-radar (migration 005_abuse.sql).

Known gaps:
- Alert-related checkboxes ("unverified channels never receive Alerts", "breaker doesn't block Alerts to verified channels") are untested because Alerts don't exist yet; ticket 14 must cover them.
- Client IP is the TCP peer, so behind a proxy all requests share one IP.
- Limit checks are check-then-insert and can overshoot.
- Real Turnstile is never called.
- /metrics is a hand-rolled single counter, not OpenTelemetry (ticket 21 should absorb it).
- Frontend does not send a Turnstile token yet.
