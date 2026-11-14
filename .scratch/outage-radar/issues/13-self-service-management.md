# 13: Self-service management, export, erasure

**What to build:** A Subscriber requests a management magic link to any verified Contact Channel. Through it they can add and remove Watched Addresses and Contact Channels and change language. Removing the last Contact Channel deletes the Subscriber entirely. They can also download an export of all data held about them, or delete everything at once. Requesting a link gives the same response whether or not the email is known.

**Blocked by:** 11 (Subscribe and verify by email)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] A management link works once and expires
- [ ] Watched Addresses, Contact Channels and language can be changed through the management session
- [ ] Removing the last Contact Channel deletes the Subscriber and all their rows (database inspection)
- [ ] The export contains every piece of personal data held, in a machine-readable format
- [ ] Delete-everything leaves no Subscriber data behind
- [ ] Requesting a link for an unknown email looks identical to a known one
