package app_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alisa/outage-radar/internal/app"
	"github.com/alisa/outage-radar/internal/testkit"
	"github.com/jackc/pgx/v5"
)

const brevoToken = "brevo-secret"

// brevoEvent posts a transactional webhook payload in Brevo's documented format.
func brevoEvent(t *testing.T, s *testkit.System, auth, event, email string) int {
	t.Helper()
	body := `{"id":1,"email":"` + email + `","message-id":"<202611140900.1@smtp-relay.mailin.fr>","date":"2026-11-14 09:00:00","ts":1794819600,"ts_event":1794819600,"event":"` + event + `","reason":"mailbox unavailable","tag":"","ts_epoch":1794819600000}`
	req, _ := http.NewRequest("POST", s.URL+"/api/webhooks/brevo", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func bounceWatcher(t *testing.T) (*testkit.System, *atomic.Bool) {
	t.Helper()
	s := testkit.StartWith(t, "all", nil, func(c *app.Config) { c.BrevoWebhookToken = brevoToken })
	importVarna(t, s)
	signup(t, s, "ivan@example.com", "en", addr{streets(t, s, "10135", "doyran")[0].ID, "11"})
	if c := verify(t, s, token(t, waitMail(t, s, 1)[0])); c != 204 {
		t.Fatalf("verify %d", c)
	}
	return s, new(atomic.Bool)
}

func metricsText(t *testing.T, s *testkit.System) string {
	t.Helper()
	resp, err := http.Get(s.MetricsURL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestHardBounceDisablesTheChannelAndIsCountedWithoutPersonalData(t *testing.T) {
	s, down := bounceWatcher(t)
	if c := brevoEvent(t, s, "Bearer "+brevoToken, "hard_bounce", "Ivan@Example.com"); c != 200 {
		t.Fatalf("webhook %d", c)
	}
	publish(s, down, hrow(1, intro+"ул. Дойран.", "9:00", "17:00"))
	waitFor(t, func() bool { return scanned(t, s) })
	waitMail(t, s, 1) // only the verification email, no Alert
	if m := metricsText(t, s); !strings.Contains(m, `delivery_failures_total{channel="email",event="hard_bounce"} 1`) {
		t.Errorf("metrics: %s", m)
	}
	conn, err := pgx.Connect(context.Background(), s.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	var dump string
	if err := conn.QueryRow(context.Background(), `SELECT coalesce(string_agg(e::text, ' '), '') FROM delivery_events e`).Scan(&dump); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"ivan", "example.com", "mailbox"} {
		if strings.Contains(strings.ToLower(dump+metricsText(t, s)), bad) {
			t.Errorf("event record holds %q", bad)
		}
	}
}

func TestComplaintDisablesTheChannel(t *testing.T) {
	s, down := bounceWatcher(t)
	if c := brevoEvent(t, s, "Bearer "+brevoToken, "spam", "ivan@example.com"); c != 200 {
		t.Fatalf("webhook %d", c)
	}
	publish(s, down, hrow(1, intro+"ул. Дойран.", "9:00", "17:00"))
	waitFor(t, func() bool { return scanned(t, s) })
	waitMail(t, s, 1)
	if m := metricsText(t, s); !strings.Contains(m, `delivery_failures_total{channel="email",event="complaint"} 1`) {
		t.Errorf("metrics: %s", m)
	}
}

func TestUnauthenticatedWebhookCallsAreRejectedAndChangeNothing(t *testing.T) {
	s, down := bounceWatcher(t)
	for _, auth := range []string{"", "Bearer wrong", "brevo-secret"} {
		if c := brevoEvent(t, s, auth, "hard_bounce", "ivan@example.com"); c != 401 {
			t.Errorf("auth %q: %d", auth, c)
		}
	}
	publish(s, down, hrow(1, intro+"ул. Дойран.", "9:00", "17:00"))
	waitMail(t, s, 2) // still alerted
	if m := metricsText(t, s); strings.Contains(m, "delivery_failures_total{") {
		t.Errorf("rejected calls were counted: %s", m)
	}
}

func TestWebhookIsRejectedWhenNoTokenIsConfigured(t *testing.T) {
	s := testkit.Start(t, "all", nil)
	if c := brevoEvent(t, s, "Bearer ", "hard_bounce", "ivan@example.com"); c != 401 {
		t.Errorf("got %d", c)
	}
}

func TestSoftBounceIsCountedButKeepsTheChannel(t *testing.T) {
	s, down := bounceWatcher(t)
	if c := brevoEvent(t, s, "Bearer "+brevoToken, "soft_bounce", "ivan@example.com"); c != 200 {
		t.Fatalf("webhook %d", c)
	}
	publish(s, down, hrow(1, intro+"ул. Дойран.", "9:00", "17:00"))
	waitMail(t, s, 2)
	if m := metricsText(t, s); !strings.Contains(m, `delivery_failures_total{channel="email",event="soft_bounce"} 1`) {
		t.Errorf("metrics: %s", m)
	}
}
