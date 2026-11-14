# 18: Review queue and Outage correction

**What to build:** Operators fix what automatic resolution missed. A review queue lists Unlocated Outages and unresolved place names. For each item, an Operator sees the original Notice beside the extracted Outages and can attach an Affected Area (draw it, or pick streets and Gazetteer entries), correct the time window or Severity, and add or edit Gazetteer entries. Placing an Unlocated Outage releases the Alerts that were held back. Corrections are audited.

**Blocked by:** 08 (Prose areas: Gazetteer, blocks, landmarks, villages, Unlocated), 14 (Alerts by email), 17 (Backoffice sign-in, health and stats, audit log)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] Through Seam 1, an Unlocated Outage appears in the queue, and attaching an area makes the matching Subscribers receive Alerts
- [ ] Correcting a time window triggers Alerts only if the correction is material
- [ ] A new Gazetteer entry makes later Notices that name it resolve automatically
- [ ] Every correction is in the audit log with the Operator and a timestamp
