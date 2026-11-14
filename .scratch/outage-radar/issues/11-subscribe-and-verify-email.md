# 11: Subscribe and verify by email

**What to build:** A resident can subscribe an Address with an email address, without creating an account (ADR 0001). The signup always returns the same neutral response, so it can't be used to discover who is subscribed. A verification email in the chosen language carries a signed, short-lived, single-use magic link. Following it verifies the Contact Channel and records consent: the time and the version of the privacy notice. Contact values and Watched Addresses are encrypted field by field using envelope encryption, with keys behind a key-provider interface. A blind index (HMAC) supports lookup and uniqueness. A Subscriber has up to 5 Watched Addresses. Unverified signups are purged after 24 hours. Email goes through the SMTP adapter (Mailpit locally, Brevo live).

**Blocked by:** 04 (Address registry and picker)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] Through Seam 1, a signup results in exactly one verification email in the SMTP sink, in the chosen language
- [ ] An expired or reused magic link is rejected
- [ ] Signing up an already-known email gives the same response and sends no extra information
- [ ] Encrypted columns hold no plaintext contact values or addresses (database inspection in the test)
- [ ] A sixth Watched Address is refused
- [ ] With the clock advanced 24h, unverified signups are purged
- [ ] No personal data appears in logs during these flows
