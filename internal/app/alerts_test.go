package app_test

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alisa/outage-radar/internal/testkit"
)

// hrow is row() with a chosen time window on 14.11.2026.
func hrow(id int, main, from, to string) string {
	return fmt.Sprintf(`{"id":%d,"from":"14.11.2026","to":"14.11.2026","hours":{"string":"","array":[[%q,%q]]},"published_at":"","main":%q}`, id, from, to, main)
}

const intro = "Поради профилактика ще бъде прекъснато електрозахранването в районите на:<br/><br/>гр. Варна - "

// watcher starts the system and has a verified Subscriber watching ул. Дойран 11; the Provider answers 404 until a feed is served.
func watcher(t *testing.T, lang string) (*testkit.System, *atomic.Bool) {
	t.Helper()
	s := testkit.Start(t, "all", nil)
	importVarna(t, s)
	signup(t, s, "ivan@example.com", lang, addr{streets(t, s, "10135", "doyran")[0].ID, "11"})
	if c := verify(t, s, token(t, waitMail(t, s, 1)[0])); c != 204 {
		t.Fatalf("verify %d", c)
	}
	return s, new(atomic.Bool)
}

// publish serves the rows and lets the next poll happen.
func publish(s *testkit.System, down *atomic.Bool, rows ...string) {
	serveFeed(s, down, feed(rows...))
	s.Clock.Advance(16 * time.Minute)
}

func TestNewOutageOnAWatchedAddressSendsExactlyOneEmailAndReingestingSendsNone(t *testing.T) {
	s, down := watcher(t, "en")
	publish(s, down, hrow(1, intro+"ул. Дойран.", "9:00", "17:00"))
	m := waitMail(t, s, 2)[1]
	if m.To != "ivan@example.com" || !strings.Contains(m.Body, "ул. Дойран") {
		t.Errorf("alert: %+v", m)
	}
	s.Clock.Advance(16 * time.Minute) // the same Notice is fetched again
	waitMail(t, s, 2)
}

func TestOnlyAMaterialChangeProducesAnotherAlert(t *testing.T) {
	s, down := watcher(t, "en")
	step := func(row string, want int) {
		t.Helper()
		publish(s, down, row)
		waitMail(t, s, want)
	}
	step(hrow(1, intro+"ул. Дойран.", "9:00", "17:00"), 2)
	step(hrow(1, intro+"ул. Дойран 11.", "9:20", "17:10"), 2)      // under 30 minutes: no Alert
	step(hrow(1, intro+"ул. Дойран 11.", "9:20", "17:45"), 3)      // end moved 45 minutes: one Alert
	step(hrow(1, intro+"част от ул. Дойран.", "9:20", "17:45"), 4) // Match Result changed
}
