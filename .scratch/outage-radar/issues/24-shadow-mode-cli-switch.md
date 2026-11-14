# 24: Shadow Mode core with a CLI switch (demo workaround)

**What to build:** The core of ticket 19 without needing the backoffice or Keycloak. A Provider can be in Shadow Mode: its Outages are published, but the Alerts they would trigger are recorded (with a shadow flag) and never delivered. Both demo Providers are seeded in Shadow Mode. The switch is a one-off CLI command from the app image, `outage-radar provider-mode <provider> <shadow|live>`, and `outage-radar shadow-alerts [provider]` prints the would-be Alerts without personal data (Outage, Settlement, count, Match Result). Every switch writes an `audit_log` row (actor recorded as `cli`, plus the OS user if available). Switching live does not send the backlog of Alerts recorded during Shadow Mode. This is a deliberate demo workaround: ticket 19 later adds the Operator backoffice endpoints and list on top of the same data, with the same audit log.

**Blocked by:** None (tickets 14 and 17 are merged)

**Status:** resolved

Spec: `.scratch/outage-radar/spec.md` (stories 52 and 53). Vocabulary: `GLOSSARY.md`. Relates to: `19-shadow-mode.md` (remaining there: backoffice endpoints and the Operator-facing list).

- [x] Through Seam 1, a Provider in Shadow Mode creates Alert records (shadow flag) but sends no email
- [x] `shadow-alerts` lists those would-be Alerts without personal data
- [x] `provider-mode <provider> live` makes new Outages alert normally, and no Shadow Mode backlog is sent
- [x] Both demo Providers are seeded in Shadow Mode
- [x] Each switch writes an audit row, and the command works against the running compose Postgres (documented in the README in two lines)

## Answer

Provider Shadow Mode is a flag: shadow Alerts store a row but enqueue no send job, so going live sends no backlog.
`provider-mode <provider> shadow|live` flips it and writes an audit row; `shadow-alerts [provider]` lists would-be Alerts without personal data. Both demo Providers are seeded shadow (migration 013).
Quiet Hours scheduling (ticket 15) still applies to live Alerts. Verified by TestShadowModeRecordsAlertsWithoutSendingAndGoingLiveSendsNoBacklog and TestBothDemoProvidersAreSeededInShadowMode.

Merge commit: 1e05d05 on feat/outage-radar (migration renumbered 012 to 013).

Known gaps:
- After going live, an unchanged Outage that already has a shadow Alert does not alert, because dedup and material-change comparison see the shadow row; only new or materially changed Outages alert.
- Ticket 19 still owns the backoffice endpoints and Operator list.
- CLI actor is `cli` + OS user.
- CLI against compose (project t24, torn down): `docker compose run --rm app provider-mode "ЕРП Север" live` worked and set shadow=false; bogus mode errors; `shadow-alerts` printed nothing on an empty DB. The README had a wrong command (extra `outage-radar`, since the image entrypoint is already the binary); fixed.
