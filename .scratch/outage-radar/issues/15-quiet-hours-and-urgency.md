# 15: Quiet Hours and urgency

**What to build:** Alerts arrive at a sensible time. Alerts created between 22:00 and 08:00 Europe/Sofia are held until 08:00. Exceptions are sent immediately: Outages starting before 09:00, and all emergency Outages.

**Blocked by:** 14 (Alerts by email)

**Status:** resolved

Spec: `.scratch/outage-radar/spec.md`. Vocabulary: `GLOSSARY.md`.

- [x] With the clock at 23:30, a planned Outage starting tomorrow at 13:00 is delivered at 08:00, not before
- [x] With the clock at 23:30, a planned Outage starting at 07:00 is delivered immediately
- [x] An emergency Outage is delivered immediately at any hour
- [x] Behaviour is correct across the daylight-saving changes in Europe/Sofia

## Answer

- Alerts created between 22:00 and 08:00 Europe/Sofia are held until 08:00; Outages starting before 09:00 on the release morning and all emergency Outages are sent immediately.
- Timing uses Europe/Sofia local time, so daylight-saving changes are handled by the time zone database.
- Tests in `internal/app/quiet_test.go` cover the 23:30 cases (13:00 held, 07:00 immediate) and emergency delivery.

Merge commit: 0b4368a (`feat: quiet hours and urgency (ticket 15)`). No migration.

Known gaps:
- The emergency test uses a hand-made ВиК Варна HTML stub and a stubbed Claude, not captured pages.
- Only the in-test SMTP sink was used, never Mailpit or Brevo.
- A held Alert is still sent at 08:00 even if its Outage has since ended or been cancelled.
- The "before 09:00 on the release morning" rule is the implementer's interpretation of the ticket (covers Outages already started).
- Two of the six new tests were regression guards that passed before the change.
