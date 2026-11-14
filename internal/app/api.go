package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/alisa/outage-radar/internal/clock"
	"github.com/jackc/pgx/v5/pgxpool"
)

const regionsQuery = `
SELECT coalesce(json_agg(json_build_object(
  'id', r.id, 'name', r.name,
  'settlements', (SELECT coalesce(json_agg(json_build_object('name', s.name, 'ekatte', s.ekatte, 'streets', EXISTS (SELECT 1 FROM streets st WHERE st.settlement_id = s.id)) ORDER BY s.id), '[]') FROM settlements s WHERE s.region_id = r.id),
  'providers',   (SELECT coalesce(json_agg(json_build_object('name', p.name, 'utility', p.utility, 'coverage', p.coverage) ORDER BY p.id), '[]') FROM providers p WHERE p.region_id = r.id)
) ORDER BY r.id), '[]') FROM regions r`

func newMux(pool *pgxpool.Pool, subs *subscriptions, clk clock.Clock) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/subscriptions", subs.subscribe)
	mux.HandleFunc("POST /api/verify", subs.verify)
	mux.HandleFunc("GET /api/regions", func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		if err := pool.QueryRow(r.Context(), regionsQuery).Scan(&body); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
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
	ID             int64    `json:"id"`
	Provider       string   `json:"provider"`
	Utility        string   `json:"utility"`
	Kind           string   `json:"kind"`
	Severity       string   `json:"severity"`
	Start          string   `json:"start"`
	End            string   `json:"end"`
	EndApproximate bool     `json:"end_approximate"`
	Places         []string `json:"places"`
	Notice         string   `json:"notice"` // the Provider's original text, never translated
}

type settlementJSON struct {
	Name    string       `json:"name"`
	Outages []outageJSON `json:"outages"`
}

type dayJSON struct {
	Date        string           `json:"date"`
	Settlements []settlementJSON `json:"settlements"`
}

// regionOutages lists Outages starting today through today+7 (Europe/Sofia), by day and Settlement.
func regionOutages(r *http.Request, pool *pgxpool.Pool, clk clock.Clock) ([]byte, error) {
	en := r.URL.Query().Get("lang") == "en"
	now := clk.Now().In(sofia)
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, sofia)
	rows, err := pool.Query(r.Context(), `
		SELECT o.id, p.name, p.utility, o.kind, o.severity, o.start_at, o.end_at, o.end_approximate, o.settlement, o.places, n.text
		FROM outages o JOIN notices n ON n.id = o.notice_id JOIN providers p ON p.id = o.provider_id
		WHERE p.region_id::text = $1 AND o.start_at >= $2 AND o.start_at < $3
		ORDER BY o.start_at, o.end_at, o.id`, r.PathValue("id"), from, from.AddDate(0, 0, 8))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byDay := map[string]map[string][]outageJSON{}
	for rows.Next() {
		var o outageJSON
		var start, end time.Time
		var settlement string
		if err := rows.Scan(&o.ID, &o.Provider, &o.Utility, &o.Kind, &o.Severity, &start, &end, &o.EndApproximate, &settlement, &o.Places, &o.Notice); err != nil {
			return nil, err
		}
		start, end = start.In(sofia), end.In(sofia)
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
