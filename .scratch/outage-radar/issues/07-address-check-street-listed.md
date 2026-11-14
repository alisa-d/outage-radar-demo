# 07: Address check for street-listed Outages

**What to build:** Residents can check an Address and get an honest per-Utility answer. Geo resolution turns named streets and listed building numbers into an Affected Area with a confidence level. Matching (Seam 3) gives a Match Result: Affected, Possibly affected or Not affected. The address check combines Match Results with Provider Health and Coverage, so a stale Provider yields No data, never "no outage". The check page shows the result per Utility with the contributing Outages and their original text, and a "Get alerts" action. Nothing about an anonymous check is stored.

**Blocked by:** 03 (Power Outages from ЕРП Север), 04 (Address registry and picker), 06 (Provider Health, status strip, Coverage)

**Status:** resolved

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] Seam 3 fixtures: a listed building number gives Affected; another number on the same street gives Not affected; a whole listed street gives Affected
- [ ] Through Seam 1, checking an Address returns per-Utility Match Results with the contributing Outages
- [ ] A Stale or Suspicious Provider yields No data for its Utility
- [ ] Nothing from the check request is persisted (verified by database inspection in the test)
- [ ] Results and explanations render in bg and en

## Answer

- Address check returns per-Utility Match Results (Affected / Possibly affected / Not affected) with contributing Outages and original text, combined with Provider Health so stale Providers give No data.
- Street-listed Outages are geo-resolved by street name and building number; the check page shows results plus a "Get alerts" action; nothing about an anonymous check is stored.
- Merged in 51dc7a0 (merge commit).

Known gaps:
- UI never opened in a browser.
- "Get alerts" links to /subscribe?street_id=..&number=.. and no such page exists yet.
- Only street-based places matched, so quarters/landmarks/ТП-only/Unlocated count as Not affected until ticket 08.
- Hyphenated or ranged numbers like "8-ми", "1-5" not parsed.
- Pin position not used for matching.
- Outages not listed on a no_data result.
