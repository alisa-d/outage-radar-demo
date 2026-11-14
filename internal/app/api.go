package app

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

const regionsQuery = `
SELECT coalesce(json_agg(json_build_object(
  'id', r.id, 'name', r.name,
  'settlements', (SELECT coalesce(json_agg(json_build_object('name', s.name, 'ekatte', s.ekatte) ORDER BY s.id), '[]') FROM settlements s WHERE s.region_id = r.id),
  'providers',   (SELECT coalesce(json_agg(json_build_object('name', p.name, 'utility', p.utility, 'coverage', p.coverage) ORDER BY p.id), '[]') FROM providers p WHERE p.region_id = r.id)
) ORDER BY r.id), '[]') FROM regions r`

func newMux(pool *pgxpool.Pool) http.Handler {
	mux := http.NewServeMux()
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
	return mux
}
