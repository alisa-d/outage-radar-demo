# 24: Shadow Mode core with a CLI switch (demo workaround)

**What to build:** The core of ticket 19 without needing the backoffice or Keycloak. A Provider can be in Shadow Mode: its Outages are published, but the Alerts they would trigger are recorded (with a shadow flag) and never delivered. Both demo Providers are seeded in Shadow Mode. The switch is a one-off CLI command from the app image, `outage-radar provider-mode <provider> <shadow|live>`, and `outage-radar shadow-alerts [provider]` prints the would-be Alerts without personal data (Outage, Settlement, count, Match Result). Every switch writes an `audit_log` row (actor recorded as `cli`, plus the OS user if available). Switching live does not send the backlog of Alerts recorded during Shadow Mode. This is a deliberate demo workaround: ticket 19 later adds the Operator backoffice endpoints and list on top of the same data, with the same audit log.

**Blocked by:** None (tickets 14 and 17 are merged)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md` (stories 52 and 53). Vocabulary: `GLOSSARY.md`. Relates to: `19-shadow-mode.md` (remaining there: backoffice endpoints and the Operator-facing list).

- [ ] Through Seam 1, a Provider in Shadow Mode creates Alert records (shadow flag) but sends no email
- [ ] `shadow-alerts` lists those would-be Alerts without personal data
- [ ] `provider-mode <provider> live` makes new Outages alert normally, and no Shadow Mode backlog is sent
- [ ] Both demo Providers are seeded in Shadow Mode
- [ ] Each switch writes an audit row, and the command works against the running compose Postgres (documented in the README in two lines)
