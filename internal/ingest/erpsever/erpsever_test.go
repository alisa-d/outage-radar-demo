package erpsever_test

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alisa/outage-radar/internal/testkit"
)

type outage struct {
	Provider       string   `json:"provider"`
	Utility        string   `json:"utility"`
	Kind           string   `json:"kind"`
	Severity       string   `json:"severity"`
	Start          string   `json:"start"`
	End            string   `json:"end"`
	EndApproximate bool     `json:"end_approximate"`
	Places         []string `json:"places"`
	Notice         string   `json:"notice"`
}

type day struct {
	Date        string `json:"date"`
	Settlements []struct {
		Name    string   `json:"name"`
		Outages []outage `json:"outages"`
	} `json:"settlements"`
}

// Captured from https://www.erpsever.bg/bg/profil/xhr/?method=get_interruptions&region_id=1&type=<type>
// on 2026-11-14 (area 1 = Varna oblast), unmodified.
func fixture(t *testing.T, name string) []byte {
	b, err := os.ReadFile("testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// stub replays feed(type) as the provider's response.
func stub(t *testing.T, s *testkit.System, feed func(typ string) []byte) *atomic.Int32 {
	var hits atomic.Int32
	s.StubProviders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/bg/profil/xhr/" || q.Get("method") != "get_interruptions" || q.Get("region_id") != "1" {
			http.NotFound(w, r)
			return
		}
		if ua := r.UserAgent(); !strings.Contains(ua, "outage-radar") {
			t.Errorf("User-Agent %q does not identify us", ua)
		}
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(feed(q.Get("type")))
	}))
	return &hits
}

func days(t *testing.T, s *testkit.System, lang string) []day {
	t.Helper()
	var got []day
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.Get(s.URL + "/api/regions/1/outages?lang=" + lang)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Days []day `json:"days"`
		}
		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if err == nil && len(body.Days) > 0 {
			return body.Days
		}
		if time.Now().After(deadline) {
			t.Fatalf("no outages after 10s (status %d, err %v)", resp.StatusCode, err)
		}
		got = body.Days
		_ = got
		time.Sleep(100 * time.Millisecond)
	}
}

func sofia(y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, time.FixedZone("EET", 2*3600))
}

func find(t *testing.T, ds []day, date, settlement string) []outage {
	t.Helper()
	for _, d := range ds {
		if d.Date != date {
			continue
		}
		for _, s := range d.Settlements {
			if s.Name == settlement {
				return s.Outages
			}
		}
	}
	t.Fatalf("no outages for %s on %s", settlement, date)
	return nil
}

func at(t *testing.T, s string) time.Time {
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return tm
}

func TestCapturedResponsesBecomeOutagesGroupedByDayAndSettlement(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	stub(t, s, func(typ string) []byte { return fixture(t, typ) })

	ds := days(t, s, "bg")

	// Window is today (2026-11-14 in Sofia) through today+7; days are in order.
	if ds[0].Date != "2026-11-14" || ds[len(ds)-1].Date > "2026-11-21" {
		t.Fatalf("days = %s .. %s", ds[0].Date, ds[len(ds)-1].Date)
	}

	// Two Notices for Варна today: a Cut and a Disturbance, ordered by time.
	v := find(t, ds, "2026-11-14", "Варна")
	if len(v) != 2 {
		t.Fatalf("Варна 14 Nov: %+v", v)
	}
	cut, dist := v[0], v[1]
	if cut.Severity != "cut" || !at(t, cut.Start).Equal(sofia(2026, 11, 14, 9, 0)) || !at(t, cut.End).Equal(sofia(2026, 11, 14, 12, 0)) {
		t.Fatalf("cut = %+v", cut)
	}
	if dist.Severity != "disturbance" || !at(t, dist.End).Equal(sofia(2026, 11, 14, 16, 0)) {
		t.Fatalf("disturbance = %+v", dist)
	}
	if cut.Provider != "ЕРП Север" || cut.Utility != "power" || cut.Kind != "planned" || cut.EndApproximate {
		t.Fatalf("cut = %+v", cut)
	}
	if !strings.HasSuffix(cut.Start, "+02:00") {
		t.Fatalf("start %q is not Europe/Sofia", cut.Start)
	}
	if !strings.Contains(cut.Notice, "ул. Брегалница 3, ХЕИ и Център за спешна медицинска помощ, електрозахранени от ТП 314") {
		t.Fatalf("original notice text missing: %q", cut.Notice)
	}
	if len(dist.Places) < 5 || dist.Places[0] != "ул. Брегалница 1, 3, 24, 26, 28, 30, 32, 34, 36, 38, 40, 42, 44, 46, 48, 65" || dist.Places[1] != "ул. Дойран 11, 12, 13, 14" {
		t.Fatalf("places = %q", dist.Places)
	}

	// A zone, in the Notice of the next day.
	z := find(t, ds, "2026-11-15", "Варна")
	if len(z) != 1 || z[0].Places[0] != "Западна промишлена зона" {
		t.Fatalf("zone = %+v", z)
	}

	// Two time windows in one Notice and three villages: 3 villages x 2 windows.
	for _, village := range []string{"Юнак", "Дъбравино", "Горен чифлик"} {
		o := find(t, ds, "2026-11-16", village)
		var windows []string
		for _, x := range o {
			if x.Severity == "disturbance" && len(x.Places) == 0 {
				windows = append(windows, at(t, x.Start).In(time.FixedZone("EET", 7200)).Format("15:04")+"-"+at(t, x.End).In(time.FixedZone("EET", 7200)).Format("15:04"))
			}
		}
		if strings.Join(windows, ",") != "09:00-11:00,15:00-17:00" {
			t.Fatalf("%s windows = %v (%+v)", village, windows, o)
		}
	}

	// A multi-day Notice (16-17 Nov, 09:30-16:00 daily) gives one Outage per day.
	for _, d := range []int{16, 17} {
		var found bool
		for _, x := range find(t, ds, "2026-11-"+strconv.Itoa(d), "Осеново") {
			if at(t, x.Start).Equal(sofia(2026, 11, d, 9, 30)) && at(t, x.End).Equal(sofia(2026, 11, d, 16, 0)) {
				found = true
			}
		}
		if !found {
			t.Fatalf("Осеново on %d Nov", d)
		}
	}

	// English: transliterated structure, Bulgarian original text.
	en := days(t, s, "en")
	ev := find(t, en, "2026-11-14", "Varna")
	if ev[0].Places[0] != "ul. Bregalnitsa 3, HEI i Tsentar za speshna meditsinska pomosht, elektrozahraneni ot TP 314" {
		t.Fatalf("en places = %q", ev[0].Places)
	}
	if !strings.Contains(ev[0].Notice, "Брегалница") {
		t.Fatalf("en notice must stay Bulgarian: %q", ev[0].Notice)
	}
}

func feedOf(typ, hoursString, start, end, main string) []byte {
	rows := "[]"
	if typ == "all_active" {
		b, _ := json.Marshal(map[string]any{
			"id": 1, "from": "15.11.2026", "to": "15.11.2026", "published_at": "13.11.2026 10:00:00", "main": main,
			"hours": map[string]any{"string": hoursString, "array": [][]string{{start, end}}},
		})
		rows = "[" + string(b) + "]"
	}
	return []byte(`[{"area_id":"1","total_location_result_rows_` + typ + `":` + rows + `}]`)
}

func TestRefetchedNoticeIsNotDuplicatedAndChangedTextReplacesItsOutages(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	var changed atomic.Bool
	hits := stub(t, s, func(typ string) []byte {
		if changed.Load() {
			return feedOf(typ, "от 10:00 до 12:00", "10:00", "12:00", "Поради оперативни превключвания, са възможни смущения в районите на:<br/><br/>гр. Варна - ул. Втора 2.<br/>")
		}
		return feedOf(typ, "от 9:00 до около 19:00", "9:00", "19:00", "Поради профилактика, ще бъде прекъснато електрозахранването в районите на:<br/><br/>гр. Варна - ул. Първа 1.<br/>")
	})

	o := find(t, days(t, s, "bg"), "2026-11-15", "Варна")
	if len(o) != 1 || o[0].Severity != "cut" || !o[0].EndApproximate || !at(t, o[0].End).Equal(sofia(2026, 11, 15, 19, 0)) {
		t.Fatalf("first fetch: %+v", o)
	}

	// Next poll after the interval: same Notice again, nothing new.
	before := hits.Load()
	s.Clock.Advance(16 * time.Minute)
	waitFor(t, func() bool { return hits.Load() >= before+2 })
	time.Sleep(300 * time.Millisecond)
	if o := find(t, days(t, s, "bg"), "2026-11-15", "Варна"); len(o) != 1 {
		t.Fatalf("after re-fetch: %+v", o)
	}

	// The Provider edits the Notice: its Outages are replaced, not added to.
	changed.Store(true)
	s.Clock.Advance(16 * time.Minute)
	waitFor(t, func() bool {
		o := find(t, days(t, s, "bg"), "2026-11-15", "Варна")
		return len(o) == 1 && o[0].Severity == "disturbance"
	})
	o = find(t, days(t, s, "bg"), "2026-11-15", "Варна")
	if o[0].EndApproximate || !at(t, o[0].Start).Equal(sofia(2026, 11, 15, 10, 0)) || len(o[0].Places) != 1 || o[0].Places[0] != "ул. Втора 2" || !strings.Contains(o[0].Notice, "Втора") {
		t.Fatalf("after change: %+v", o)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); !cond(); time.Sleep(100 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("condition not met within 10s")
		}
	}
}
