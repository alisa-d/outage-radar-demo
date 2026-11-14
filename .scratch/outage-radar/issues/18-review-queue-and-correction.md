# 18: Review queue and Outage correction

**What to build:** Operators fix what automatic resolution missed. A review queue lists Unlocated Outages and unresolved place names. For each item, an Operator sees the original Notice beside the extracted Outages and can attach an Affected Area (draw it, or pick streets and Gazetteer entries), correct the time window or Severity, and add or edit Gazetteer entries. Placing an Unlocated Outage releases the Alerts that were held back. Corrections are audited.

**Blocked by:** 08 (Prose areas: Gazetteer, blocks, landmarks, villages, Unlocated), 14 (Alerts by email), 17 (Backoffice sign-in, health and stats, audit log)

**Status:** resolved

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [x] Through Seam 1, an Unlocated Outage appears in the queue, and attaching an area makes the matching Subscribers receive Alerts
- [x] Correcting a time window triggers Alerts only if the correction is material
- [x] A new Gazetteer entry makes later Notices that name it resolve automatically
- [x] Every correction is in the audit log with the Operator and a timestamp

## Answer

Operators get a review queue of Unlocated Outages and unresolved place names, with the Notice beside the extracted Outages.
A PATCH on an Outage attaches streets, Gazetteer entries or a drawn ring, and corrects the time window or Severity. It releases held Alerts, and a time change alerts only when material.
Gazetteer entries can be added and edited, and every correction is audited with the Operator and a timestamp. Merged in 873d721 (no migration).

Known gaps:
- A correction lives on the Outage row, so a later change to the Provider's Notice replaces it.
- When an Operator adds an area, the unresolved place names are dropped from `places` (the Notice text keeps them).
- Adding a Gazetteer entry does not rescan existing Notices, so an already-ingested Outage needs a PATCH to release its Alerts.
- Village Outages and unknown Settlements are not queued.
- A drawn ring becomes a Gazetteer area `Корекция <notice> <ordinal>` that the map does not show.
- The ring, time-window and Gazetteer tests were written after the code and never seen failing on their own.
- No frontend; never run against real Keycloak, a browser or compose.
