package app

import (
	"context"
	"encoding/json"
	"net/http"
)

// shadowRoutes are the Operator endpoints for Shadow Mode (ticket 19); they reuse the ticket 24 functions.
func (b *backoffice) shadowRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /backoffice/api/shadow-alerts", b.audited(roleOperator, func(ctx context.Context, r *http.Request, _ principal) (string, any, error) {
		alerts, err := shadowAlertRows(ctx, b.pool, r.URL.Query().Get("provider"))
		return "view_shadow_alerts", alerts, err
	}))
	mux.HandleFunc("POST /backoffice/api/providers/{name}/mode", b.guarded(roleOperator, func(w http.ResponseWriter, r *http.Request, p principal) error {
		var body struct{ Mode string }
		json.NewDecoder(r.Body).Decode(&body)
		tx, err := b.pool.Begin(r.Context())
		if err != nil {
			return err
		}
		defer tx.Rollback(r.Context())
		if err := applyProviderMode(r.Context(), tx, r.PathValue("name"), body.Mode, p.name); err != nil {
			return err
		}
		if err := tx.Commit(r.Context()); err != nil {
			return err
		}
		w.WriteHeader(http.StatusOK)
		return nil
	}))
}
