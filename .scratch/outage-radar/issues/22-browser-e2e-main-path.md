# 22: Browser end-to-end tests of the main path

**What to build:** A small set of browser end-to-end tests run against the composed stack, with Mailpit capturing mail. They cover: the Region page → address check → subscribe → open the verification email in Mailpit → verify → manage Watched Addresses → unsubscribe from an Alert email. This runs in CI. Everything else stays covered at the API.

**Blocked by:** 13 (Self-service management, export, erasure), 15 (Quiet Hours and urgency)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] The end-to-end suite runs against `docker compose` with stubbed providers and Mailpit
- [ ] It covers every step of the main path listed above, in both bg and en for at least one step
- [ ] It runs in CI and fails the build on regression
