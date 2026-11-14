package app_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alisa/outage-radar/internal/app"
	"github.com/alisa/outage-radar/internal/testkit"
	"github.com/jackc/pgx/v5"
)

func startLimited(t *testing.T, tweak func(*app.Config)) (*testkit.System, int) {
	s := testkit.StartWith(t, "all", nil, tweak)
	importVarna(t, s)
	return s, streets(t, s, "10135", "ferdinand")[0].ID
}

func TestSignupWithoutValidTurnstileTokenIsRejected(t *testing.T) {
	s, st := started(t)
	for _, tok := range []any{nil, "forged"} {
		body := map[string]any{"email": "bot@example.com", "language": "en", "addresses": []addr{{st, "5"}}}
		if tok != nil {
			body["turnstile_token"] = tok
		}
		if code, _ := post(t, s.URL+"/api/subscriptions", body); code != 400 {
			t.Errorf("token %v: status %d, want 400", tok, code)
		}
	}
	waitMail(t, s, 0)
}

func TestRateLimitsReturn429AndSendNoEmail(t *testing.T) {
	for _, c := range []struct {
		name  string
		tweak func(*app.Config)
		email func(i int) string
	}{
		{"per IP", func(c *app.Config) { c.IPLimit = 1 }, func(i int) string { return fmt.Sprintf("u%d@example.com", i) }},
		{"per channel", func(c *app.Config) { c.ChannelLimit = 1 }, func(i int) string { return "same@example.com" }},
		{"per address", func(c *app.Config) { c.AddressLimit = 1 }, func(i int) string { return fmt.Sprintf("u%d@example.com", i) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, st := startLimited(t, c.tweak)
			if code, _ := signup(t, s, c.email(0), "en", addr{st, "5"}); code != 202 {
				t.Fatalf("first: %d", code)
			}
			if code, _ := signup(t, s, c.email(1), "en", addr{st, "5"}); code != 429 {
				t.Fatalf("second: %d, want 429", code)
			}
			waitMail(t, s, 1)
		})
	}
}

func TestVerifyIsRateLimitedPerIP(t *testing.T) {
	s, _ := startLimited(t, func(c *app.Config) { c.IPLimit = 1 })
	verify(t, s, "x.y")
	if code := verify(t, s, "x.y"); code != 429 {
		t.Fatalf("status %d, want 429", code)
	}
}

func TestDailyVerificationCapPerChannelResetsWithClock(t *testing.T) {
	s, st := startLimited(t, func(c *app.Config) { c.ChannelDailyCap = 2 })
	for i, want := range []int{202, 202, 429} {
		if code, _ := signup(t, s, "ivan@example.com", "en", addr{st, "5"}); code != want {
			t.Fatalf("attempt %d: %d, want %d", i, code, want)
		}
	}
	waitMail(t, s, 2)
	s.Clock.Advance(25 * time.Hour)
	if code, _ := signup(t, s, "ivan@example.com", "en", addr{st, "5"}); code != 202 {
		t.Fatalf("after reset: %d", code)
	}
	waitMail(t, s, 3)
}

func TestTrippedBreakerPausesNewVerificationsButNotVerifiedSubscribers(t *testing.T) {
	s, st := startLimited(t, func(c *app.Config) { c.EmailDailyBudget = 1 })
	signup(t, s, "ivan@example.com", "en", addr{st, "5"})
	tok := token(t, waitMail(t, s, 1)[0])
	for i := 0; i < 2; i++ {
		if code, _ := signup(t, s, "maria@example.com", "en", addr{st, "5"}); code != 503 {
			t.Fatalf("tripped signup: %d, want 503", code)
		}
	}
	waitMail(t, s, 1)
	if code := verify(t, s, tok); code != 204 { // a pending link still works
		t.Fatalf("verify while tripped: %d", code)
	}
	if m := metrics(t, s); !strings.Contains(m, `circuit_breaker_trips_total{channel="email"} 1`) {
		t.Errorf("metrics:\n%s", m)
	}
	s.Clock.Advance(25 * time.Hour)
	if code, _ := signup(t, s, "maria@example.com", "en", addr{st, "5"}); code != 202 {
		t.Fatalf("after reset: %d", code)
	}
}

func metrics(t *testing.T, s *testkit.System) string {
	resp, err := http.Get(s.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestRateLimitKeysHoldNoPlaintextPersonalData(t *testing.T) {
	s, st := started(t)
	signup(t, s, "ivan@example.com", "en", addr{st, "5"})
	conn, err := pgx.Connect(context.Background(), s.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	var dump string
	var n int
	if err := conn.QueryRow(context.Background(), `SELECT coalesce(string_agg(h::text, ' '), ''), count(*) FROM rate_hits h`).Scan(&dump, &n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("no rate-limit keys recorded")
	}
	for _, bad := range []string{"ivan", "example.com", "127.0.0.1"} {
		if strings.Contains(dump, bad) {
			t.Errorf("rate_hits contains %q", bad)
		}
	}
}
