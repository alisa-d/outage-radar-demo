# 20: Break-glass and data-subject requests

**What to build:** A Privacy officer can open one Subscriber's personal data through break-glass access, which requires a stated reason and expires after a time limit. Every use is written to the audit log. A Privacy officer can also run an export or an erasure on a Subscriber's behalf. Operators can do neither.

**Blocked by:** 13 (Self-service management, export, erasure), 17 (Backoffice sign-in, health and stats, audit log)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] Break-glass access requires a reason, and access ends when the time limit passes (tested with the clock)
- [ ] Each break-glass access and data-subject action is audited with the Privacy officer, reason and time
- [ ] An Operator cannot perform break-glass access or data-subject requests
- [ ] An export or erasure run on someone's behalf gives the same result as the self-service path
