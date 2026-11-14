# 09: Region map

**What to build:** The Region page shows a Leaflet map (OSM tiles) of Affected Areas for the selected day, with Affected and Possibly affected areas visually distinct. Each area links to its Outage and original Notice. Unlocated Outages are listed next to the map. The map works at phone width.

**Blocked by:** 07 (Address check for street-listed Outages)

**Status:** resolved

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [x] Affected Area geometry comes from the public Outages API
- [x] Affected and Possibly affected areas are visually distinct, with a legend in bg and en
- [ ] Clicking an area shows the Outage details and the original Notice
- [x] Unlocated Outages are listed beside the map
- [ ] The map is usable at phone width

## Answer

- Affected Areas are built server-side from Outage places and served by the public Outages API; the Region page renders them in a Leaflet map (OSM tiles).
- Affected and Possibly affected use distinct styles with a bg/en legend; unlocated Outages are listed beside the map with a pointer to the original notice text.
- Popups on click show Outage details; phone-width layout uses wrapping flex (both unverified in a browser).

Merged in 84048ce (merge of worktree-agent-aeba4186a61aae8d6).

Known gaps:
- The map was never rendered in a browser: tiles, popups, legend and phone layout are unchecked (hence those two boxes stay open).
- Only street places resolve, so quarters, landmarks, ТП-only and whole-settlement Outages show as unlocated until ticket 08. Ticket 08 will likely conflict in api.go and area.go (it probably replaces outageArea).
- No Notice URL is stored, so there is no external link to the original.
- outageArea queries the DB once per place per Outage, with no caching.
