# 16: Bounce and complaint handling

**What to build:** A Brevo webhook endpoint receives bounce and complaint events. A hard bounce or a complaint disables that Contact Channel, so no further Alerts go to it. Webhook requests are authenticated. Events are recorded for metrics without personal data.

**Blocked by:** 14 (Alerts by email)

**Status:** resolved

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [x] An authenticated hard-bounce event disables the matching Contact Channel
- [x] A complaint event disables the matching Contact Channel
- [x] Unauthenticated webhook calls are rejected
- [x] Delivery-failure metrics count these events per channel

## Answer

- A Brevo webhook endpoint takes Bearer-token-authenticated events; hard bounces and complaints disable the Contact Channel, so no further Alerts go to it.
- Soft bounces are counted but keep the channel; unauthenticated calls (or no configured token) are rejected and change nothing.
- Events land in `delivery_events` (migration 012, no address or reason text) and show as `delivery_failures_total{channel,event}` metrics.

Merge commit: 18bb402 (`feat: bounce and complaint handling (ticket 16)`).

Known gaps:
- Never run against real Brevo or Mailpit. Payload shape, Bearer-token auth and event names (hard_bounce, invalid_email, blocked, soft_bounce, spam) are from memory of Brevo's docs and must be checked when wiring the real webhook.
- `BREVO_WEBHOOK_TOKEN` is not set in compose or deploy config; add it to .env and compose.prod.yaml.
- Suppression is not undone if an address later verifies again.
- No logging assertion for the endpoint.
