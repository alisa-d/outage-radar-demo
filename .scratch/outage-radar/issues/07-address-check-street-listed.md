# 07: Address check for street-listed Outages

**What to build:** Residents can check an Address and get an honest per-Utility answer. Geo resolution turns named streets and listed building numbers into an Affected Area with a confidence level. Matching (Seam 3) gives a Match Result: Affected, Possibly affected or Not affected. The address check combines Match Results with Provider Health and Coverage, so a stale Provider yields No data, never "no outage". The check page shows the result per Utility with the contributing Outages and their original text, and a "Get alerts" action. Nothing about an anonymous check is stored.

**Blocked by:** 03 (Power Outages from ЕРП Север), 04 (Address registry and picker), 06 (Provider Health, status strip, Coverage)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] Seam 3 fixtures: a listed building number gives Affected; another number on the same street gives Not affected; a whole listed street gives Affected
- [ ] Through Seam 1, checking an Address returns per-Utility Match Results with the contributing Outages
- [ ] A Stale or Suspicious Provider yields No data for its Utility
- [ ] Nothing from the check request is persisted (verified by database inspection in the test)
- [ ] Results and explanations render in bg and en
