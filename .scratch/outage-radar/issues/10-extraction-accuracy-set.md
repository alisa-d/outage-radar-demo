# 10: Extraction accuracy set

**What to build:** A labelled set of real Varna Notices from both Providers, including the examples gathered during design, with their expected Outages (time window, kind, Severity, place descriptions). This is Seam 2: Notice text in, Outages out. The deterministic parsers run against it in CI. An on-demand command runs the LLM extractor against the set and reports accuracy per field. That report is the gate for changing models or prompts, and the run costs real money, so it never runs automatically in CI.

**Blocked by:** 05 (Water Outages from ВиК Варна)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] At least 40 labelled Notices covering every place-description form seen in the spec
- [ ] CI fails if a deterministic parser regresses on the set
- [ ] An on-demand command runs the LLM path and prints accuracy per field and the token cost
- [ ] Adding a Notice to the set requires only adding data, no code
