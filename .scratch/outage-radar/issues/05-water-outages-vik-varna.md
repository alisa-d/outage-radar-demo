# 05: Water Outages from ВиК Варна

**What to build:** Residents see planned and emergency water Outages. The ВиК Варна adapter fetches the HTML lists (`repair` and `breakdown`) for the Varna region, paging only as far as needed, and yields Notices with Provider IDs. Their prose goes to the shared LLM extractor, which sits behind the same extraction interface as the deterministic parsers. The extractor is Claude Haiku 5.5 with a structured output schema, receives only Notice text, and runs once per unique Notice text. A Notice with no date uses its publication date, and "около" end times are flagged approximate. Water Outages appear in the Region list next to power Outages.

**Blocked by:** 03 (Power Outages from ЕРП Север)

**Status:** resolved

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] Through Seam 1, captured real ВиК Варна pages replayed by the stub server plus stubbed Claude responses produce the expected water Outages
- [ ] An identical Notice text is never sent to the LLM twice
- [ ] Kind (planned or emergency) follows the list the Notice came from
- [ ] An LLM failure or invalid output leaves the Notice queued for retry, without inventing Outages, and is counted in metrics
- [ ] No personal data is ever part of an extraction request

## Answer

- ВиК Варна water Outages come from the Район Варна planned and emergency lists; each Notice page is fetched and extracted by Claude.
- Extractions are cached by Notice text and stored in `008_llm_extractions.sql`, so identical text is never sent twice; a failure leaves the Notice for retry and is counted in `outage_radar_extraction_failures_total` (OpenTelemetry, label `provider`).
- Kind follows the list the Notice came from, and no personal data goes into extraction requests.

Merged in b952c20 (`git merge --no-ff`, branch `worktree-agent-a24d27722230c2d36`). The migration was renumbered from 007 to 008.

Known gaps:
- Model ID `claude-haiku-5-5` and extraction quality have never been tested against the real Claude API.
- Seam 2 labelled-set accuracy is ticket 10.
- There are no recent planned-work (repair) Notices in the live feed, so one old Notice covers the planned path.
- Only 3 captured Notices have stubbed extractions.
- Water-next-to-power is not asserted in a test.
- The Notice-page cache is in memory only.
- Only the Район Варна list is fetched.
