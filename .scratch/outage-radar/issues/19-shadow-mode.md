# 19: Shadow Mode

**What to build:** A Provider can be in Shadow Mode. Its Outages are published, but the Alerts they would trigger are recorded and never delivered. The backoffice shows the would-be Alerts per Provider, without personal data: Outage, Settlement, count, Match Result. An Operator can switch a Provider live, and the switch is audited. Switching live does not send a backlog of the Alerts recorded during Shadow Mode. Both demo Providers start in Shadow Mode.

**Blocked by:** 14 (Alerts by email), 17 (Backoffice sign-in, health and stats, audit log)

**Status:** resolved

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [x] Through Seam 1, a Provider in Shadow Mode creates Alert records but sends no email
- [x] The backoffice lists those would-be Alerts without personal data
- [x] After switching live, new Outages alert normally and no Shadow Mode backlog is sent
- [x] Both demo Providers are seeded in Shadow Mode
- [x] The switch is audited

## Comments

Ticket 24 delivered the Shadow Mode core (flag, no send jobs, no backlog on live, audit) and the CLI switch plus shadow-alerts list; remaining for 19: backoffice endpoints and the Operator list of would-be Alerts.

## Answer

- A Provider in Shadow Mode records would-be Alerts and sends nothing; both demo Providers are seeded in Shadow Mode (ticket 24 `shadow_test.go` covers seeding and no-send).
- The backoffice lists would-be Alerts per Provider without personal data (Outage, Settlement, count, Match Result), and an Operator can switch a Provider live with an audit entry; no backlog is sent (`internal/app/backoffice_shadow_test.go` covers the list, audit and no backlog).
- Merged in `df86c2e` (`feat: shadow mode for outage radar (ticket 19)`).

Known gaps:
- Nothing was run against compose or a real Keycloak, only the stub OIDC provider.
- A Cyrillic Provider name in the URL path is only tested via `url.PathEscape`.
- After going live, an unchanged Outage that already has a shadow Alert does not alert (ticket 24 gap).
