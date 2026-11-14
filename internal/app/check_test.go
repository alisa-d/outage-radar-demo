package app_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alisa/outage-radar/internal/testkit"
	"github.com/jackc/pgx/v5/pgxpool"
)

type checkOutage struct {
	Provider string   `json:"provider"`
	Severity string   `json:"severity"`
	Places   []string `json:"places"`
	Notice   string   `json:"notice"`
}

type utilityResult struct {
	Utility  string        `json:"utility"`
	Provider string        `json:"provider"`
	Coverage string        `json:"coverage"`
	Result   string        `json:"result"`
	Outages  []checkOutage `json:"outages"`
	// Unlocated and Health: known Outages stay listed when the Provider is not healthy.
	Unlocated []checkOutage `json:"unlocated"`
	Health    string        `json:"provider_health"`
}

type checkResponse struct {
	Utilities []utilityResult `json:"utilities"`
}

func check(t *testing.T, s *testkit.System, streetQ string, num string, lang string) map[string]utilityResult {
	t.Helper()
	id := streets(t, s, "10135", streetQ)[0].ID
	var out checkResponse
	u := fmt.Sprintf("%s/api/check?street=%d&number=%s&lang=%s", s.URL, id, num, lang)
	if c := getJSON(t, u, &out); c != 200 {
		t.Fatalf("check status %d", c)
	}
	m := map[string]utilityResult{}
	for _, r := range out.Utilities {
		m[r.Utility] = r
	}
	return m
}

func fixtureFeed(t *testing.T, typ string) []byte {
	b, err := os.ReadFile("../ingest/erpsever/testdata/" + typ + ".json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// serveFeed replays feed(type), or 500 while down is set.
func serveFeed(s *testkit.System, down *atomic.Bool, feed func(typ string) []byte) {
	s.StubProviders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bg/profil/xhr/" { // other Providers share the stub server
			http.NotFound(w, r)
			return
		}
		if down.Load() {
			http.Error(w, "boom", 500)
			return
		}
		w.Write(feed(r.URL.Query().Get("type")))
	}))
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatal("condition not met in 10s")
}

func ingested(t *testing.T, s *testkit.System) {
	t.Helper()
	waitFor(t, func() bool {
		var b struct {
			Days []any `json:"days"`
		}
		return getJSON(t, s.URL+"/api/regions/1/outages", &b) == 200 && len(b.Days) > 0
	})
	// Outages are stored before the poll is recorded as a success.
	waitFor(t, func() bool {
		var rs []struct {
			Providers []struct {
				Name   string `json:"name"`
				Health struct {
					State string `json:"state"`
				} `json:"health"`
			} `json:"providers"`
		}
		getJSON(t, s.URL+"/api/regions", &rs)
		for _, p := range rs[0].Providers {
			if p.Name == "ЕРП Север" {
				return p.Health.State == "healthy"
			}
		}
		return false
	})
}

func TestListedBuildingNumberIsAffectedAndAnotherNumberOnTheSameStreetIsNot(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	var down atomic.Bool
	serveFeed(s, &down, func(typ string) []byte { return fixtureFeed(t, typ) })
	importVarna(t, s)
	ingested(t, s)

	// Notices 91494 (cut, 09-12) and 91495 (disturbance, 09-16) both list ул. Брегалница 3.
	got := check(t, s, "bregalnitsa", "3", "bg")["power"]
	if got.Result != "affected" || len(got.Outages) != 2 || got.Provider != "ЕРП Север" || got.Coverage != "planned outages only" {
		t.Fatalf("number 3: %+v", got)
	}
	sev := got.Outages[0].Severity + "," + got.Outages[1].Severity
	if sev != "cut,disturbance" && sev != "disturbance,cut" {
		t.Errorf("severities %s", sev)
	}
	if !strings.Contains(got.Outages[0].Notice, "ТП 314") && !strings.Contains(got.Outages[1].Notice, "ТП 314") {
		t.Errorf("contributing Outages must carry the original text: %+v", got.Outages)
	}

	// Only the disturbance lists 24; number 5 is on the street but in no Notice.
	if r := check(t, s, "bregalnitsa", "24", "bg")["power"]; r.Result != "affected" || len(r.Outages) != 1 || r.Outages[0].Severity != "disturbance" {
		t.Errorf("number 24: %+v", r)
	}
	if r := check(t, s, "bregalnitsa", "5", "bg")["power"]; r.Result != "not_affected" || len(r.Outages) != 0 {
		t.Errorf("number 5: %+v", r)
	}
	// A street named in no Notice.
	if r := check(t, s, "ferdinand", "5", "bg")["power"]; r.Result != "not_affected" {
		t.Errorf("Фердинанд 5: %+v", r)
	}
	// Water has no data yet: ВиК Варна has no adapter, so silence means nothing.
	if r := check(t, s, "bregalnitsa", "3", "bg")["water"]; r.Result != "no_data" || len(r.Outages) != 0 {
		t.Errorf("water: %+v", r)
	}
}

// row builds one ЕРП Север feed row.
func row(id int, main string) string {
	return fmt.Sprintf(`{"id":%d,"from":"14.11.2026","to":"14.11.2026","hours":{"string":"","array":[["9:00","17:00"]]},"published_at":"","main":%q}`, id, main)
}

func feed(rows ...string) func(string) []byte {
	return func(typ string) []byte {
		return []byte(`[{"area_id":"1","total_location_result_rows_` + typ + `":[` + strings.Join(rows, ",") + `]}]`)
	}
}

func TestWholeListedStreetIsAffectedAndPartOfAStreetIsPossiblyAffected(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	var down atomic.Bool
	serveFeed(s, &down, feed(
		row(1, "Поради профилактика ще бъде прекъснато електрозахранването в районите на:<br/><br/>гр. Варна - ул. Дойран, ул. Иглика 6, 8."),
		row(2, "Поради профилактика ще бъде прекъснато електрозахранването в районите на:<br/><br/>гр. Варна - част от ул. Фердинанд."),
		row(3, "Поради профилактика ще бъде прекъснато електрозахранването в районите на:<br/><br/>гр. Девня - ул. Брегалница."),
	))
	importVarna(t, s)
	ingested(t, s)

	if r := check(t, s, "doyran", "11", "bg")["power"]; r.Result != "affected" || len(r.Outages) != 1 {
		t.Errorf("whole street: %+v", r)
	}
	if r := check(t, s, "ferdinand", "5", "bg")["power"]; r.Result != "possibly_affected" {
		t.Errorf("part of street: %+v", r)
	}
	// Same street name, different Settlement.
	if r := check(t, s, "bregalnitsa", "3", "bg")["power"]; r.Result != "not_affected" {
		t.Errorf("other settlement: %+v", r)
	}
}

func TestStaleOrSuspiciousProviderYieldsNoDataNeverNoOutage(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	var down atomic.Bool
	serveFeed(s, &down, func(typ string) []byte { return fixtureFeed(t, typ) })
	importVarna(t, s)
	ingested(t, s)
	if r := check(t, s, "bregalnitsa", "5", "bg")["power"]; r.Result != "not_affected" {
		t.Fatalf("healthy: %+v", r)
	}

	down.Store(true)
	s.Clock.Advance(46 * time.Minute) // past 3 x 15 min
	waitFor(t, func() bool { return check(t, s, "bregalnitsa", "5", "bg")["power"].Result == "no_data" })
	// Known Outages stay valid (story 43): only the claim of completeness is withdrawn.
	if r := check(t, s, "bregalnitsa", "3", "bg")["power"]; r.Result != "affected" || len(r.Outages) != 2 || r.Health != "stale" {
		t.Errorf("stale, listed number: %+v", r)
	}
	if r := check(t, s, "bregalnitsa", "5", "bg")["power"]; r.Result != "no_data" || len(r.Outages) != 0 || r.Health != "stale" {
		t.Errorf("stale, unlisted number: %+v", r)
	}

	down.Store(false)
	s.Clock.Advance(16 * time.Minute)
	waitFor(t, func() bool { return check(t, s, "bregalnitsa", "5", "bg")["power"].Result == "not_affected" })

	// Suspicious: fetches succeed but the response is empty where Notices are normal.
	serveFeed(s, &down, feed())
	s.Clock.Advance(16 * time.Minute)
	waitFor(t, func() bool { return check(t, s, "bregalnitsa", "5", "bg")["power"].Result == "no_data" })
}

func TestEnglishCheckTransliteratesPlacesAndKeepsOriginalText(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	var down atomic.Bool
	serveFeed(s, &down, func(typ string) []byte { return fixtureFeed(t, typ) })
	importVarna(t, s)
	ingested(t, s)
	r := check(t, s, "bregalnitsa", "3", "en")["power"]
	if len(r.Outages) == 0 || !strings.Contains(strings.Join(r.Outages[0].Places, ";"), "Bregalnitsa") || !strings.Contains(r.Outages[0].Notice, "Брегалница") {
		t.Errorf("en: %+v", r)
	}
}

func TestCheckStoresNothing(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	var down atomic.Bool
	serveFeed(s, &down, func(typ string) []byte { return fixtureFeed(t, typ) })
	importVarna(t, s)
	ingested(t, s)
	pool, err := pgxpool.New(context.Background(), s.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	snapshot := func() string {
		rows, err := pool.Query(context.Background(), `SELECT relname FROM pg_class WHERE relkind = 'r' AND relnamespace = 'public'::regnamespace ORDER BY 1`)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for rows.Next() {
			var n string
			rows.Scan(&n)
			names = append(names, n)
		}
		rows.Close()
		var b strings.Builder
		for _, n := range names {
			// hash of the whole table content, so an update counts as well as an insert
			var h string
			if err := pool.QueryRow(context.Background(), `SELECT coalesce(md5(string_agg(t::text, '|' ORDER BY t::text)), '') FROM `+n+` t`).Scan(&h); err != nil {
				t.Fatal(err)
			}
			b.WriteString(n + "=" + h + ";")
		}
		return b.String()
	}
	time.Sleep(300 * time.Millisecond)
	before := snapshot()
	for i := 0; i < 3; i++ {
		check(t, s, "bregalnitsa", strconv.Itoa(3+i), "bg")
	}
	if after := snapshot(); after != before {
		t.Errorf("database changed by anonymous checks:\n%s\n%s", before, after)
	}
}

func TestUnlocatedOutagesAreListedEvenWhenTheProviderIsStale(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	var down atomic.Bool
	serveFeed(s, &down, feed(row(1, intro+"клиентите, електрозахранени от ТП 314.")))
	importVarna(t, s)
	ingested(t, s)
	down.Store(true)
	s.Clock.Advance(46 * time.Minute)
	waitFor(t, func() bool { return check(t, s, "doyran", "11", "bg")["power"].Result == "no_data" })
	if r := check(t, s, "doyran", "11", "bg")["power"]; len(r.Unlocated) != 1 || r.Result != "no_data" {
		t.Errorf("stale unlocated: %+v", r)
	}
}
