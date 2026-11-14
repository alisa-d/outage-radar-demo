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
	Providers *httptest.Server // stub provider sites
	Claude    *httptest.Server // stub Claude API
	SMTP      *SMTPSink
	App       *app.App
}

// Start migrates a fresh database and runs the app against it.
func Start(t testing.TB, role string, handlers map[string]app.Handler) *System {
	t.Helper()
	dsn := NewDatabase(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := app.Migrate(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s := &System{
		DSN:       dsn,
		Clock:     clock.NewFake(time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)),
		Providers: httptest.NewServer(http.NotFoundHandler()),
		Claude:    httptest.NewServer(http.NotFoundHandler()),
		SMTP:      newSMTPSink(t),
	}
	t.Cleanup(s.Providers.Close)
	t.Cleanup(s.Claude.Close)
	a, err := app.Start(ctx, app.Config{Role: role, DatabaseURL: dsn, HTTPAddr: "127.0.0.1:0"}, s.Clock, handlers)
	if err != nil {
		t.Fatalf("start app: %v", err)
	}
	t.Cleanup(func() { cancel(); a.Wait() })
	s.App = a
	s.URL = "http://" + a.Addr()
	return s
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
