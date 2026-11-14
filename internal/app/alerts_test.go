package app_test

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alisa/outage-radar/internal/app"
	"github.com/alisa/outage-radar/internal/testkit"
	"github.com/jackc/pgx/v5"
)

// hrow is row() with a chosen time window on 14.11.2026.
func hrow(id int, main, from, to string) string {
	return fmt.Sprintf(`{"id":%d,"from":"14.11.2026","to":"14.11.2026","hours":{"string":"","array":[[%q,%q]]},"published_at":"","main":%q}`, id, from, to, main)
}

const intro = "Поради профилактика ще бъде прекъснато електрозахранването в районите на:<br/><br/>гр. Варна - "
const disturbanceIntro = "Възможни смущения в електрозахранването в районите на:<br/><br/>гр. Варна - "

// watcher starts the system and has a verified Subscriber watching ул. Дойран 11; the Provider answers 404 until a feed is served.
func watcher(t *testing.T, lang string) (*testkit.System, *atomic.Bool) {
	t.Helper()
	return watcherAt(t, lang, time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC))
}

func watcherAt(t *testing.T, lang string, now time.Time) (*testkit.System, *atomic.Bool) {
	t.Helper()
	s := testkit.StartAt(t, "all", nil, now)
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

func TestPossiblyAffectedAndDisturbanceAlertsHaveTheirOwnWording(t *testing.T) {
	for _, c := range []struct {
		name, lang, notice string
		want, not          []string
	}{
		{"en possibly", "en", intro + "част от ул. Дойран.", []string{"possibly affecting", "Original notice (in Bulgarian)", "прекъснато електрозахранването"}, []string{"expected at your address"}},
		{"en disturbance", "en", disturbanceIntro + "ул. Дойран.", []string{"Possible Power disturbances"}, []string{"outage expected"}},
		{"bg possibly", "bg", intro + "част от ул. Дойран.", []string{"Възможно спиране на тока, което може да засегне", "Оригинално съобщение", "прекъснато електрозахранването"}, nil},
		{"bg disturbance", "bg", disturbanceIntro + "ул. Дойран.", []string{"Възможни смущения в подаването на тока"}, []string{"Очаквано спиране"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, down := watcher(t, c.lang)
			publish(s, down, hrow(1, c.notice, "9:00", "17:00"))
			m := waitMail(t, s, 2)[1]
			for _, w := range c.want {
				if !strings.Contains(m.Subject+m.Body, w) {
					t.Errorf("missing %q in %q / %q", w, m.Subject, m.Body)
				}
			}
			for _, w := range c.not {
				if strings.Contains(m.Subject+m.Body, w) {
					t.Errorf("unexpected %q", w)
				}
			}
		})
	}
}

func TestOneClickUnsubscribeDisablesTheChannelAtOnce(t *testing.T) {
	s, down := watcher(t, "en")
	publish(s, down, hrow(1, intro+"ул. Дойран.", "9:00", "17:00"))
	m := waitMail(t, s, 2)[1]
	raw := s.SMTP.Messages()[1]
	h := regexp.MustCompile(`(?m)^List-Unsubscribe: <http://app\.test(/api/unsubscribe\?token=[^>]+)>\r$`).FindStringSubmatch(raw)
	if h == nil || !strings.Contains(raw, "List-Unsubscribe-Post: List-Unsubscribe=One-Click") {
		t.Fatalf("RFC 8058 headers missing in %q", raw)
	}
	if !strings.Contains(m.Body, "http://app.test/unsubscribe?token=") {
		t.Errorf("no footer link: %q", m.Body)
	}
	if c, _ := post(t, s.URL+h[1]+"x", nil); c != 400 {
		t.Errorf("forged token: %d", c)
	}
	resp, err := http.Post(s.URL+h[1], "application/x-www-form-urlencoded", strings.NewReader("List-Unsubscribe=One-Click"))
	if err != nil || resp.StatusCode != 204 {
		t.Fatalf("unsubscribe: %v %v", resp, err)
	}
	publish(s, down, hrow(1, intro+"ул. Дойран.", "9:00", "17:00"), hrow(2, intro+"ул. Дойран.", "10:00", "11:00"))
	waitMail(t, s, 2)
}

func TestTransientSMTPFailureIsRetriedAndAttemptsAreRecordedWithoutBodies(t *testing.T) {
	s, down := watcher(t, "en")
	s.SMTP.FailNext(1)
	publish(s, down, hrow(1, intro+"ул. Дойран.", "9:00", "17:00"))
	waitFor(t, func() bool { return attempts(t, s) == "failed" })
	s.Clock.Advance(3 * time.Second) // past the first backoff
	waitMail(t, s, 2)
	waitFor(t, func() bool { return attempts(t, s) == "failed,sent" })
}

func attempts(t *testing.T, s *testkit.System) string {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), s.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	var out, dump string
	if err := conn.QueryRow(context.Background(), `SELECT coalesce(string_agg(outcome, ',' ORDER BY id), ''), coalesce(string_agg(d::text, ' '), '') FROM delivery_attempts d`).Scan(&out, &dump); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"ivan", "example.com", "Дойран"} {
		if strings.Contains(dump, bad) {
			t.Errorf("delivery_attempts holds %q", bad)
		}
	}
	return out
}

func TestUnlocatedOutageCreatesNoAlert(t *testing.T) {
	s, down := watcher(t, "en")
	// Only a transformer station is named, so the city's Outage cannot be placed.
	publish(s, down, hrow(1, intro+"клиентите, електрозахранени от ТП 314.", "9:00", "17:00"))
	waitFor(t, func() bool { return scanned(t, s) })
	waitMail(t, s, 1)
}

func scanned(t *testing.T, s *testkit.System) bool {
	conn, err := pgx.Connect(context.Background(), s.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	var done int
	conn.QueryRow(context.Background(), `SELECT count(*) FROM jobs WHERE kind = 'scan-alerts' AND done_at IS NOT NULL`).Scan(&done)
	return done > 0
}

func TestUnverifiedChannelsNeverGetAlertsAndTheBreakerDoesNotBlockVerifiedOnes(t *testing.T) {
	s := testkit.StartWith(t, "all", nil, func(c *app.Config) { c.EmailDailyBudget = 2 })
	importVarna(t, s)
	st := streets(t, s, "10135", "doyran")[0].ID
	signup(t, s, "ivan@example.com", "en", addr{st, "11"})
	signup(t, s, "pending@example.com", "en", addr{st, "11"}) // never verified
	mails := waitMail(t, s, 2)
	if code, _ := signup(t, s, "maria@example.com", "en", addr{st, "11"}); code != 503 {
		t.Fatalf("want tripped breaker, got %d", code)
	}
	if verify(t, s, token(t, mails[0])) != 204 {
		t.Fatal("verify")
	}
	publish(s, new(atomic.Bool), hrow(1, intro+"ул. Дойран.", "9:00", "17:00"))
	if m := waitMail(t, s, 3)[2]; m.To != "ivan@example.com" {
		t.Errorf("alert to %s", m.To)
	}
}
