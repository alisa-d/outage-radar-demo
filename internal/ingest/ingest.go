// Package ingest polls Provider adapters and keeps Notices and Outages in the
// database. It knows nothing about any one Provider.
package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/alisa/outage-radar/internal/clock"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Notice is a Provider's announcement as fetched: verbatim text plus the
// source's own fields.
type Notice struct {
	ExternalID string // the Provider's own ID
	Published  time.Time
	Text       string
	Fields     json.RawMessage
}

// Outage is one interruption extracted from a Notice.
type Outage struct {
	Start, End     time.Time
	EndApproximate bool
	Severity       string // cut | disturbance
	Kind           string // planned | emergency
	Settlement     string
	Places         []string
}

// Extractor turns a Notice into Outages. Parsers and the LLM both satisfy it.
type Extractor interface {
	Extract(n Notice) ([]Outage, error)
}

// Adapter is everything a Provider needs to plug in.
type Adapter interface {
	Name() string // must match providers.name
	Interval() time.Duration
	// Fetch returns the current Notices; use the client for the shared fetch policy.
	Fetch(ctx context.Context, c *http.Client) ([]Notice, error)
	// Extract is the Provider's deterministic parser.
	Extract(n Notice) ([]Outage, error)
}

type uaTransport struct{}

func (uaTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("User-Agent", "outage-radar/0.1 (+https://github.com/alisa/outage-radar)")
	return http.DefaultTransport.RoundTrip(r)
}

// shortcut: no response caching or failure backoff, a failed poll retries after the normal interval; add both if a Provider rate-limits us.
var client = &http.Client{Timeout: 30 * time.Second, Transport: uaTransport{}}

// Run polls every adapter on its interval (by the injected clock) until ctx ends.
func Run(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, m *Metrics, adapters []Adapter) {
	next := make([]time.Time, len(adapters))
	for {
		for i, a := range adapters {
			if now := clk.Now(); !now.Before(next[i]) {
				next[i] = now.Add(a.Interval())
				if err := Poll(ctx, pool, clk, m, a); err != nil {
					fmt.Fprintf(os.Stderr, "ingest %s: %v\n", a.Name(), err)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Poll fetches once, stores new or changed Notices with their Outages and records Provider Health.
func Poll(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, m *Metrics, a Adapter) error {
	var providerID int
	if err := pool.QueryRow(ctx, `SELECT id FROM providers WHERE name = $1`, a.Name()).Scan(&providerID); err != nil {
		return fmt.Errorf("provider: %w", err)
	}
	began := time.Now()
	notices, err := a.Fetch(ctx, client)
	m.fetched(ctx, a.Name(), time.Since(began), err)
	if err != nil {
		if rerr := record(ctx, pool, providerID, clk.Now(), a, err, 0, 0); rerr != nil {
			return rerr
		}
		return err
	}
	failed, unresolved := 0, 0
	for _, n := range notices {
		h := sha256.Sum256(append([]byte(n.Text), n.Fields...))
		hash := hex.EncodeToString(h[:])
		if known(ctx, pool, providerID, n.ExternalID, hash) {
			continue
		}
		outages, err := a.Extract(n)
		if err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "ingest %s notice %s: %v\n", a.Name(), n.ExternalID, err)
			continue
		}
		for _, o := range outages {
			var ok bool
			pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM settlements s JOIN providers p ON p.region_id = s.region_id WHERE p.id = $1 AND s.name = $2)`,
				providerID, o.Settlement).Scan(&ok)
			if !ok {
				unresolved++
			}
		}
		if err := store(ctx, pool, providerID, n, hash, outages); err != nil {
			return err
		}
	}
	m.parsed(ctx, a.Name(), len(notices), failed, unresolved)
	return record(ctx, pool, providerID, clk.Now(), a, nil, len(notices), failed)
}

func known(ctx context.Context, pool *pgxpool.Pool, providerID int, externalID, hash string) bool {
	var ok bool
	pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM notices WHERE provider_id = $1 AND external_id = $2 AND content_hash = $3)`,
		providerID, externalID, hash).Scan(&ok)
	return ok
}

func store(ctx context.Context, pool *pgxpool.Pool, providerID int, n Notice, hash string, outages []Outage) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id int64
	// A changed Notice keeps its row; its Outages are replaced.
	if err := tx.QueryRow(ctx, `INSERT INTO notices (provider_id, external_id, text, content_hash, published_at) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (provider_id, external_id) DO UPDATE SET text = excluded.text, content_hash = excluded.content_hash, published_at = excluded.published_at
		RETURNING id`, providerID, n.ExternalID, n.Text, hash, n.Published).Scan(&id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM outages WHERE notice_id = $1`, id); err != nil {
		return err
	}
	for _, o := range outages {
		if o.Places == nil {
			o.Places = []string{}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO outages (notice_id, provider_id, kind, severity, start_at, end_at, end_approximate, settlement, places)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, providerID, o.Kind, o.Severity, o.Start, o.End, o.EndApproximate, o.Settlement, o.Places); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
