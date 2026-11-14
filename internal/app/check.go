package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/alisa/outage-radar/internal/clock"
	"github.com/alisa/outage-radar/internal/ingest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Match Results (GLOSSARY). noData is decided by Provider Health, not by matching.
const (
	affected    = "affected"
	possibly    = "possibly_affected"
	notAffected = "not_affected"
	noData      = "no_data"
)

var (
	streetPlace = regexp.MustCompile(`^(част от )?(?:ул|бул|пл)\.\s*(.+)$`)
	numberToken = regexp.MustCompile(`^\d+[\p{L}]?$`)
)

// matchPlace is Seam 3 for one place description of an Outage in the Address's Settlement.
// shortcut: only "ул./бул./пл. Name [numbers]" and "част от ул. Name" are resolved; quarters,
// landmarks, transformer stations and prose are Not affected here until ticket 08 adds the Gazetteer.
func matchPlace(place, street, number string) string {
	part, name, listed, ok := parsePlace(place)
	if !ok || normalize(name) != normalize(street) {
		return notAffected
	}
	for _, n := range listed {
		if strings.EqualFold(n, number) {
			return affected
		}
	}
	switch {
	case len(listed) > 0:
		return notAffected // numbers are listed and this one is not among them
	case part:
		return possibly
	}
	return affected
}

func matchOutage(settlement string, places []string, addrSettlement, street, number string) string {
	if !strings.EqualFold(strings.TrimSpace(settlement), addrSettlement) {
		return notAffected
	}
	best := notAffected
	for _, p := range places {
		switch matchPlace(p, street, number) {
		case affected:
			return affected
		case possibly:
			best = possibly
		}
	}
	return best
}

type utilityCheck struct {
	Utility  string       `json:"utility"`
	Provider string       `json:"provider"`
	Coverage string       `json:"coverage"`
	Result   string       `json:"result"`
	Outages  []outageJSON `json:"outages"`
}

// addressCheck answers "is this Address affected?" per Utility. It only reads: an anonymous
// check is never stored or logged.
func addressCheck(pool *pgxpool.Pool, clk clock.Clock) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fail := func(err error) {
			if errors.Is(err, errNotFound) || errors.Is(err, pgx.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		q := r.URL.Query()
		var street, settlement string
		var regionID int
		if err := pool.QueryRow(r.Context(), `SELECT st.name, s.name, s.region_id FROM streets st JOIN settlements s ON s.id = st.settlement_id
			WHERE st.id::text = $1`, q.Get("street")).Scan(&street, &settlement, &regionID); err != nil {
			fail(err)
			return
		}
		b, err := lookupNumber(r.Context(), pool, q.Get("street"), q.Get("number"))
		if err != nil {
			fail(err)
			return
		}
		now := clk.Now()
		hs, err := ingest.Healths(r.Context(), pool, now)
		if err != nil {
			fail(err)
			return
		}
		en := q.Get("lang") == "en"
		prows, err := pool.Query(r.Context(), `SELECT id, name, utility, coverage FROM providers WHERE region_id = $1 ORDER BY id`, regionID)
		if err != nil {
			fail(err)
			return
		}
		type prov struct {
			id                      int
			name, utility, coverage string
		}
		var provs []prov
		for prows.Next() {
			var p prov
			if err := prows.Scan(&p.id, &p.name, &p.utility, &p.coverage); err != nil {
				prows.Close()
				fail(err)
				return
			}
			provs = append(provs, p)
		}
		prows.Close()

		sofiaNow := now.In(sofia)
		to := time.Date(sofiaNow.Year(), sofiaNow.Month(), sofiaNow.Day(), 0, 0, 0, 0, sofia).AddDate(0, 0, 8)
		out := []utilityCheck{}
		for _, p := range provs {
			u := utilityCheck{Utility: p.utility, Provider: p.name, Coverage: p.coverage, Result: noData, Outages: []outageJSON{}}
			// Only a Healthy Provider can say "not affected"; otherwise silence means nothing.
			if hs[p.name].State == "healthy" {
				u.Result = notAffected
				rows, err := pool.Query(r.Context(), `
					SELECT o.id, o.kind, o.severity, o.start_at, o.end_at, o.end_approximate, o.settlement, o.places, n.text
					FROM outages o JOIN notices n ON n.id = o.notice_id
					WHERE o.provider_id = $1 AND o.end_at > $2 AND o.start_at < $3 ORDER BY o.start_at, o.id`, p.id, now, to)
				if err != nil {
					fail(err)
					return
				}
				for rows.Next() {
					o := outageJSON{Provider: p.name, Utility: p.utility}
					var start, end time.Time
					var os string
					if err := rows.Scan(&o.ID, &o.Kind, &o.Severity, &start, &end, &o.EndApproximate, &os, &o.Places, &o.Notice); err != nil {
						rows.Close()
						fail(err)
						return
					}
					m := matchOutage(os, o.Places, settlement, street, b.Number)
					if m == notAffected {
						continue
					}
					o.Start, o.End = start.In(sofia).Format(time.RFC3339), end.In(sofia).Format(time.RFC3339)
					if en {
						for i, pl := range o.Places {
							o.Places[i] = latin(pl)
						}
					}
					u.Outages = append(u.Outages, o)
					if m == affected || u.Result == notAffected {
						u.Result = m
					}
				}
				rows.Close()
				if err := rows.Err(); err != nil {
					fail(err)
					return
				}
			}
			out = append(out, u)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(map[string]any{
			"address":   map[string]any{"settlement": settlement, "street": street, "number": b.Number, "approximate": b.Approximate},
			"utilities": out,
		})
	}
}
