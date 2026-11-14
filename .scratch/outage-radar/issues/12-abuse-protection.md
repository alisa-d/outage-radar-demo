# 12: Abuse protection

**What to build:** Bots gain nothing and cost nothing. Signup requires a valid Cloudflare Turnstile token, verified on the server and stubbed in tests. Rate limits apply per IP, per Contact Channel (by blind index) and per Address, and there is a daily cap on verification attempts per Contact Channel. Unverified channels never receive Alerts. A global per-channel daily budget circuit breaker pauses new verifications when it trips, while existing verified Subscribers keep receiving Alerts. The breaker is set high for email and is ready for SMS.

**Blocked by:** 11 (Subscribe and verify by email)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] A signup without a valid Turnstile token is rejected
- [ ] Exceeding each rate limit returns a limited response and sends no email
- [ ] The daily verification cap per Contact Channel is enforced, and the clock resets it
- [ ] A tripped circuit breaker blocks new verifications but not Alerts to verified channels, and emits a metric
- [ ] Rate-limit keys contain no plaintext personal data
