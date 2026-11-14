package app

import (
	"encoding/json"
	"errors"
	"net/http"
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

type utilityCheck struct {
	Utility  string `json:"utility"`
	Provider string `json:"provider"`
	Coverage string `json:"coverage"`
	Result   string `json:"result"`
	// Health is the Provider's state; Affected with a non-healthy Provider means the list may be incomplete.
	Health  string       `json:"provider_health"`
	Outages []outageJSON `json:"outages"`
	// Unlocated lists the Settlement's Unlocated Outages: shown, never a Match Result (ADR 0003).
	Unlocated []outageJSON `json:"unlocated"`
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
		bldg, err := lookupNumber(r.Context(), pool, q.Get("street"), q.Get("number"))
		if err != nil {
			fail(err)
			return
		}
		res, err := loadResolver(r.Context(), pool, settlement)
		if err != nil {
			fail(err)
			return
		}
		at := &target{street: street, number: bldg.Number, pt: [2]float64{bldg.Lon, bldg.Lat}}
		now := clk.Now()
		healths, err := ingest.Healths(r.Context(), pool, now)
		if err != nil {
			fail(err)
			return
		}
		en := q.Get("lang") == "en"
		providerRows, err := pool.Query(r.Context(), `SELECT id, name, utility, coverage FROM providers WHERE region_id = $1 ORDER BY id`, regionID)
		if err != nil {
			fail(err)
			return
		}
		type prov struct {
			id                      int
			name, utility, coverage string
		}
		var providers []prov
		for providerRows.Next() {
			var p prov
			if err := providerRows.Scan(&p.id, &p.name, &p.utility, &p.coverage); err != nil {
				providerRows.Close()
				fail(err)
				return
			}
			providers = append(providers, p)
		}
		providerRows.Close()

		_, to := outageWindow(now)
		out := []utilityCheck{}
		for _, p := range providers {
			health := healths[p.name].State
			u := utilityCheck{Utility: p.utility, Provider: p.name, Coverage: p.coverage, Result: noData, Health: health, Outages: []outageJSON{}, Unlocated: []outageJSON{}}
			// Only a Healthy Provider can say "not affected"; otherwise silence means nothing.
			// Outages already known stay valid either way (spec decision 8, story 43).
			if health == "healthy" {
				u.Result = notAffected
			}
			rows, err := pool.Query(r.Context(), outageSelect+`
				WHERE o.provider_id = $1 AND o.end_at > $2 AND o.start_at < $3 ORDER BY o.start_at, o.id`, p.id, now, to)
			if err != nil {
				fail(err)
				return
			}
			for rows.Next() {
				o, start, end, settlement, err := scanOutage(rows)
				if err != nil {
					rows.Close()
					fail(err)
					return
				}
				m, unlocated, err := matchOutage(res, settlement, o.Places, at)
				if err != nil {
					rows.Close()
					fail(err)
					return
				}
				if m == notAffected && !unlocated {
					continue
				}
				o.Start, o.End = start.In(sofia).Format(time.RFC3339), end.In(sofia).Format(time.RFC3339)
				if en {
					for i, pl := range o.Places {
						o.Places[i] = latin(pl)
					}
				}
				if unlocated {
					u.Unlocated = append(u.Unlocated, o)
					continue
				}
				u.Outages = append(u.Outages, o)
				if m == affected || u.Result == notAffected || u.Result == noData {
					u.Result = m
				}
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				fail(err)
				return
			}
			out = append(out, u)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(map[string]any{
			"address":   map[string]any{"settlement": settlement, "street": street, "number": bldg.Number, "approximate": bldg.Approximate},
			"utilities": out,
		})
	}
}
