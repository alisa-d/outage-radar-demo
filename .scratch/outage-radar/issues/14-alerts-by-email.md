# 14: Alerts by email

**What to build:** Subscribers are alerted when an Outage newly Affects or Possibly affects one of their Watched Addresses, or changes materially (the Match Result changes, or the time window moves by 30 minutes or more). Alerts carry a deduplication key, so the same Alert is never sent twice. Each Alert is delivered as a job per Contact Channel through the channel adapter interface, with retries and backoff. Every attempt is recorded without the message body. Templates are per language: structured fields in the Subscriber's language, original Notice in Bulgarian, Possibly affected and Disturbance worded honestly. Every email carries the RFC 8058 `List-Unsubscribe` and `List-Unsubscribe-Post` headers and a footer link that removes that Contact Channel. Unlocated Outages create no Alerts. Cancellations are not alerted.

**Blocked by:** 07 (Address check for street-listed Outages), 11 (Subscribe and verify by email)

**Status:** resolved

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [x] Through Seam 1, a new Outage covering a Watched Address produces exactly one email in the SMTP sink
- [x] Re-ingesting the same Notice produces no further email
- [x] A time shift of 30 minutes or more, or a change of Match Result, produces one new Alert; a smaller shift produces none
- [x] Possibly affected and Disturbance Alerts use their own wording, in bg and en
- [x] A one-click unsubscribe POST disables that Contact Channel immediately
- [x] A transient SMTP failure is retried, and delivery attempts are recorded without bodies
- [x] An Unlocated Outage produces no Alert

## Answer

- Subscribers get one email per Outage that newly Affects or Possibly affects a Watched Address, or changes materially (Match Result change, or window shift of 30 minutes or more); a deduplication key prevents repeats.
- Delivery is a job per Contact Channel via the channel adapter, with retries and backoff; attempts are recorded without bodies; bg/en templates, honest Possibly affected and Disturbance wording, RFC 8058 headers and a footer /unsubscribe link.
- Unlocated Outages and cancellations create no Alerts.

Merge commit: c63db77 (merge of ticket 14 into feat/outage-radar).

Known gaps:
- No Quiet Hours or urgency (ticket 15).
- No Shadow Mode (ticket 19), so every Outage alerts for real.
- Unlocated release left to 08/18.
- A Watched Address added after an Outage was ingested gets no Alert for it.
- Outage identity is Notice id plus position within the Notice; the Alert shows current Notice text, not a snapshot.
- Several tests were written after the code and never seen failing (unsubscribe, retry, wording, Unlocated, unverified).
- Only the in-test SMTP sink was used, never Mailpit or Brevo.
- The /unsubscribe page was never opened in a browser.
- Each scan decrypts every verified Subscriber's addresses; fine for demo only.
