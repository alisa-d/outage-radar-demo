package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Review queue and Outage correction (ticket 18). Everything here is Notice text, Outages and the Gazetteer: no
// Subscriber data is read, so an Operator can never see any. Corrections edit the stored Outage in place and
// enqueue an Alert scan, whose deduplication and material-change rules decide who is told.
// shortcut: a correction lives on the Outage row, so a later change of the Provider's Notice replaces it; re-apply by hand.

type httpErr struct {
	code int
	msg  string
}

func (e httpErr) Error() string { return e.msg }

// handleWith is backoffice.handle for endpoints that read the request and the Operator's name; fn returns the audit action.
func (b *backoffice) handleWith(role string, fn func(context.Context, *http.Request, principal) (action string, out any, err error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := b.authenticate(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !p.mfa || !slices.Contains(p.roles, role) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		action, out, err := fn(r.Context(), r, p)
		var he httpErr
		if errors.As(err, &he) {
			http.Error(w, he.msg, he.code)
			return
		}
		if err == nil {
			_, err = b.pool.Exec(r.Context(), `INSERT INTO audit_log (at, actor, action) VALUES ($1, $2, $3)`, b.clk.Now(), p.name, action)
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	}
}

func (b *backoffice) reviewRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /backoffice/api/review-queue", b.handleWith(roleOperator, b.reviewQueue))
	mux.HandleFunc("PATCH /backoffice/api/outages/{notice}/{ordinal}", b.handleWith(roleOperator, b.correctOutage))
	mux.HandleFunc("GET /backoffice/api/gazetteer", b.handleWith(roleOperator, b.listGazetteer))
	mux.HandleFunc("POST /backoffice/api/gazetteer", b.handleWith(roleOperator, b.putGazetteer))
}

type outageRow struct {
	NoticeID   int64     `json:"notice_id"`
	Ordinal    int       `json:"ordinal"`
	Notice     string    `json:"notice"`
	Kind       string    `json:"kind"`
	Severity   string    `json:"severity"`
	Start      time.Time `json:"-"`
	End        time.Time `json:"-"`
	StartS     string    `json:"start"`
	EndS       string    `json:"end"`
	Settlement string    `json:"settlement"`
	Places     []string  `json:"places"`
	Unlocated  bool      `json:"unlocated"`
	Unresolved []string  `json:"unresolved_places"`
}

// reviewQueue lists current city Outages that are Unlocated or name a place nothing resolves, beside the original Notice.
func (b *backoffice) reviewQueue(ctx context.Context, _ *http.Request, _ principal) (string, any, error) {
	rows, err := b.pool.Query(ctx, `SELECT o.notice_id, row_number() OVER (PARTITION BY o.notice_id ORDER BY o.id) - 1, n.text, o.kind, o.severity, o.start_at, o.end_at, o.settlement, o.places
		FROM outages o JOIN notices n ON n.id = o.notice_id WHERE o.end_at > $1 ORDER BY o.notice_id, o.id`, b.clk.Now())
	if err != nil {
		return "", nil, err
	}
	var all []outageRow
	for rows.Next() {
		var o outageRow
		if err := rows.Scan(&o.NoticeID, &o.Ordinal, &o.Notice, &o.Kind, &o.Severity, &o.Start, &o.End, &o.Settlement, &o.Places); err != nil {
			rows.Close()
			return "", nil, err
		}
		all = append(all, o)
	}
	rows.Close()
	out := []outageRow{}
	resolvers := map[string]*resolver{}
	for _, o := range all {
		rs, ok := resolvers[o.Settlement]
		if !ok {
			if rs, err = loadResolver(ctx, b.pool, o.Settlement); err != nil {
				return "", nil, err
			}
			resolvers[o.Settlement] = rs
		}
		if rs == nil || rs.village {
			continue
		}
		o.Unresolved = []string{}
		for _, p := range o.Places {
			if _, placed, err := matchPlace(rs, p, nil); err != nil {
				return "", nil, err
			} else if !placed {
				o.Unresolved = append(o.Unresolved, p)
			}
		}
		if _, o.Unlocated, err = matchOutage(rs, o.Settlement, o.Places, nil); err != nil {
			return "", nil, err
		}
		if o.Unlocated || len(o.Unresolved) > 0 {
			o.StartS, o.EndS = o.Start.In(sofia).Format(time.RFC3339), o.End.In(sofia).Format(time.RFC3339)
			out = append(out, o)
		}
	}
	return "view_review_queue", out, nil
}

type correction struct {
	Streets   []string     `json:"streets"`   // registry streets to add to the Outage's places
	Gazetteer []string     `json:"gazetteer"` // Gazetteer entries to add
	Ring      [][2]float64 `json:"ring"`      // a drawn area, [lon, lat]
	Start     *time.Time   `json:"start"`
	End       *time.Time   `json:"end"`
	Severity  *string      `json:"severity"`
}

func (b *backoffice) correctOutage(ctx context.Context, r *http.Request, _ principal) (string, any, error) {
	notice, err1 := strconv.ParseInt(r.PathValue("notice"), 10, 64)
	ordinal, err2 := strconv.Atoi(r.PathValue("ordinal"))
	var c correction
	if err1 != nil || err2 != nil || json.NewDecoder(r.Body).Decode(&c) != nil {
		return "", nil, httpErr{http.StatusBadRequest, "bad request"}
	}
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback(ctx)
	var id int64
	var settlement, severity string
	var places []string
	var start, end time.Time
	err = tx.QueryRow(ctx, `SELECT id, settlement, severity, places, start_at, end_at FROM outages WHERE notice_id = $1 ORDER BY id OFFSET $2 LIMIT 1 FOR UPDATE`, notice, ordinal).
		Scan(&id, &settlement, &severity, &places, &start, &end)
	if err == pgx.ErrNoRows {
		return "", nil, httpErr{http.StatusNotFound, "no such outage"}
	} else if err != nil {
		return "", nil, err
	}
	rs, err := loadResolver(ctx, b.pool, settlement)
	if err != nil {
		return "", nil, err
	}
	if rs == nil {
		return "", nil, httpErr{http.StatusUnprocessableEntity, "unknown settlement"}
	}
	if len(c.Streets)+len(c.Gazetteer)+len(c.Ring) > 0 {
		// The Operator has reviewed the places: names nothing resolves are dropped (the Notice text keeps them).
		kept := []string{}
		for _, p := range places {
			if _, placed, err := matchPlace(rs, p, nil); err != nil {
				return "", nil, err
			} else if placed {
				kept = append(kept, p)
			}
		}
		places = kept
	}
	var fields []string
	add := func(field, place string) error {
		if _, placed, err := matchPlace(rs, place, nil); err != nil {
			return err
		} else if !placed {
			return httpErr{http.StatusUnprocessableEntity, "unknown " + field + ": " + place}
		}
		places = append(places, place)
		return nil
	}
	for _, st := range c.Streets {
		if !streetPlace.MatchString(strings.TrimSpace(st)) {
			st = "ул. " + st
		}
		if err := add("street", st); err != nil {
			return "", nil, err
		}
		fields = append(fields, "streets")
	}
	for _, g := range c.Gazetteer {
		if err := add("gazetteer entry", g); err != nil {
			return "", nil, err
		}
		fields = append(fields, "gazetteer")
	}
	if len(c.Ring) > 0 {
		if len(c.Ring) < 3 {
			return "", nil, httpErr{http.StatusBadRequest, "ring needs 3 points"}
		}
		name := fmt.Sprintf("Корекция %d %d", notice, ordinal)
		if err := upsertGazetteer(ctx, tx, rs.id, name, "area", append(c.Ring, c.Ring[0])); err != nil {
			return "", nil, err
		}
		places = append(places, name)
		fields = append(fields, "ring")
	}
	if c.Start != nil {
		start = *c.Start
		fields = append(fields, "start")
	}
	if c.End != nil {
		end = *c.End
		fields = append(fields, "end")
	}
	if c.Severity != nil {
		if *c.Severity != "cut" && *c.Severity != "disturbance" {
			return "", nil, httpErr{http.StatusBadRequest, "bad severity"}
		}
		severity = *c.Severity
		fields = append(fields, "severity")
	}
	if len(fields) == 0 || !end.After(start) {
		return "", nil, httpErr{http.StatusBadRequest, "nothing to correct, or end not after start"}
	}
	if _, err := tx.Exec(ctx, `UPDATE outages SET places = $2, start_at = $3, end_at = $4, severity = $5 WHERE id = $1`, id, places, start, end, severity); err != nil {
		return "", nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO jobs (kind, payload) VALUES ('scan-alerts', $1)`, []byte(fmt.Sprint(notice))); err != nil {
		return "", nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("correct_outage notice=%d ordinal=%d fields=%s", notice, ordinal, strings.Join(slices.Compact(fields), ",")), map[string]bool{"ok": true}, nil
}

func upsertGazetteer(ctx context.Context, tx pgx.Tx, settlementID int, name, kind string, pts [][2]float64) error {
	geom, _ := json.Marshal(pts)
	_, err := tx.Exec(ctx, `INSERT INTO gazetteer (settlement_id, name, search, kind, geom) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (settlement_id, name) DO UPDATE SET search = EXCLUDED.search, kind = EXCLUDED.kind, geom = EXCLUDED.geom`,
		settlementID, name, areaKey(name), kind, geom)
	return err
}

func (b *backoffice) listGazetteer(ctx context.Context, r *http.Request, _ principal) (string, any, error) {
	rows, err := b.pool.Query(ctx, `SELECT g.name, g.kind FROM gazetteer g JOIN settlements s ON s.id = g.settlement_id WHERE lower(s.name) = lower($1) ORDER BY g.name`, r.URL.Query().Get("settlement"))
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	out := []map[string]string{}
	for rows.Next() {
		var n, k string
		if err := rows.Scan(&n, &k); err != nil {
			return "", nil, err
		}
		out = append(out, map[string]string{"name": n, "kind": k})
	}
	return "view_gazetteer", out, rows.Err()
}

// putGazetteer adds or replaces a Gazetteer entry; later Notices that name it resolve by themselves.
func (b *backoffice) putGazetteer(ctx context.Context, r *http.Request, _ principal) (string, any, error) {
	var in struct {
		Settlement, Name, Kind string
		Geom                   [][2]float64
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || strings.TrimSpace(in.Name) == "" ||
		in.Kind == "area" && len(in.Geom) < 4 || in.Kind == "point" && len(in.Geom) != 1 || in.Kind != "area" && in.Kind != "point" {
		return "", nil, httpErr{http.StatusBadRequest, "bad request"}
	}
	var sid int
	if err := b.pool.QueryRow(ctx, `SELECT id FROM settlements WHERE lower(name) = lower($1)`, in.Settlement).Scan(&sid); err != nil {
		return "", nil, httpErr{http.StatusUnprocessableEntity, "unknown settlement"}
	}
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback(ctx)
	if err := upsertGazetteer(ctx, tx, sid, in.Name, in.Kind, in.Geom); err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("put_gazetteer settlement=%s name=%s", in.Settlement, in.Name), map[string]bool{"ok": true}, tx.Commit(ctx)
}
