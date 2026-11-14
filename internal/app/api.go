package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/alisa/outage-radar/internal/ingest"

	"github.com/alisa/outage-radar/internal/clock"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const regionsQuery = `
SELECT coalesce(json_agg(json_build_object(
  'id', r.id, 'name', r.name,
  'settlements', (SELECT coalesce(json_agg(json_build_object('name', s.name, 'ekatte', s.ekatte, 'streets', EXISTS (SELECT 1 FROM streets st WHERE st.settlement_id = s.id)) ORDER BY s.id), '[]') FROM settlements s WHERE s.region_id = r.id),
  'providers',   (SELECT coalesce(json_agg(json_build_object('name', p.name, 'utility', p.utility, 'coverage', p.coverage) ORDER BY p.id), '[]') FROM providers p WHERE p.region_id = r.id)
) ORDER BY r.id), '[]') FROM regions r`

func newMux(pool *pgxpool.Pool, subs *subscriptions, clk clock.Clock, bo *backoffice) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/subscriptions", subs.subscribe)
	mux.HandleFunc("POST /api/verify", subs.verify)
	subs.routes(mux)
	mux.HandleFunc("POST /api/unsubscribe", subs.unsubscribe)
	mux.HandleFunc("POST /api/webhooks/brevo", subs.brevoWebhook)
	bo.routes(mux)
	mux.HandleFunc("GET /api/regions", func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		if err := pool.QueryRow(r.Context(), regionsQuery).Scan(&body); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		// Provider Health is derived from the clock, so it is added here rather than in SQL.
		hs, err := ingest.Healths(r.Context(), pool, clk.Now())
		var regions []map[string]any
		if err != nil || json.Unmarshal(body, &regions) != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		for _, reg := range regions {
			for _, p := range reg["providers"].([]any) {
				p := p.(map[string]any)
				h := hs[p["name"].(string)]
				health := map[string]any{"state": h.State, "last_success": nil}
				if h.LastSuccess != nil {
					health["last_success"] = h.LastSuccess.In(sofia).Format(time.RFC3339)
				}
				p["health"] = health
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(regions)
	})
	mux.HandleFunc("GET /api/settlements/{ekatte}/streets", func(w http.ResponseWriter, r *http.Request) {
		q := normalize(r.URL.Query().Get("q"))
		rows, err := pool.Query(r.Context(), `SELECT st.id, st.name FROM streets st JOIN settlements s ON s.id = st.settlement_id
			WHERE s.ekatte = $1 AND st.search LIKE '%' || replace(replace($2, '%', ''), '_', '') || '%' ORDER BY st.name LIMIT 10`, r.PathValue("ekatte"), q)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id int
			var name string
			if rows.Scan(&id, &name) == nil {
				out = append(out, map[string]any{"id": id, "name": name})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("GET /api/streets/{id}/numbers/{number}", func(w http.ResponseWriter, r *http.Request) {
		b, err := lookupNumber(r.Context(), pool, r.PathValue("id"), r.PathValue("number"))
		if errors.Is(err, errNotFound) {
			http.NotFound(w, r)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(b)
	})
	mux.HandleFunc("GET /api/check", addressCheck(pool, clk))
	mux.HandleFunc("GET /api/regions/{id}/outages", func(w http.ResponseWriter, r *http.Request) {
		body, err := regionOutages(r, pool, clk)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})
	return mux
}

var sofia = func() *time.Location {
	l, err := time.LoadLocation("Europe/Sofia")
	if err != nil {
		panic(err)
	}
	return l
}()

type outageJSON struct {
	ID             int64     `json:"id"`
	Provider       string    `json:"provider"`
	Utility        string    `json:"utility"`
	Kind           string    `json:"kind"`
	Severity       string    `json:"severity"`
	Start          string    `json:"start"`
	End            string    `json:"end"`
	EndApproximate bool      `json:"end_approximate"`
	Places         []string  `json:"places"`
	Notice         string    `json:"notice"`         // the Provider's original text, never translated
	Area           []feature `json:"area,omitempty"` // GeoJSON features with a confidence property
	Unlocated      bool      `json:"unlocated,omitempty"`
}

type settlementJSON struct {
	Name    string       `json:"name"`
	Outages []outageJSON `json:"outages"`
}

type dayJSON struct {
	Date        string           `json:"date"`
	Settlements []settlementJSON `json:"settlements"`
}

// outageSelect is the shared outage query head; callers append WHERE and ORDER BY.
const outageSelect = `SELECT o.id, p.name, p.utility, o.kind, o.severity, o.start_at, o.end_at, o.end_approximate, o.settlement, o.places, n.text
	FROM outages o JOIN notices n ON n.id = o.notice_id JOIN providers p ON p.id = o.provider_id `

// scanOutage reads one outageSelect row; the caller formats the times and places.
func scanOutage(rows pgx.Rows) (o outageJSON, start, end time.Time, settlement string, err error) {
	err = rows.Scan(&o.ID, &o.Provider, &o.Utility, &o.Kind, &o.Severity, &start, &end, &o.EndApproximate, &settlement, &o.Places, &o.Notice)
	return
}

// outageWindow is the 8 Sofia days starting at today's midnight: today through today+7.
func outageWindow(now time.Time) (from, to time.Time) {
	n := now.In(sofia)
	from = time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, sofia)
	return from, from.AddDate(0, 0, 8)
}

// regionOutages lists Outages starting today through today+7 (Europe/Sofia), by day and Settlement.
func regionOutages(r *http.Request, pool *pgxpool.Pool, clk clock.Clock) ([]byte, error) {
	en := r.URL.Query().Get("lang") == "en"
	from, to := outageWindow(clk.Now())
	rows, err := pool.Query(r.Context(), outageSelect+`
		WHERE p.region_id::text = $1 AND o.start_at >= $2 AND o.start_at < $3
		ORDER BY o.start_at, o.end_at, o.id`, r.PathValue("id"), from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	resolvers := map[string]*resolver{}
	byDay := map[string]map[string][]outageJSON{}
	for rows.Next() {
		o, start, end, settlement, err := scanOutage(rows)
		if err != nil {
			return nil, err
		}
		rs, ok := resolvers[settlement]
		if !ok {
			if rs, err = loadResolver(r.Context(), pool, settlement); err != nil {
				return nil, err
			}
			resolvers[settlement] = rs
		}
		if _, o.Unlocated, err = matchOutage(rs, settlement, o.Places, nil); err != nil {
			return nil, err
		}
		start, end = start.In(sofia), end.In(sofia)
		o.Area = outageArea(r.Context(), pool, settlement, o.Places)
		o.Start, o.End = start.Format(time.RFC3339), end.Format(time.RFC3339)
		if en {
			settlement = latin(settlement)
			for i, p := range o.Places {
				o.Places[i] = latin(p)
			}
		}
		date := start.Format("2006-01-02")
		if byDay[date] == nil {
			byDay[date] = map[string][]outageJSON{}
		}
		byDay[date][settlement] = append(byDay[date][settlement], o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	days := []dayJSON{}
	for date, bySettlement := range byDay {
		d := dayJSON{Date: date, Settlements: []settlementJSON{}}
		for name, outages := range bySettlement {
			d.Settlements = append(d.Settlements, settlementJSON{name, outages})
		}
		sort.Slice(d.Settlements, func(i, j int) bool { return d.Settlements[i].Name < d.Settlements[j].Name })
		days = append(days, d)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Date < days[j].Date })
	return json.Marshal(map[string]any{"days": days})
}
