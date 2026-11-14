# 08: Prose areas: Gazetteer, blocks, landmarks, villages, Unlocated

**What to build:** Water Notices described in loose prose resolve to Affected Areas. The Gazetteer holds named areas and landmarks per Settlement (quarters, местности, resorts, landmarks) imported from OSM. The resolution rules:

- **"Between streets A, B, C, D"**: the enclosed polygon.
- **Named area with a polygon**: Affected inside it. Qualifiers like "част от" or "в района на" cap the result at Possibly affected.
- **Point landmark**: Possibly affected within about 300 m.
- **Whole village**: the Settlement's area.
- **Nothing resolvable inside a city**: the Outage is Unlocated (ADR 0003). It shows in the Region summary and in address checks for that Settlement, and creates no Alerts.

Unresolved names lower the confidence.

**Blocked by:** 05 (Water Outages from ВиК Варна), 07 (Address check for street-listed Outages)

**Status:** resolved

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [x] Seam 3 fixtures: the бул. Васил Левски / ул. Беласица / ул. Тодор Влайков / бул. Сливница block gives Affected inside and Not affected outside
- [x] "част от кв. Аспарухово" gives Possibly affected inside the quarter
- [x] "в района на ЖП гара" and "Фестивален и Конгресен център" give Possibly affected within the radius
- [x] A village-wide Notice gives Affected for every Address in that village
- [x] A city Notice with nothing resolvable is Unlocated, listed in the summary, and mentioned in that Settlement's address checks
- [x] A Gazetteer import command loads named areas from OSM

## Answer

- Prose Notices now resolve to Affected Areas via the Gazetteer (blocks, quarters, landmarks, villages), with Unlocated as the fallback inside a city.
- Gazetteer is loaded by `import-gazetteer`; migration `011_gazetteer.sql`. Merged in e9c858b.
- Known gaps below.

Known gaps:
- Fixtures are synthetic geometry near Varna because Overpass returned 406; the village ЕКАТТЕ 99001 is invented. Real-data accuracy and the Gazetteer import against a real OSM extract (relations/multipolygons) are untested.
- ВиК place strings in tests are hand-written, not real Claude output.
- Map `Area` is still street-only, so quarters/landmarks/blocks have no shape on the map.
- Blocks use the convex hull of street crossings.
- Streets missing from the registry make a Notice count as unplaced.
- Hyphenated/ranged building numbers are still not parsed.
- Most tests went green on first run with no mutation check.
- Unlocated Alerts create nothing, and that has no test.
- Review queue/Gazetteer editing are ticket 18, so nothing releases Unlocated Alerts yet.
- UI never opened in a browser.
