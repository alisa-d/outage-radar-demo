# 15: Quiet Hours and urgency

**What to build:** Alerts arrive at a sensible time. Alerts created between 22:00 and 08:00 Europe/Sofia are held until 08:00. Exceptions are sent immediately: Outages starting before 09:00, and all emergency Outages.

**Blocked by:** 14 (Alerts by email)

**Status:** ready-for-agent

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [ ] With the clock at 23:30, a planned Outage starting tomorrow at 13:00 is delivered at 08:00, not before
- [ ] With the clock at 23:30, a planned Outage starting at 07:00 is delivered immediately
- [ ] An emergency Outage is delivered immediately at any hour
- [ ] Behaviour is correct across the daylight-saving changes in Europe/Sofia
