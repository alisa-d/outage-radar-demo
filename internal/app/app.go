// Package app wires the single binary: role selection, migrations, the job
// queue and the public API.
package app

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/alisa/outage-radar/internal/clock"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Config comes only from the environment (ADR 0002).
type Config struct {
	Role        string // api | ingest | notify | all
	DatabaseURL string
	HTTPAddr    string
}

func ConfigFromEnv() Config {
	get := func(k, def string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return def
	}
	return Config{Role: get("ROLE", "all"), DatabaseURL: os.Getenv("DATABASE_URL"), HTTPAddr: get("HTTP_ADDR", ":8080")}
}

// Handler processes one job; returning an error retries it with backoff.
type Handler func(ctx context.Context, payload []byte) error

// Migrate applies pending migrations; safe to run repeatedly and concurrently.
func Migrate(ctx context.Context, dsn string) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(42)`); err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name text PRIMARY KEY)`); err != nil {
		return err
	}
	files, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })
	for _, f := range files {
		var done bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = $1)`, f.Name()).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		sql, _ := migrationFS.ReadFile("migrations/" + f.Name())
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("%s: %w", f.Name(), err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, f.Name()); err != nil {
			tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Enqueue adds a job inside the caller's transaction: it exists only if tx commits.
func Enqueue(ctx context.Context, tx pgx.Tx, kind string, payload []byte) error {
	_, err := tx.Exec(ctx, `INSERT INTO jobs (kind, payload) VALUES ($1, $2)`, kind, payload)
	return err
}

type App struct {
	addr string
	wg   sync.WaitGroup
	pool *pgxpool.Pool
}

// Addr is the HTTP listen address, empty when the role does not serve HTTP.
func (a *App) Addr() string { return a.addr }

// Wait blocks until everything started has stopped (after ctx is cancelled).
func (a *App) Wait() { a.wg.Wait(); a.pool.Close() }

// Start runs the role's components until ctx is cancelled. clk may be nil.
func Start(ctx context.Context, cfg Config, clk clock.Clock, handlers map[string]Handler) (*App, error) {
	var serveAPI, runWorker bool
	switch cfg.Role {
	case "api":
		serveAPI = true
	case "ingest", "notify":
		runWorker = true
	case "all":
		serveAPI, runWorker = true, true
	default:
		return nil, fmt.Errorf("unknown ROLE %q (want api, ingest, notify or all)", cfg.Role)
	}
	if clk == nil {
		clk = clock.System()
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	a := &App{pool: pool}
	if serveAPI {
		l, err := net.Listen("tcp", cfg.HTTPAddr)
		if err != nil {
			pool.Close()
			return nil, err
		}
		a.addr = l.Addr().String()
		srv := &http.Server{Handler: newMux(pool)}
		a.wg.Add(2)
		go func() {
			defer a.wg.Done()
			if err := srv.Serve(l); !errors.Is(err, http.ErrServerClosed) {
				fmt.Fprintln(os.Stderr, "http:", err)
			}
		}()
		go func() {
			defer a.wg.Done()
			<-ctx.Done()
			srv.Close()
		}()
	}
	if runWorker {
		a.wg.Add(1)
		go func() { defer a.wg.Done(); work(ctx, pool, clk, handlers) }()
	}
	return a, nil
}

func work(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, handlers map[string]Handler) {
	kinds := make([]string, 0, len(handlers))
	for k := range handlers {
		kinds = append(kinds, k)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
		for runOne(ctx, pool, clk, kinds, handlers) {
		}
	}
}

// runOne claims one ready job (FOR UPDATE SKIP LOCKED), runs it and records the outcome.
func runOne(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, kinds []string, handlers map[string]Handler) bool {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false
	}
	defer tx.Rollback(ctx)
	var id int64
	var kind string
	var payload []byte
	var attempts int
	err = tx.QueryRow(ctx, `SELECT id, kind, payload, attempts FROM jobs
		WHERE done_at IS NULL AND kind = ANY($1) AND (run_at IS NULL OR run_at <= $2)
		ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED`, kinds, clk.Now()).Scan(&id, &kind, &payload, &attempts)
	if err != nil {
		return false
	}
	if herr := handlers[kind](ctx, payload); herr != nil {
		backoff := time.Duration(1<<min(attempts, 10)) * time.Second
		tx.Exec(ctx, `UPDATE jobs SET attempts = attempts + 1, run_at = $2 WHERE id = $1`, id, clk.Now().Add(backoff))
	} else {
		tx.Exec(ctx, `UPDATE jobs SET done_at = $2 WHERE id = $1`, id, clk.Now())
	}
	return tx.Commit(ctx) == nil
}
