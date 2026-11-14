package app

import (
	"context"
	"fmt"
	"io"
	"os/user"
	"time"

	"github.com/jackc/pgx/v5"
)

// SetProviderMode is the demo CLI switch (ticket 24): mode is "shadow" or "live".
// Going live sends no backlog: Shadow Mode Alerts never had delivery jobs.
func SetProviderMode(ctx context.Context, dsn, provider, mode, actor string) error {
	if mode != "shadow" && mode != "live" {
		return fmt.Errorf("mode must be shadow or live")
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE providers SET shadow = $2 WHERE name = $1`, provider, mode == "shadow")
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("unknown provider %q", provider)
	}
	if u, err := user.Current(); err == nil {
		actor += " (" + u.Username + ")"
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_log (at, actor, action) VALUES (now(), $1, $2)`, actor, "provider_mode "+provider+" "+mode); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ShadowAlerts prints the would-be Alerts of Shadow Mode without personal data:
// Outage (Notice and window), Settlement, count and Match Result.
func ShadowAlerts(ctx context.Context, dsn, provider string, w io.Writer) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, `SELECT p.name, a.notice_id, a.ordinal, a.start_at, a.end_at, o.settlement, a.match_result, count(*)
		FROM alerts a JOIN notices n ON n.id = a.notice_id JOIN providers p ON p.id = n.provider_id
		JOIN LATERAL (SELECT settlement FROM outages WHERE notice_id = a.notice_id ORDER BY id OFFSET a.ordinal LIMIT 1) o ON true
		WHERE a.shadow AND ($1 = '' OR p.name = $1)
		GROUP BY 1, 2, 3, 4, 5, 6, 7 ORDER BY 1, 2, 3, 4`, provider)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var p, settlement, match string
		var notice, count int64
		var ordinal int
		var start, end time.Time
		if err := rows.Scan(&p, &notice, &ordinal, &start, &end, &settlement, &match, &count); err != nil {
			return err
		}
		fmt.Fprintf(w, "%s\tnotice %d/%d\t%s - %s\t%s\t%s\t%d\n", p, notice, ordinal, start.In(sofia).Format("2006-01-02 15:04"), end.In(sofia).Format("2006-01-02 15:04"), settlement, match, count)
	}
	return rows.Err()
}
