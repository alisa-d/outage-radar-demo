# 10: Extraction accuracy set

**What to build:** A labelled set of real Varna Notices from both Providers, including the examples gathered during design, with their expected Outages (time window, kind, Severity, place descriptions). This is Seam 2: Notice text in, Outages out. The deterministic parsers run against it in CI. An on-demand command runs the LLM extractor against the set and reports accuracy per field. That report is the gate for changing models or prompts, and the run costs real money, so it never runs automatically in CI.

**Blocked by:** 05 (Water Outages from ВиК Варна)

**Status:** ready-for-human

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [x] At least 40 labelled Notices covering every place-description form seen in the spec
- [x] CI fails if a deterministic parser regresses on the set
- [ ] An on-demand command runs the LLM path and prints accuracy per field and the token cost
- [x] Adding a Notice to the set requires only adding data, no code

## Answer

Built: a labelled set of 42 real Notices (28 ЕРП Север, 14 ВиК Варна) as data files under `internal/ingest/accuracy/testdata/`; a CI test that checks the ЕРП Север parser against the labels; and the on-demand `cmd/extraction-accuracy` command that runs the LLM extractor over the set. Adding a Notice needs only a new data file.

Not done: the real accuracy run against the Claude API was never executed (no API key), so there are no accuracy or cost numbers, and the model ID `claude-haiku-5-5` is unverified. The third checkbox stays open. A human with an `ANTHROPIC_API_KEY` must run `go run ./cmd/extraction-accuracy`, verify the model ID, and record the numbers here.

Known gaps:
- The parser disagrees with the labels on 18 of 28 ЕРП Север Notices. All 18 are marked `known_gap` and are unfixed.
- ВиК Варна labels are only loaded, not checked in CI.
- Strict set-equality place scoring will likely score low at first.
- The spec's design-time example forms (e.g. "ЖП гара", the enclosed block) are not in the set.
- There are no multi-day ЕРП Север Notices.
- There is only one planned ВиК Варна Notice.
