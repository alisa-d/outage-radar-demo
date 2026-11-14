package app_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

var sofiaLoc, _ = time.LoadLocation("Europe/Sofia")

// at is a wall-clock time in Europe/Sofia, as the fake clock's UTC instant.
func at(y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, sofiaLoc).UTC()
}

// drow is hrow on a chosen day (dd.mm.yyyy).
func drow(id int, day, main, from, to string) string {
	return fmt.Sprintf(`{"id":%d,"from":%q,"to":%q,"hours":{"string":"","array":[[%q,%q]]},"published_at":"","main":%q}`, id, day, day, from, to, main)
}

func TestQuietHours(t *testing.T) {
	cases := []struct {
		name      string
		now       time.Time // when the Alert is created
		day       string
		from, to  string
		release   time.Time // zero: delivered immediately
		emergency bool
	}{
		{"planned tomorrow 13:00 is held until 08:00", at(2026, 11, 14, 23, 30), "15.11.2026", "13:00", "17:00", at(2026, 11, 15, 8, 0), false},
		{"planned at 07:00 is sent at once", at(2026, 11, 14, 23, 30), "15.11.2026", "7:00", "17:00", time.Time{}, false},
		{"emergency is sent at once", at(2026, 11, 14, 23, 30), "15.11.2026", "13:00", "17:00", time.Time{}, true},
		{"spring forward: held until 08:00 EEST", at(2027, 3, 27, 23, 30), "28.03.2027", "13:00", "17:00", at(2027, 3, 28, 8, 0), false},
		{"fall back: held until 08:00 EET", at(2026, 10, 24, 23, 30), "25.10.2026", "13:00", "17:00", at(2026, 10, 25, 8, 0), false},
		{"early morning is held to the same morning", at(2026, 11, 15, 3, 0), "15.11.2026", "13:00", "17:00", at(2026, 11, 15, 8, 0), false},
		{"daytime is sent at once", at(2026, 11, 14, 12, 0), "15.11.2026", "13:00", "17:00", time.Time{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, down := watcherAt(t, "en", c.now)
			if c.emergency {
				serveEmergency(s, c.day)
				s.Clock.Advance(16 * time.Minute)
			} else {
				publish(s, down, drow(1, c.day, intro+"ул. Дойран.", c.from, c.to))
			}
			if c.release.IsZero() {
				waitMail(t, s, 2)
				return
			}
			waitMail(t, s, 1) // only the verification mail while held
			s.Clock.Advance(c.release.Add(-time.Minute).Sub(s.Clock.Now()))
			waitMail(t, s, 1)
			s.Clock.Advance(time.Minute)
			if m := waitMail(t, s, 2)[1]; !strings.Contains(m.Body, "ул. Дойран") {
				t.Errorf("alert: %+v", m)
			}
		})
	}
}

// serveEmergency serves one ВиК Варна breakdown Notice whose Outage starts at 13:00 on day, and an empty ЕРП Север feed.
func serveEmergency(s interface {
	StubProviders(http.Handler)
	StubClaude(http.Handler)
}, day string) {
	s.StubProviders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/bg/profil/xhr/":
			w.Write(feed()(q.Get("type")))
		case r.URL.Path == "/bg/messages.html" && q.Get("type") == "breakdown" && q.Get("page") == "1":
			fmt.Fprint(w, `<h2><a href="/bg/messages/breakdown/900.html">Авария</a></h2><div class="list-item-date">14.11.2026</div>
<div class="list-item-text">Авария на ул. Дойран<a href="x">повече</a>`)
		case r.URL.Path == "/bg/messages.html": // every other list page is empty
		case r.URL.Path == "/bg/messages/breakdown/900.html":
			fmt.Fprint(w, `<div class="view"><p>Авария на водопровода на улица Дойран.</p><div class="clear">`)
		default:
			http.NotFound(w, r)
		}
	}))
	s.StubClaude(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"type":"message","role":"assistant","stop_reason":"tool_use","content":[{"type":"tool_use","id":"t1","name":"record_outages","input":{"outages":[{"date":%q,"start":"13:00","end":"17:00","end_approximate":false,"severity":"cut","settlement":"Варна","places":["ул. Дойран"]}]}}]}`, day[:5])
	}))
}
