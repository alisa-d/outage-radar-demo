# 13: Self-service management, export, erasure

**What to build:** A Subscriber requests a management magic link to any verified Contact Channel. Through it they can add and remove Watched Addresses and Contact Channels and change language. Removing the last Contact Channel deletes the Subscriber entirely. They can also download an export of all data held about them, or delete everything at once. Requesting a link gives the same response whether or not the email is known.

**Blocked by:** 11 (Subscribe and verify by email)

**Status:** resolved

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] A management link works once and expires
- [ ] Watched Addresses, Contact Channels and language can be changed through the management session
- [ ] Removing the last Contact Channel deletes the Subscriber and all their rows (database inspection)
- [ ] The export contains every piece of personal data held, in a machine-readable format
- [ ] Delete-everything leaves no Subscriber data behind
- [ ] Requesting a link for an unknown email looks identical to a known one

## Answer

A Subscriber can request a one-time, expiring management link; the response is identical for known and unknown emails.
Through it they edit Contact Channels and language, export all their data, or delete everything; removing the last channel deletes the Subscriber.
Merged as dec4974 on feat/outage-radar (migration renumbered to 006_management.sql).

Known gaps:
- Tests were written alongside the code and never seen failing.
- /manage and /verify pages are built but never opened in a browser; no frontend tests.
- /manage cannot add a Watched Address: AddressPicker returns a street name, not a street id.
- Queued verify-email jobs are not linked to a Subscriber, so erasure leaves a pending one.
- The log-leak test does not cover management flows.
- No timing equalisation, and no CAPTCHA or rate limit on the new /api/manage endpoints.
