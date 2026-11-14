# 14: Alerts by email

**What to build:** Subscribers are alerted when an Outage newly Affects or Possibly affects one of their Watched Addresses, or changes materially (the Match Result changes, or the time window moves by 30 minutes or more). Alerts carry a deduplication key, so the same Alert is never sent twice. Each Alert is delivered as a job per Contact Channel through the channel adapter interface, with retries and backoff. Every attempt is recorded without the message body. Templates are per language: structured fields in the Subscriber's language, original Notice in Bulgarian, Possibly affected and Disturbance worded honestly. Every email carries the RFC 8058 `List-Unsubscribe` and `List-Unsubscribe-Post` headers and a footer link that removes that Contact Channel. Unlocated Outages create no Alerts. Cancellations are not alerted.

**Blocked by:** 07 (Address check for street-listed Outages), 11 (Subscribe and verify by email)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] Through Seam 1, a new Outage covering a Watched Address produces exactly one email in the SMTP sink
- [ ] Re-ingesting the same Notice produces no further email
- [ ] A time shift of 30 minutes or more, or a change of Match Result, produces one new Alert; a smaller shift produces none
- [ ] Possibly affected and Disturbance Alerts use their own wording, in bg and en
- [ ] A one-click unsubscribe POST disables that Contact Channel immediately
- [ ] A transient SMTP failure is retried, and delivery attempts are recorded without bodies
- [ ] An Unlocated Outage produces no Alert
