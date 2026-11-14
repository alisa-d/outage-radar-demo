# 03: Power Outages from ЕРП Север

**What to build:** Residents see planned power Outages for the Varna Region. The ЕРП Север adapter polls the oblast JSON endpoint (`for_next_48_hours` and `all_active`) on its interval using the shared fetch policy. The Notice store deduplicates by Provider ID and re-extracts Notices whose text changes. A deterministic parser turns the regular format (`гр. Варна - ул. X 1, 3, 5`, zones, villages, "ТП N") into Outages with time window, approximate flag, Severity (Cut for "прекъснато", Disturbance for "възможни смущения") and place descriptions. The Region page lists Outages for today and the next 7 days, grouped by day and Settlement, with the original Notice text, in bg and en. No geo matching yet.

**Blocked by:** 01 (Walking skeleton)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] Through Seam 1, captured real ЕРП Север responses replayed by the stub server produce the expected Outages through the public API
- [ ] Re-fetching the same Notices creates no duplicates, and a changed Notice text replaces its Outages
- [ ] Severity is Cut or Disturbance according to the Notice wording
- [ ] Multi-day windows and approximate end times are represented correctly in Europe/Sofia time
- [ ] The Region page shows Outages grouped by day and Settlement, with original text, in both languages
- [ ] Adding this adapter required no changes outside the adapter beyond registering it
