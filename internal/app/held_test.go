package app_test

import "testing"

func TestHeldAlertIsSkippedWhenItsOutageHasEndedBeforeRelease(t *testing.T) {
	s, down := watcherAt(t, "en", at(2026, 11, 14, 23, 30))
	publish(s, down, drow(1, "15.11.2026", intro+"ул. Дойран.", "13:00", "17:00")) // held until 08:00
	waitMail(t, s, 1)
	// The Provider corrects the Notice to a past day before the Alert is released.
	publish(s, down, drow(1, "13.11.2026", intro+"ул. Дойран.", "13:00", "17:00"))
	waitFor(t, func() bool { return count(t, s, "SELECT count(*) FROM outages WHERE end_at < '2026-11-14'") == 1 })
	s.Clock.Advance(at(2026, 11, 15, 8, 1).Sub(s.Clock.Now()))
	waitFor(t, func() bool { return attempts(t, s) == "skipped" })
	waitMail(t, s, 1)
}
