package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
)

// shadowRoutes are the Operator endpoints for Shadow Mode (ticket 19); they reuse the ticket 24 functions.
func (b *backoffice) shadowRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /backoffice/api/shadow-alerts", b.operator(func(w http.ResponseWriter, r *http.Request, p principal) error {
		alerts, err := shadowAlertRows(r.Context(), b.pool, r.URL.Query().Get("provider"))
		if err == nil {
			err = b.audit1(r, p, "view_shadow_alerts")
		}
		if err != nil {
			return err
		}
		w.Header().Set("Content-Type", "application/json")
		return json.NewEncoder(w).Encode(alerts)
	}))
	mux.HandleFunc("POST /backoffice/api/providers/{name}/mode", b.operator(func(w http.ResponseWriter, r *http.Request, p principal) error {
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

// operator authenticates an Operator with 2FA and maps the shared errors to statuses.
func (b *backoffice) operator(fn func(http.ResponseWriter, *http.Request, principal) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := b.authenticate(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !p.mfa || !slices.Contains(p.roles, roleOperator) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		switch err := fn(w, r, p); {
		case err == nil:
		case errors.Is(err, errBadMode):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, errUnknownProvider):
			http.Error(w, err.Error(), http.StatusNotFound)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
	}
}

func (b *backoffice) audit1(r *http.Request, p principal, action string) error {
	_, err := b.pool.Exec(r.Context(), `INSERT INTO audit_log (at, actor, action) VALUES ($1, $2, $3)`, b.clk.Now(), p.name, action)
	return err
}
