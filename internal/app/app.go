// Package app wires the single binary: role selection, migrations, the job
// queue and the public API.
package app

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alisa/outage-radar/internal/clock"
	"github.com/alisa/outage-radar/internal/ingest"
	"github.com/alisa/outage-radar/internal/ingest/erpsever"
	"github.com/alisa/outage-radar/internal/ingest/llm"
	"github.com/alisa/outage-radar/internal/ingest/vikvarna"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Config comes only from the environment (ADR 0002).
type Config struct {
	Role        string // api | ingest | notify | all
	DatabaseURL string
	HTTPAddr    string
	MetricsAddr string // every role serves /metrics here, never on the public listener
	ErpSeverURL string // base URL of the ЕРП Север site
	VikVarnaURL string // base URL of the ВиК Варна site

	AnthropicURL string // base URL of the Claude API
	AnthropicKey string
	DataKey      string // base64 32-byte key-encryption key (env key provider)
	SMTPAddr     string
	SMTPUser     string
	SMTPPass     string
	MailFrom     string
	PublicURL    string // base of links in emails

	OIDCIssuer   string // backoffice sign-in; empty disables the backoffice API
	OIDCClientID string // expected audience of ID tokens

	TurnstileSecret  string
	TurnstileURL     string // siteverify endpoint
	IPLimit          int    // signup or verify requests per IP per hour
	ChannelLimit     int    // signups per Contact Channel per hour
	AddressLimit     int    // signups per Address per hour
	ChannelDailyCap  int    // verification attempts per Contact Channel per day
	EmailDailyBudget int    // circuit breaker: verification emails per day, all channels

	BrevoWebhookToken string // bearer token Brevo sends with webhooks; empty rejects every call

	TrustedProxies []*net.IPNet // peers whose X-Forwarded-For is believed; empty = the TCP peer is the client
}

// parseCIDRs reads a comma-separated CIDR list; an entry that does not parse is dropped, which trusts less.
func parseCIDRs(v string) []*net.IPNet {
	var out []*net.IPNet
	for _, f := range strings.Split(v, ",") {
		if _, n, err := net.ParseCIDR(strings.TrimSpace(f)); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func ConfigFromEnv() Config {
	get := func(k, def string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return def
	}
	num := func(k string, def int) int {
		if n, err := strconv.Atoi(os.Getenv(k)); err == nil {
			return n
		}
		return def
	}
	return Config{TurnstileSecret: os.Getenv("TURNSTILE_SECRET"), TurnstileURL: get("TURNSTILE_URL", "https://challenges.cloudflare.com/turnstile/v0/siteverify"),
		IPLimit: num("LIMIT_IP", 20), ChannelLimit: num("LIMIT_CHANNEL", 3), AddressLimit: num("LIMIT_ADDRESS", 30),
		ChannelDailyCap: num("CAP_CHANNEL_DAILY", 5), EmailDailyBudget: num("BUDGET_EMAIL_DAILY", 100000),
		OIDCIssuer: os.Getenv("OIDC_ISSUER"), OIDCClientID: get("OIDC_CLIENT_ID", "backoffice"),
		Role: get("ROLE", "all"), DatabaseURL: os.Getenv("DATABASE_URL"), HTTPAddr: get("HTTP_ADDR", ":8080"), MetricsAddr: get("METRICS_ADDR", ":9100"),
		DataKey: os.Getenv("DATA_KEY"), SMTPAddr: get("SMTP_ADDR", "localhost:1025"), SMTPUser: os.Getenv("SMTP_USER"), SMTPPass: os.Getenv("SMTP_PASS"),
		MailFrom: get("MAIL_FROM", "alerts@outage-radar.alisa-d.dev"), PublicURL: get("PUBLIC_URL", "http://localhost:8081"),
		ErpSeverURL: get("ERPSEVER_BASE_URL", "https://www.erpsever.bg"),
		VikVarnaURL: get("VIKVARNA_BASE_URL", "https://vikvarna.com"), AnthropicURL: get("ANTHROPIC_BASE_URL", "https://api.anthropic.com"), AnthropicKey: os.Getenv("ANTHROPIC_API_KEY"), BrevoWebhookToken: os.Getenv("BREVO_WEBHOOK_TOKEN"), TrustedProxies: parseCIDRs(os.Getenv("TRUSTED_PROXIES"))}
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
	addr, metricsAddr string
	wg                sync.WaitGroup
	pool              *pgxpool.Pool
}

// Addr is the HTTP listen address, empty when the role does not serve HTTP.
func (a *App) Addr() string { return a.addr }

// MetricsAddr is where /metrics is served (every role).
func (a *App) MetricsAddr() string { return a.metricsAddr }

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
	kp, err := NewEnvKeyProvider(cfg.DataKey)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	subs := &subscriptions{pool: pool, kp: kp, clk: clk, cfg: cfg}
	handlers = maps.Clone(handlers)
	if handlers == nil {
		handlers = map[string]Handler{}
	}
	handlers[verifyEmailJob] = func(ctx context.Context, p []byte) error { return subs.sendVerifyEmail(ctx, p) }
	handlers[scanAlertsJob] = func(ctx context.Context, p []byte) error { return subs.scanAlerts(ctx, p) }
	handlers[sendAlertJob] = func(ctx context.Context, p []byte) error { return subs.sendAlert(ctx, p) }
	reg := prometheus.NewRegistry()
	exp, err := otelprom.New(otelprom.WithRegisterer(reg), otelprom.WithoutScopeInfo(), otelprom.WithoutTargetInfo())
	if err != nil {
		pool.Close()
		return nil, err
	}
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exp)).Meter("outage-radar")
	metrics, err := ingest.NewMetrics(meter, pool, clk)
	if err == nil {
		subs.trips, err = meter.Int64Counter("circuit_breaker_trips", metric.WithDescription("Signup circuit-breaker trips"))
	}
	if err == nil {
		subs.trips.Add(ctx, 0, metric.WithAttributes(attribute.String("channel", "email"))) // export 0 so the first trip is an increase
		err = subs.registerDeliveryMetrics(meter)
	}
	if err != nil {
		pool.Close()
		return nil, err
	}
	ml, err := net.Listen("tcp", cfg.MetricsAddr)
	if err != nil {
		pool.Close()
		return nil, err
	}
	a := &App{pool: pool, metricsAddr: ml.Addr().String()}
	msrv := &http.Server{Handler: promhttp.HandlerFor(reg, promhttp.HandlerOpts{DisableCompression: true})}
	a.wg.Add(2)
	go func() { defer a.wg.Done(); msrv.Serve(ml) }()
	go func() { defer a.wg.Done(); <-ctx.Done(); msrv.Close() }()
	if serveAPI {
		l, err := net.Listen("tcp", cfg.HTTPAddr)
		if err != nil {
			pool.Close()
			return nil, err
		}
		a.addr = l.Addr().String()
		srv := &http.Server{Handler: newMux(pool, subs, clk, &backoffice{pool: pool, subs: subs, clk: clk, cfg: cfg})}
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
		if cfg.Role != "notify" {
			// Registering a Provider is adding its adapter here.
			adapters := []ingest.Adapter{erpsever.New(cfg.ErpSeverURL),
				vikvarna.New(cfg.VikVarnaURL, llm.New(pool, cfg.AnthropicURL, cfg.AnthropicKey))}
			a.wg.Add(1)
			go func() { defer a.wg.Done(); ingest.Run(ctx, pool, clk, metrics, adapters) }()
		}
	}
	return a, nil
}

func work(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, handlers map[string]Handler) {
	kinds := make([]string, 0, len(handlers))
	for k := range handlers {
		kinds = append(kinds, k)
	}
	var lastPurge time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
		if now := clk.Now(); now.Sub(lastPurge) >= time.Minute {
			lastPurge = now
			purgeUnverified(ctx, pool, clk)
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
