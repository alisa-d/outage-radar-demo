package ingest

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrFormat marks a response that arrived but is not in the Provider's format
// (garbage, an HTML error page); adapters wrap it.
var ErrFormat = errors.New("unexpected response format")

// Health is a Provider's Provider Health as shown to residents.
type Health struct {
	State       string // healthy | stale | suspicious | unknown (no fetch has succeeded yet)
	LastSuccess *time.Time
}

// Healths derives every Provider's health at now, keyed by Provider name.
// Providers without an adapter are "unknown". Stale is 3x the polling interval since the last success.
func Healths(ctx context.Context, pool *pgxpool.Pool, now time.Time) (map[string]Health, error) {
	rows, err := pool.Query(ctx, `
		SELECT p.name, h.last_success_at, CASE
		  WHEN h.provider_id IS NULL THEN 'unknown'
		  WHEN $1 - coalesce(h.last_success_at, h.first_seen_at) > 3 * h.interval_s * interval '1 second' THEN 'stale'
		  WHEN h.suspicious THEN 'suspicious'
		  WHEN h.last_success_at IS NULL THEN 'unknown'
		  ELSE 'healthy' END
		FROM providers p LEFT JOIN provider_health h ON h.provider_id = p.id`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Health{}
	for rows.Next() {
		var name string
		var h Health
		if err := rows.Scan(&name, &h.LastSuccess, &h.State); err != nil {
			return nil, err
		}
		out[name] = h
	}
	return out, rows.Err()
}

// record stores the outcome of one poll. fetchErr non-nil means nothing was fetched.
func record(ctx context.Context, pool *pgxpool.Pool, providerID int, now time.Time, a Adapter, fetchErr error, notices, failed int) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO provider_health (provider_id, interval_s, first_seen_at) VALUES ($1, $2, $3)
		ON CONFLICT (provider_id) DO UPDATE SET interval_s = excluded.interval_s`, providerID, int(a.Interval().Seconds()), now)
	if err != nil {
		return err
	}
	if fetchErr != nil {
		// Garbage where Notices are normal is Suspicious; plain failures just let the Provider go Stale.
		_, err = pool.Exec(ctx, `UPDATE provider_health SET last_error = $2, suspicious = suspicious OR ($3 AND seen_notices) WHERE provider_id = $1`,
			providerID, fetchErr.Error(), errors.Is(fetchErr, ErrFormat))
		return err
	}
	_, err = pool.Exec(ctx, `
		UPDATE provider_health SET last_success_at = $2, last_error = NULL,
		  suspicious = ($3 = 0 AND seen_notices) OR ($4 > 0 AND $4 * 2 > $3),
		  seen_notices = seen_notices OR $3 > 0
		WHERE provider_id = $1`, providerID, now, notices, failed)
	return err
}
