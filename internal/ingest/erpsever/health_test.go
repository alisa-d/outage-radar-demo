package erpsever_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alisa/outage-radar/internal/testkit"
)

type health struct {
	State       string  `json:"state"`
	LastSuccess *string `json:"last_success"`
}

func providerHealth(t *testing.T, s *testkit.System, name string) health {
	t.Helper()
	resp, err := http.Get(s.URL + "/api/regions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var regions []struct {
		Providers []struct {
			Name   string `json:"name"`
			Health health `json:"health"`
		} `json:"providers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&regions); err != nil {
		t.Fatal(err)
	}
	for _, p := range regions[0].Providers {
		if p.Name == name {
			return p.Health
		}
	}
	t.Fatalf("no provider %s", name)
	return health{}
}

func waitState(t *testing.T, s *testkit.System, want string) health {
	t.Helper()
	var h health
	waitFor(t, func() bool { h = providerHealth(t, s, "ЕРП Север"); return h.State == want })
	return h
}

// replay answers with the captured fixtures, or one of the failing modes.
func replay(s *testkit.System, t *testing.T, mode *atomicString) {
	s.StubProviders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bg/profil/xhr/" { // the ВиК Варна adapter shares this stub
			http.NotFound(w, r)
			return
		}
		typ := r.URL.Query().Get("type")
		switch mode.Load() {
		case "ok":
			w.Write(fixture(t, typ))
		case "empty":
			w.Write(fixture(t, "empty_"+typ))
		case "garbage":
			w.Write(fixture(t, "garbage"))
		case "unparseable": // rows the parser cannot read
			w.Write([]byte(`[{"area_id":"1","total_location_result_rows_` + typ + `":[
			 {"id":9001,"from":"x","to":"y","hours":{"string":"","array":[]},"published_at":"","main":"???"},
			 {"id":9002,"from":"x","to":"y","hours":{"string":"","array":[]},"published_at":"","main":"???"}]}]`))
		default:
			http.Error(w, "boom", http.StatusInternalServerError)
		}
	}))
}

type atomicString struct{ v atomic.Value }

func (a *atomicString) Load() string   { return a.v.Load().(string) }
func (a *atomicString) Store(s string) { a.v.Store(s) }

func newMode(s string) *atomicString { m := &atomicString{}; m.Store(s); return m }

func TestHealthyAfterASuccessfulFetch(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	replay(s, t, newMode("ok"))
	h := waitState(t, s, "healthy")
	// 09:00 UTC is 11:00 in Sofia.
	if h.LastSuccess == nil || !strings.HasPrefix(*h.LastSuccess, "2026-11-14T11:00:00+02:00") {
		t.Fatalf("last_success = %v", h.LastSuccess)
	}
}

func TestProviderBecomesStaleAfterThreeIntervalsAndKnownOutagesStay(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	mode := newMode("ok")
	replay(s, t, mode)
	waitState(t, s, "healthy")
	before := len(days(t, s, "bg"))

	mode.Store("error")
	s.Clock.Advance(44 * time.Minute) // under 3 x 15 min
	time.Sleep(500 * time.Millisecond)
	if h := providerHealth(t, s, "ЕРП Север"); h.State != "healthy" {
		t.Fatalf("after 44 min: %+v", h)
	}
	s.Clock.Advance(2 * time.Minute)
	h := waitState(t, s, "stale")
	if !strings.HasPrefix(*h.LastSuccess, "2026-11-14T11:00:00+02:00") {
		t.Fatalf("stale since should be the last success: %v", *h.LastSuccess)
	}
	if after := len(days(t, s, "bg")); after != before {
		t.Fatalf("known Outages changed while stale: %d -> %d days", before, after)
	}

	mode.Store("ok")
	s.Clock.Advance(16 * time.Minute)
	waitState(t, s, "healthy")
}

func TestEmptyOrGarbageResponseWhereNoticesAreNormalMakesProviderSuspicious(t *testing.T) {
	for _, bad := range []string{"empty", "garbage", "unparseable"} {
		t.Run(bad, func(t *testing.T) {
			s := testkit.Start(t, "all", nil)
			mode := newMode("ok")
			replay(s, t, mode)
			waitState(t, s, "healthy")
			days(t, s, "bg")

			mode.Store(bad)
			s.Clock.Advance(16 * time.Minute)
			waitState(t, s, "suspicious")
			if len(days(t, s, "bg")) == 0 {
				t.Fatal("known Outages vanished")
			}

			mode.Store("ok")
			s.Clock.Advance(16 * time.Minute)
			waitState(t, s, "healthy")
		})
	}
}

func TestEmptyFirstResponseIsNotSuspiciousWhereNothingWasEverSeen(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	replay(s, t, newMode("empty"))
	waitState(t, s, "healthy")
}

func TestAdapterMetricsAreExported(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	mode := newMode("ok")
	replay(s, t, mode)
	waitState(t, s, "healthy")
	mode.Store("unparseable")
	s.Clock.Advance(16 * time.Minute)
	waitState(t, s, "suspicious")

	resp, err := http.Get(s.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	for _, want := range []string{
		`outage_radar_fetch_total{`, `outage_radar_fetch_duration_seconds_bucket{`, `outage_radar_notices_found{`,
		`outage_radar_extraction_failures_total{`, `outage_radar_unresolved_names_total{`, `outage_radar_provider_health{`,
		`provider="ЕРП Север"`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("metrics lack %s", want)
		}
	}
}
