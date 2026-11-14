// Package testkit is the Seam 1 harness: the system started against real
// Postgres (testcontainers) with stubs for everything outside it.
package testkit

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alisa/outage-radar/internal/app"
	"github.com/alisa/outage-radar/internal/clock"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	pgOnce sync.Once
	pgDSN  string // DSN of the shared container's default database
	pgErr  error
	dbSeq  int
	dbMu   sync.Mutex
)

// NewDatabase returns the DSN of a fresh, empty database in a shared Postgres container.
func NewDatabase(t testing.TB) string {
	t.Helper()
	pgOnce.Do(func() {
		ctx := context.Background()
		c, err := tcpostgres.Run(ctx, "postgres:17-alpine",
			tcpostgres.WithDatabase("postgres"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("postgres"),
			testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)))
		if err != nil {
			pgErr = err
			return
		}
		pgDSN, pgErr = c.ConnectionString(ctx, "sslmode=disable")
	})
	if pgErr != nil {
		t.Fatalf("start postgres: %v", pgErr)
	}
	dbMu.Lock()
	dbSeq++
	name := fmt.Sprintf("test_%d", dbSeq)
	dbMu.Unlock()
	pool, err := pgxpool.New(context.Background(), pgDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(), "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	return strings.Replace(pgDSN, "/postgres?", "/"+name+"?", 1)
}

// System is a running app plus its stubbed surroundings.
type System struct {
	URL       string
	Clock     *clock.Fake
	DSN       string
	Providers *httptest.Server // stub provider sites; set the replayed responses with StubProviders
	Claude    *httptest.Server // stub Claude API
	SMTP      *SMTPSink
	OIDC      *OIDC // stub identity provider for the backoffice
	App       *app.App
	stub      atomic.Pointer[http.Handler]
	claude    atomic.Pointer[http.Handler]
}

// StubClaude sets what the stub Claude API answers (404 until called).
func (s *System) StubClaude(h http.Handler) { s.claude.Store(&h) }

// Start migrates a fresh database and runs the app against it.
func Start(t testing.TB, role string, handlers map[string]app.Handler) *System {
	t.Helper()
	return StartWith(t, role, handlers, nil)
}

// ValidTurnstileToken is the only token the stub Turnstile accepts.
const ValidTurnstileToken = "valid-token"

// StartWith is Start with a hook to adjust the Config (limits are generous by default).
func StartWith(t testing.TB, role string, handlers map[string]app.Handler, tweak func(*app.Config)) *System {
	t.Helper()
	return startAt(t, role, handlers, tweak, time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC))
}

// StartAt is Start with the fake clock set to now.
func StartAt(t testing.TB, role string, handlers map[string]app.Handler, now time.Time) *System {
	t.Helper()
	return startAt(t, role, handlers, nil, now)
}

func startAt(t testing.TB, role string, handlers map[string]app.Handler, tweak func(*app.Config), now time.Time) *System {
	t.Helper()
	dsn := NewDatabase(t)
	var s *System
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := app.Migrate(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s = &System{
		DSN:       dsn,
		Clock:     clock.NewFake(now),
		Providers: httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.providerHandler().ServeHTTP(w, r) })),
		Claude: httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if h := s.claude.Load(); h != nil {
				(*h).ServeHTTP(w, r)
				return
			}
			http.NotFound(w, r)
		})),
		SMTP: newSMTPSink(t),
	}
	s.OIDC = newOIDC(t, func() time.Time { return s.Clock.Now() })
	t.Cleanup(s.Providers.Close)
	t.Cleanup(s.Claude.Close)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		fmt.Fprintf(w, `{"success":%t}`, r.Form.Get("secret") == "test-secret" && r.Form.Get("response") == ValidTurnstileToken)
	}))
	t.Cleanup(ts.Close)
	cfg := app.Config{Role: role, DatabaseURL: dsn, HTTPAddr: "127.0.0.1:0",
		DataKey: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=", SMTPAddr: s.SMTP.Addr, MailFrom: "alerts@test.invalid", PublicURL: "http://app.test", ErpSeverURL: s.Providers.URL, VikVarnaURL: s.Providers.URL, AnthropicURL: s.Claude.URL, AnthropicKey: "test-key",
		OIDCIssuer: s.OIDC.URL, OIDCClientID: OIDCClientID,
		TurnstileSecret: "test-secret", TurnstileURL: ts.URL,
		IPLimit: 1000, ChannelLimit: 1000, AddressLimit: 1000, ChannelDailyCap: 1000, EmailDailyBudget: 1000}
	if tweak != nil {
		tweak(&cfg)
	}
	a, err := app.Start(ctx, cfg, s.Clock, handlers)
	if err != nil {
		t.Fatalf("start app: %v", err)
	}
	t.Cleanup(func() { cancel(); a.Wait() })
	s.App = a
	s.URL = "http://" + a.Addr()
	return s
}

// StubProviders sets what the stub provider sites answer (404 until called).
func (s *System) StubProviders(h http.Handler) { s.stub.Store(&h) }

func (s *System) providerHandler() http.Handler {
	if h := s.stub.Load(); h != nil {
		return *h
	}
	return http.NotFoundHandler()
}

// SMTPSink accepts mail on a local port and records the raw messages.
type SMTPSink struct {
	Addr string
	mu   sync.Mutex
	msgs []string
}

func (s *SMTPSink) Messages() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.msgs...)
}

func newSMTPSink(t testing.TB) *SMTPSink {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &SMTPSink{Addr: l.Addr().String()}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
	return s
}

func (s *SMTPSink) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	fmt.Fprint(c, "220 sink\r\n")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
		case strings.HasPrefix(cmd, "DATA"):
			fmt.Fprint(c, "354 go\r\n")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			s.mu.Lock()
			s.msgs = append(s.msgs, b.String())
			s.mu.Unlock()
			fmt.Fprint(c, "250 ok\r\n")
		case strings.HasPrefix(cmd, "QUIT"):
			fmt.Fprint(c, "221 bye\r\n")
			return
		default:
			fmt.Fprint(c, "250 ok\r\n")
		}
	}
}
