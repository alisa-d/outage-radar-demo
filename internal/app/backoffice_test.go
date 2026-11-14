package app_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alisa/outage-radar/internal/testkit"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	healthPath = "/backoffice/api/providers/health"
	statsPath  = "/backoffice/api/stats"
	auditPath  = "/backoffice/api/audit"
)

func TestBackofficeRejectsMissingInvalidAndInsufficientSessions(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	op := []string{"operator"}
	cases := []struct {
		name, token, path string
		want              int
	}{
		{"no token", "", healthPath, 401},
		{"garbage", "abc.def.ghi", healthPath, 401},
		{"expired", s.OIDC.Token(testkit.Claims{Roles: op, Expires: -time.Minute}), healthPath, 401},
		{"wrong audience", s.OIDC.Token(testkit.Claims{Roles: op, Audience: "other"}), healthPath, 401},
		{"foreign signature", s.OIDC.Token(testkit.Claims{Roles: op, Signer: other}), healthPath, 401},
		{"no 2FA", s.OIDC.Token(testkit.Claims{Roles: op, NoMFA: true}), healthPath, 403},
		{"no role", s.OIDC.Token(testkit.Claims{}), healthPath, 403},
		{"privacy officer on operator page", s.OIDC.Token(testkit.Claims{Roles: []string{"privacy_officer"}}), healthPath, 403},
		{"operator on audit log", s.OIDC.Token(testkit.Claims{Roles: op}), auditPath, 403},
		{"operator ok", s.OIDC.Token(testkit.Claims{Roles: op}), healthPath, 200},
		{"privacy officer ok", s.OIDC.Token(testkit.Claims{Roles: []string{"privacy_officer"}}), auditPath, 200},
	}
	for _, c := range cases {
		if got, body := do(t, "GET", s.URL+c.path, c.token, nil); got != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.name, got, c.want, body)
		}
	}
}

type providerStub struct{ fail bool }

func (p providerStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if p.fail {
		http.Error(w, "boom", 500)
		return
	}
	fmt.Fprintf(w, `[{"area_id":"1","total_location_result_rows_%s":[]}]`, r.URL.Query().Get("type"))
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); !cond(); time.Sleep(100 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("condition not met within 10s")
		}
	}
}

func TestBackofficeProviderHealthShowsStateLastSuccessAndRecentError(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	type prov struct {
		Name         string   `json:"name"`
		State        string   `json:"state"`
		LastSuccess  *string  `json:"last_success"`
		RecentErrors []string `json:"recent_errors"`
	}
	get := func() prov {
		tok := s.OIDC.Token(testkit.Claims{Roles: []string{"operator"}}) // fresh: the test advances the clock
		_, body := do(t, "GET", s.URL+healthPath, tok, nil)
		var ps []prov
		if err := json.Unmarshal([]byte(body), &ps); err != nil {
			t.Fatalf("%v: %s", err, body)
		}
		for _, p := range ps {
			if p.Name == "ЕРП Север" {
				return p
			}
		}
		t.Fatalf("provider missing: %s", body)
		return prov{}
	}
	if p := get(); p.State != "unknown" {
		t.Fatalf("before any fetch: %+v", p)
	}
	s.StubProviders(providerStub{})
	s.Clock.Advance(16 * time.Minute) // the first poll already ran against the unstubbed site
	waitUntil(t, func() bool { return get().State == "healthy" })
	if p := get(); p.LastSuccess == nil || len(p.RecentErrors) != 0 {
		t.Fatalf("healthy: %+v", p)
	}
	s.StubProviders(providerStub{fail: true})
	s.Clock.Advance(46 * time.Minute)
	waitUntil(t, func() bool { p := get(); return p.State == "stale" && len(p.RecentErrors) > 0 })
}

func TestBackofficeStatsAreAggregateOnlyAndExposeNoPersonalData(t *testing.T) {
	s, st := started(t)
	signup(t, s, "ivan@example.com", "en", addr{st, "5"})
	signup(t, s, "maria@example.com", "bg", addr{st, "9"}, addr{st, "11"})
	signup(t, s, "unverified@example.com", "bg", addr{st, "1"})
	for _, m := range waitMail(t, s, 3)[:2] {
		verify(t, s, token(t, m))
	}
	pool, _ := pgxpool.New(context.Background(), s.DSN)
	defer pool.Close()
	// Alerts are produced by a later ticket; seed the table the statistics read.
	if _, err := pool.Exec(context.Background(), `INSERT INTO alerts (created_at, delivery) VALUES
		($1, 'delivered'), ($1, 'delivered'), ($1, 'failed'), ($1::timestamptz - interval '1 day', 'delivered')`, s.Clock.Now()); err != nil {
		t.Fatal(err)
	}

	tok := s.OIDC.Token(testkit.Claims{Roles: []string{"operator"}})
	code, body := do(t, "GET", s.URL+statsPath, tok, nil)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var got struct {
		Subscribers []struct {
			Settlement string `json:"settlement"`
			Count      int    `json:"count"`
		} `json:"subscribers_per_settlement"`
		Alerts []struct {
			Date      string `json:"date"`
			Sent      int    `json:"sent"`
			Delivered int    `json:"delivered"`
		} `json:"alerts_per_day"`
		DeliverySuccess float64 `json:"delivery_success"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Subscribers) != 1 || got.Subscribers[0].Settlement != "Варна" || got.Subscribers[0].Count != 2 {
		t.Errorf("subscribers = %+v", got.Subscribers)
	}
	if len(got.Alerts) != 2 || got.Alerts[1].Date != "2026-11-14" || got.Alerts[1].Sent != 3 || got.Alerts[1].Delivered != 2 {
		t.Errorf("alerts = %+v", got.Alerts)
	}
	if got.DeliverySuccess < 0.74 || got.DeliverySuccess > 0.76 {
		t.Errorf("delivery success = %v", got.DeliverySuccess)
	}
	for _, path := range []string{healthPath, statsPath} {
		_, b := do(t, "GET", s.URL+path, tok, nil)
		for _, pii := range []string{"ivan", "maria", "example.com", "Фердинанд", "Ferdinand"} {
			if strings.Contains(strings.ToLower(b), strings.ToLower(pii)) {
				t.Errorf("%s leaks %q: %s", path, pii, b)
			}
		}
	}
}

func TestBackofficeActionsAreWrittenToTheAuditLog(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	op := s.OIDC.Token(testkit.Claims{Roles: []string{"operator"}})
	do(t, "GET", s.URL+healthPath, op, nil)
	do(t, "GET", s.URL+statsPath, op, nil)
	do(t, "GET", s.URL+healthPath, "", nil) // rejected, so not an action
	po := s.OIDC.Token(testkit.Claims{Roles: []string{"privacy_officer"}})
	_, body := do(t, "GET", s.URL+auditPath, po, nil)
	var entries []struct {
		At     string `json:"at"`
		Actor  string `json:"actor"`
		Action string `json:"action"`
	}
	if err := json.Unmarshal([]byte(body), &entries); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	var actions []string
	for _, e := range entries {
		if e.Actor != "oscar" || e.At == "" {
			t.Errorf("entry %+v", e)
		}
		actions = append(actions, e.Action)
	}
	if strings.Join(actions, ",") != "view_provider_health,view_stats" {
		t.Errorf("actions = %v", actions)
	}
}
