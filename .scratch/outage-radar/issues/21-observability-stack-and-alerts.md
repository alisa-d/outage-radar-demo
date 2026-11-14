# 21: Observability stack and engineering alerts

**What to build:** Engineering hears about problems before residents do. Compose adds Prometheus, Grafana and Alertmanager. Alert rules notify engineering by email when a Provider is Stale or Suspicious, when Alert delivery failures spike, and when the circuit breaker trips. A Grafana dashboard shows adapter health, extraction, Alerts and delivery. A test confirms that structured logs contain no personal data across the main flows.

**Blocked by:** 06 (Provider Health, status strip, Coverage), 14 (Alerts by email)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] `docker compose up` includes Prometheus, Grafana and Alertmanager with provisioned dashboards
- [ ] A simulated Stale Provider results in an engineering alert email
- [ ] Delivery-failure and circuit-breaker alert rules exist and have been tested
- [ ] A test scanning the logs from the Seam 1 main flows finds no emails, addresses or coordinates
