# 16: Bounce and complaint handling

**What to build:** A Brevo webhook endpoint receives bounce and complaint events. A hard bounce or a complaint disables that Contact Channel, so no further Alerts go to it. Webhook requests are authenticated. Events are recorded for metrics without personal data.

**Blocked by:** 14 (Alerts by email)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] An authenticated hard-bounce event disables the matching Contact Channel
- [ ] A complaint event disables the matching Contact Channel
- [ ] Unauthenticated webhook calls are rejected
- [ ] Delivery-failure metrics count these events per channel
