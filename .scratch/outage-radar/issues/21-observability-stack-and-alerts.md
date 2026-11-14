# 21: Observability stack and engineering alerts

**What to build:** Engineering hears about problems before residents do. Compose adds Prometheus, Grafana and Alertmanager. Alert rules notify engineering by email when a Provider is Stale or Suspicious, when Alert delivery failures spike, and when the circuit breaker trips. A Grafana dashboard shows adapter health, extraction, Alerts and delivery. A test confirms that structured logs contain no personal data across the main flows.

**Blocked by:** 06 (Provider Health, status strip, Coverage), 14 (Alerts by email)

**Status:** resolved

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [x] `docker compose up` includes Prometheus, Grafana and Alertmanager with provisioned dashboards
- [x] A simulated Stale Provider results in an engineering alert email
- [x] Delivery-failure and circuit-breaker alert rules exist and have been tested
- [x] A test scanning the logs from the Seam 1 main flows finds no emails, addresses or coordinates

## Answer

- Compose now also runs Prometheus, Alertmanager and Grafana (config and provisioned dashboard under `deploy/observability/`).
- Alert rules cover Stale/Suspicious Provider, delivery-failure spike and circuit-breaker trip, with promtool rule tests; a simulated Stale Provider produces an engineering email.
- A log-scan test over the main flows finds no emails, addresses or coordinates. Whole suite (`go vet`, `go test ./...`) and `e2e/run.sh` pass after the merge.

Merge commit: d9d8a60

Known gaps:
- Grafana dashboard panels were never viewed in a browser; only provisioning was confirmed.
- Delivery-spike and breaker alerts were tested only with promtool synthetic series, not end to end.
- Alertmanager has no production SMTP wiring, and none of Prometheus/Alertmanager/Grafana are in compose.prod.yaml.
- Alert recipient is the placeholder engineering@outage-radar.invalid.
- The spike rule counts hard and soft bounces alike.
- During local bring-up the app briefly polled the real erpsever.bg and vikvarna.com before provider URLs were pointed at an unreachable address.
- /metrics now lives on METRICS_ADDR (default :9100) and is not proxied, so prod needs Prometheus on the VM to reach it.
