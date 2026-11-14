# 06: Provider Health, status strip, Coverage

Status: resolved

**What to build:** Residents can always tell whether the data can be trusted. Each Provider's health is tracked:

- **Stale**: no successful fetch and parse within 3× its polling interval.
- **Suspicious**: zero Notices where some are normally present, or an unusual share of items that fail to parse.

The Region page shows a status strip per Utility ("Water: data from ВиК Варна unavailable since 09:15") and Coverage notes ("Power: planned outages only"). Known Outages stay visible while a Provider is unhealthy. Adapters emit OpenTelemetry metrics: fetch success and latency, Notices found, extraction failures, unresolved names and health state.

**Blocked by:** 03 (Power Outages from ЕРП Север)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] With the clock advanced past 3× the interval after the last success, the Provider becomes Stale through the public API
- [ ] An empty or garbage response where Notices are normally present makes the Provider Suspicious
- [ ] Outages already known remain listed while the Provider is unhealthy
- [ ] The status strip and Coverage notes render in bg and en
- [ ] The adapter metrics listed above are exported

## Answer

- Provider health (Stale after 3x interval, Suspicious on empty/garbage where Notices are normal) is tracked per Provider (migration 007) and exposed through the public API; known Outages stay listed.
- Region page shows a status strip per Utility and Coverage notes in bg and en.
- Adapters export fetch success/latency, Notices found, extraction failures, unresolved names and health state via /metrics.

Merge commit: b08d3cf (feat: provider health and status strip, ticket 06).

Known gaps:
- Status strip never rendered in a browser (frontend only built, not viewed).
- /metrics is on the API listener only, so the ingest role alone exports nothing, and the frontend doesn't proxy it.
- unresolved_names metric is a name-equality check that ticket 05 may replace.
- Failure backoff still missing.
- Operator view and Stale/Suspicious alert (spec stories 50, 60) not built, left for tickets 17 and 21.
- Address-check "No data" path not built, left for ticket 07.
