# 08: Prose areas: Gazetteer, blocks, landmarks, villages, Unlocated

**What to build:** Water Notices described in loose prose resolve to Affected Areas. The Gazetteer holds named areas and landmarks per Settlement (quarters, местности, resorts, landmarks) imported from OSM. The resolution rules:

- **"Between streets A, B, C, D"**: the enclosed polygon.
- **Named area with a polygon**: Affected inside it. Qualifiers like "част от" or "в района на" cap the result at Possibly affected.
- **Point landmark**: Possibly affected within about 300 m.
- **Whole village**: the Settlement's area.
- **Nothing resolvable inside a city**: the Outage is Unlocated (ADR 0003). It shows in the Region summary and in address checks for that Settlement, and creates no Alerts.

Unresolved names lower the confidence.

**Blocked by:** 05 (Water Outages from ВиК Варна), 07 (Address check for street-listed Outages)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] Seam 3 fixtures: the бул. Васил Левски / ул. Беласица / ул. Тодор Влайков / бул. Сливница block gives Affected inside and Not affected outside
- [ ] "част от кв. Аспарухово" gives Possibly affected inside the quarter
- [ ] "в района на ЖП гара" and "Фестивален и Конгресен център" give Possibly affected within the radius
- [ ] A village-wide Notice gives Affected for every Address in that village
- [ ] A city Notice with nothing resolvable is Unlocated, listed in the summary, and mentioned in that Settlement's address checks
- [ ] A Gazetteer import command loads named areas from OSM
