package app_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alisa/outage-radar/internal/testkit"
)

func TestIngestRoleAloneExportsMetrics(t *testing.T) {
	s := testkit.Start(t, "ingest", nil)
	if s.App.Addr() != "" {
		t.Fatalf("ingest must not serve the API, got %q", s.App.Addr())
	}
	resp, err := http.Get(s.MetricsURL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), "outage_radar_provider_health{") {
		t.Errorf("metrics:\n%s", b)
	}
}

// The Seam 1 main flows: signup, verify, Alert delivery (one transient failure), bounce.
func TestLogsOfTheMainFlowsHoldNoPersonalData(t *testing.T) {
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	s, down := bounceWatcher(t)
	s.SMTP.FailNext(1)
	publish(s, down, hrow(1, intro+"ул. Дойран.", "9:00", "17:00"))
	time.Sleep(500 * time.Millisecond)
	s.Clock.Advance(time.Minute) // past the retry backoff
	waitMail(t, s, 2)
	brevoEvent(t, s, "Bearer "+brevoToken, "hard_bounce", "ivan@example.com")

	out := logs.String()
	if !strings.Contains(out, "alert delivery failed") {
		t.Fatalf("flows did not log what the test relies on:\n%s", out)
	}
	for _, bad := range []string{"ivan", "example.com", "@", "Дойран", "doyran", "43.2", "27.9", "lat", "lon"} {
		if strings.Contains(strings.ToLower(out), strings.ToLower(bad)) {
			t.Errorf("logs hold %q:\n%s", bad, out)
		}
	}
}
