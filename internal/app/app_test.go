package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/alisa/outage-radar/internal/app"
	"github.com/alisa/outage-radar/internal/testkit"
	"github.com/jackc/pgx/v5/pgxpool"
)

type region struct {
	Name        string `json:"name"`
	Settlements []struct {
		Name   string `json:"name"`
		Ekatte string `json:"ekatte"`
	} `json:"settlements"`
	Providers []struct {
		Name     string `json:"name"`
		Utility  string `json:"utility"`
		Coverage string `json:"coverage"`
	} `json:"providers"`
}

func TestRegionsAPIReturnsVarnaWithSettlementsProvidersAndCoverage(t *testing.T) {
	s := testkit.Start(t, "all", nil)

	resp, err := http.Get(s.URL + "/api/regions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var regions []region
	if err := json.NewDecoder(resp.Body).Decode(&regions); err != nil {
		t.Fatal(err)
	}
	if len(regions) != 1 || regions[0].Name != "Варна" {
		t.Fatalf("regions = %+v", regions)
	}
	r := regions[0]
	if len(r.Settlements) == 0 || r.Settlements[0].Name != "Варна" || r.Settlements[0].Ekatte != "10135" {
		t.Fatalf("settlements = %+v", r.Settlements)
	}
	got := map[string]string{}
	for _, p := range r.Providers {
		got[p.Name+"/"+p.Utility] = p.Coverage
	}
	if got["ВиК Варна/water"] == "" || got["ЕРП Север/power"] != "planned outages only" {
		t.Fatalf("providers = %+v", r.Providers)
	}
}

func TestRolesAreSelectedOnlyByEnvironment(t *testing.T) {
	cases := []struct {
		role    string
		wantAPI bool
	}{{"api", true}, {"all", true}, {"ingest", false}, {"notify", false}}
	for _, c := range cases {
		t.Run(c.role, func(t *testing.T) {
			t.Setenv("ROLE", c.role)
			t.Setenv("HTTP_ADDR", "127.0.0.1:0")
			t.Setenv("DATABASE_URL", testkit.NewDatabase(t))
			cfg := app.ConfigFromEnv()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := app.Migrate(ctx, cfg.DatabaseURL); err != nil {
				t.Fatal(err)
			}
			a, err := app.Start(ctx, cfg, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { cancel(); a.Wait() }()
			if (a.Addr() != "") != c.wantAPI {
				t.Fatalf("role %s: serving HTTP = %v, want %v", c.role, a.Addr() != "", c.wantAPI)
			}
		})
	}
	if _, err := app.Start(context.Background(), app.Config{Role: "bogus"}, nil, nil); err == nil {
		t.Fatal("unknown role must be rejected")
	}
}

func TestMigrateIsRepeatable(t *testing.T) {
	dsn := testkit.NewDatabase(t)
	for i := 0; i < 2; i++ {
		if err := app.Migrate(context.Background(), dsn); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}

func TestJobEnqueuedInTransactionIsProcessedOnlyIfItCommits(t *testing.T) {
	done := make(chan string, 4)
	s := testkit.Start(t, "all", map[string]app.Handler{
		"probe": func(ctx context.Context, payload []byte) error { done <- string(payload); return nil },
	})
	pool, err := pgxpool.New(context.Background(), s.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	for _, c := range []struct {
		payload string
		commit  bool
	}{{"rolled-back", false}, {"committed", true}} {
		tx, _ := pool.Begin(context.Background())
		if err := app.Enqueue(context.Background(), tx, "probe", []byte(c.payload)); err != nil {
			t.Fatal(err)
		}
		if c.commit {
			tx.Commit(context.Background())
		} else {
			tx.Rollback(context.Background())
		}
	}
	select {
	case got := <-done:
		if got != "committed" {
			t.Fatalf("processed %q first", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("committed job was never processed")
	}
	select {
	case got := <-done:
		t.Fatalf("unexpected extra job %q", got)
	case <-time.After(500 * time.Millisecond):
	}
}
