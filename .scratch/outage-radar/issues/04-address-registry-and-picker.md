# 04: Address registry and picker

**What to build:** Residents can pick an exact Address in Varna city. The registry is seeded from OpenStreetMap: Settlements with ЕКАТТЕ codes, Varna streets, and the house numbers that exist. Missing house numbers are interpolated along the street geometry, odd and even numbers on opposite sides, and flagged as approximate. The public API offers street autocomplete for a Settlement and building-number lookup, returning coordinates and the approximate flag. The UI flow is Settlement → street (autocomplete) → number → pin on a map that the user confirms or drags. The confirmed position is what later steps use. Other Settlements in the Region are selectable at Settlement level only.

**Blocked by:** 01 (Walking skeleton)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] An import command loads Varna streets and house numbers from an OSM extract and can be re-run
- [ ] Autocomplete matches Cyrillic input, Latin transliteration and common abbreviations (ул., бул.)
- [ ] A house number missing in OSM gets an interpolated position flagged approximate
- [ ] The picker shows a pin the user can confirm or drag, and the confirmed coordinates are returned
- [ ] Choosing a non-Varna Settlement skips street selection
