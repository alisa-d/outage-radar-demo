# outage-radar-demo
Demo project for a talk on spec-driven development with AI agents: an outage alert service for water and power, built end to end with Claude Code

## Deploy

`deploy/wizard.sh` walks the human steps (old demo, backup bucket, Brevo, DNS) and writes `.env`. The stack runs from `compose.prod.yaml` behind Caddy; `deploy/backup.sh` makes the nightly encrypted dump and `deploy/restore-test.sh` proves a restore locally.

## Shadow Mode (demo switch)

Both demo Providers start in Shadow Mode (Alerts recorded, never sent). From the app image against the compose Postgres: `docker compose run --rm app provider-mode "ЕРП Север" live` (or `shadow`), and `... shadow-alerts ["ЕРП Север"]` lists the would-be Alerts without personal data.
Every switch writes an `audit_log` row (actor `cli (<os user>)`); going live sends no backlog.

## Observability

`docker compose up` also starts Prometheus (:9090), Alertmanager (:9093) and Grafana (:3000, dashboard "Outage Radar"). Every role serves `/metrics` on `METRICS_ADDR` (default `:9100`, never on the public listener). Alert rules (Provider Stale or Suspicious, delivery-failure spike, circuit breaker, scrape down) live in `deploy/observability/rules.yml`; check them with `promtool test rules deploy/observability/rules_test.yml`. Locally Alertmanager emails engineering into Mailpit; production needs a real SMTP smarthost and is not wired in `compose.prod.yaml` yet.

## Browser end-to-end test

`e2e/run.sh` composes the stack (Mailpit, stubbed siteverify, notify worker, no Provider ingest), loads `e2e/fixture` and `e2e/seed.sql` (test-only), and runs the Playwright main-path test (Region page to unsubscribe). Needs Docker and Node; env `PROJECT`, `FRONTEND_PORT`, `MAILPIT_PORT`. CI: `.github/workflows/e2e.yml`.
