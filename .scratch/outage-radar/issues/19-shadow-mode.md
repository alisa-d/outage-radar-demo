# 19: Shadow Mode

**What to build:** A Provider can be in Shadow Mode. Its Outages are published, but the Alerts they would trigger are recorded and never delivered. The backoffice shows the would-be Alerts per Provider, without personal data: Outage, Settlement, count, Match Result. An Operator can switch a Provider live, and the switch is audited. Switching live does not send a backlog of the Alerts recorded during Shadow Mode. Both demo Providers start in Shadow Mode.

**Blocked by:** 14 (Alerts by email), 17 (Backoffice sign-in, health and stats, audit log)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] Through Seam 1, a Provider in Shadow Mode creates Alert records but sends no email
- [ ] The backoffice lists those would-be Alerts without personal data
- [ ] After switching live, new Outages alert normally and no Shadow Mode backlog is sent
- [ ] Both demo Providers are seeded in Shadow Mode
- [ ] The switch is audited
