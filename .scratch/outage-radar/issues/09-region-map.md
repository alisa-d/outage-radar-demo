# 09: Region map

**What to build:** The Region page shows a Leaflet map (OSM tiles) of Affected Areas for the selected day, with Affected and Possibly affected areas visually distinct. Each area links to its Outage and original Notice. Unlocated Outages are listed next to the map. The map works at phone width.

**Blocked by:** 07 (Address check for street-listed Outages)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] Affected Area geometry comes from the public Outages API
- [ ] Affected and Possibly affected areas are visually distinct, with a legend in bg and en
- [ ] Clicking an area shows the Outage details and the original Notice
- [ ] Unlocated Outages are listed beside the map
- [ ] The map is usable at phone width
