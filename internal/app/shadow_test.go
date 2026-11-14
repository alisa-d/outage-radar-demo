package app_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alisa/outage-radar/internal/app"
	"github.com/alisa/outage-radar/internal/testkit"
	"github.com/jackc/pgx/v5"
)

const erp = "ЕРП Север"

func TestShadowModeRecordsAlertsWithoutSendingAndGoingLiveSendsNoBacklog(t *testing.T) {
	s, down := watcher(t, "en")
	ctx := context.Background()
	if err := app.SetProviderMode(ctx, s.DSN, erp, "shadow", "cli"); err != nil {
		t.Fatal(err)
	}
	publish(s, down, hrow(1, intro+"ул. Дойран.", "9:00", "17:00"))
	waitFor(t, func() bool { return scanned(t, s) })
	waitMail(t, s, 1) // only the verification mail

	var out bytes.Buffer
	if err := app.ShadowAlerts(ctx, s.DSN, erp, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Варна", "affected", "1"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("shadow-alerts lacks %q: %s", want, out.String())
		}
	}
	for _, pii := range []string{"ivan", "Дойран", "Doyran"} {
		if strings.Contains(out.String(), pii) {
			t.Errorf("personal data %q in output", pii)
		}
	}

	if err := app.SetProviderMode(ctx, s.DSN, erp, "live", "cli"); err != nil {
		t.Fatal(err)
	}
	s.Clock.Advance(16 * time.Minute)
	waitMail(t, s, 1) // no backlog
	publish(s, down, hrow(1, intro+"ул. Дойран.", "9:00", "17:00"), hrow(2, intro+"ул. Дойран.", "10:00", "18:00"))
	if m := waitMail(t, s, 2); len(m) != 2 {
		t.Fatalf("want exactly the new Outage's alert, got %d mails", len(m))
	}

	conn, _ := pgx.Connect(ctx, s.DSN)
	defer conn.Close(ctx)
	var n int
	conn.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE actor LIKE 'cli%'`).Scan(&n)
	if n != 2 {
		t.Errorf("want 2 audit rows, got %d", n)
	}
}

func TestBothDemoProvidersAreSeededInShadowMode(t *testing.T) {
	dsn := testkit.NewDatabase(t)
	if err := app.Migrate(context.Background(), dsn); err != nil {
		t.Fatal(err)
	}
	conn, _ := pgx.Connect(context.Background(), dsn)
	defer conn.Close(context.Background())
	var live int
	conn.QueryRow(context.Background(), `SELECT count(*) FROM providers WHERE NOT shadow`).Scan(&live)
	if live != 0 {
		t.Errorf("%d providers live", live)
	}
}
