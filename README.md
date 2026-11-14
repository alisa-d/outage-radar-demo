# outage-radar-demo
Demo project for a talk on spec-driven development with AI agents: an outage alert service for water and power, built end to end with Claude Code

## Deploy

`deploy/wizard.sh` walks the human steps (old demo, backup bucket, Brevo, DNS) and writes `.env`. The stack runs from `compose.prod.yaml` behind Caddy; `deploy/backup.sh` makes the nightly encrypted dump and `deploy/restore-test.sh` proves a restore locally.

## Shadow Mode (demo switch)

Both demo Providers start in Shadow Mode (Alerts recorded, never sent). From the app image against the compose Postgres: `docker compose run --rm app outage-radar provider-mode "ЕРП Север" live` (or `shadow`), and `... outage-radar shadow-alerts ["ЕРП Север"]` lists the would-be Alerts without personal data.
Every switch writes an `audit_log` row (actor `cli (<os user>)`); going live sends no backlog.
