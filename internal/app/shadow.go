package app

import (
	"context"
	"errors"
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
		return errBadMode
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
	if u, err := user.Current(); err == nil {
		actor += " (" + u.Username + ")"
	}
	if err := applyProviderMode(ctx, tx, provider, mode, actor); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

var (
	errBadMode         = errors.New("mode must be shadow or live")
	errUnknownProvider = errors.New("unknown provider")
)

// applyProviderMode flips the flag and writes the audit row inside the caller's transaction.
func applyProviderMode(ctx context.Context, tx pgx.Tx, provider, mode, actor string) error {
	if mode != "shadow" && mode != "live" {
		return errBadMode
	}
	tag, err := tx.Exec(ctx, `UPDATE providers SET shadow = $2 WHERE name = $1`, provider, mode == "shadow")
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w %q", errUnknownProvider, provider)
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_log (at, actor, action) VALUES (now(), $1, $2)`, actor, "provider_mode "+provider+" "+mode)
	return err
}

type shadowAlert struct {
	Provider    string    `json:"provider"`
	NoticeID    int64     `json:"notice_id"`
	Ordinal     int       `json:"ordinal"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	Settlement  string    `json:"settlement"`
	MatchResult string    `json:"match_result"`
	Count       int64     `json:"count"`
}

type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// shadowAlertRows lists the would-be Alerts of Shadow Mode without personal data: Outage (Notice and window),
// Settlement, count and Match Result.
func shadowAlertRows(ctx context.Context, q querier, provider string) ([]shadowAlert, error) {
	rows, err := q.Query(ctx, `SELECT p.name, a.notice_id, a.ordinal, a.start_at, a.end_at, o.settlement, a.match_result, count(*)
		FROM alerts a JOIN notices n ON n.id = a.notice_id JOIN providers p ON p.id = n.provider_id
		JOIN LATERAL (SELECT settlement FROM outages WHERE notice_id = a.notice_id ORDER BY id OFFSET a.ordinal LIMIT 1) o ON true
		WHERE a.shadow AND ($1 = '' OR p.name = $1)
		GROUP BY 1, 2, 3, 4, 5, 6, 7 ORDER BY 1, 2, 3, 4`, provider)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []shadowAlert{}
	for rows.Next() {
		var a shadowAlert
		if err := rows.Scan(&a.Provider, &a.NoticeID, &a.Ordinal, &a.Start, &a.End, &a.Settlement, &a.MatchResult, &a.Count); err != nil {
			return nil, err
		}
		a.Start, a.End = a.Start.In(sofia), a.End.In(sofia)
		out = append(out, a)
	}
	return out, rows.Err()
}

// ShadowAlerts prints the would-be Alerts of Shadow Mode (see shadowAlertRows).
func ShadowAlerts(ctx context.Context, dsn, provider string, w io.Writer) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	alerts, err := shadowAlertRows(ctx, conn, provider)
	if err != nil {
		return err
	}
	for _, a := range alerts {
		fmt.Fprintf(w, "%s\tnotice %d/%d\t%s - %s\t%s\t%s\t%d\n", a.Provider, a.NoticeID, a.Ordinal, a.Start.Format("2006-01-02 15:04"), a.End.Format("2006-01-02 15:04"), a.Settlement, a.MatchResult, a.Count)
	}
	return nil
}
