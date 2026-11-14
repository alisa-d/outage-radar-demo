# 05: Water Outages from ВиК Варна

**What to build:** Residents see planned and emergency water Outages. The ВиК Варна adapter fetches the HTML lists (`repair` and `breakdown`) for the Varna region, paging only as far as needed, and yields Notices with Provider IDs. Their prose goes to the shared LLM extractor, which sits behind the same extraction interface as the deterministic parsers. The extractor is Claude Haiku 5.5 with a structured output schema, receives only Notice text, and runs once per unique Notice text. A Notice with no date uses its publication date, and "около" end times are flagged approximate. Water Outages appear in the Region list next to power Outages.

**Blocked by:** 03 (Power Outages from ЕРП Север)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] Through Seam 1, captured real ВиК Варна pages replayed by the stub server plus stubbed Claude responses produce the expected water Outages
- [ ] An identical Notice text is never sent to the LLM twice
- [ ] Kind (planned or emergency) follows the list the Notice came from
- [ ] An LLM failure or invalid output leaves the Notice queued for retry, without inventing Outages, and is counted in metrics
- [ ] No personal data is ever part of an extraction request
