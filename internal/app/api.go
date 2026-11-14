package app

import (
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
	return mux
}
