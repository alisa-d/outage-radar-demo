package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// breakGlassTTL is how long a break-glass grant lasts.
const breakGlassTTL = 15 * time.Minute

// privacyRoutes are the Privacy officer's only paths to a Subscriber's personal data. Each needs a stated reason and is audited with it.
func (b *backoffice) privacyRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /backoffice/api/break-glass", b.privacy(func(w http.ResponseWriter, r *http.Request, p principal, sub int64, reason string) error {
		idb := make([]byte, 16)
		rand.Read(idb)
		id := hex.EncodeToString(idb)
		exp := b.clk.Now().Add(breakGlassTTL)
		if _, err := b.pool.Exec(r.Context(), `INSERT INTO break_glass_grants (id, officer, subscriber_id, reason, expires_at) VALUES ($1, $2, $3, $4, $5)`, id, p.name, sub, reason, exp); err != nil {
			return err
		}
		if err := b.record(r.Context(), p.name, "break_glass_open", reason); err != nil {
			return err
		}
		return reply(w, map[string]any{"id": id, "expires_at": exp.In(sofia).Format(time.RFC3339)})
	}))
	mux.HandleFunc("POST /backoffice/api/data-subject/export", b.privacy(func(w http.ResponseWriter, r *http.Request, p principal, sub int64, reason string) error {
		v, err := b.subs.load(r.Context(), sub)
		if err != nil {
			return err
		}
		if err := b.record(r.Context(), p.name, "data_subject_export", reason); err != nil {
			return err
		}
		return reply(w, v)
	}))
	mux.HandleFunc("POST /backoffice/api/data-subject/erase", b.privacy(func(w http.ResponseWriter, r *http.Request, p principal, sub int64, reason string) error {
		if err := b.subs.eraseSubscriber(r.Context(), sub); err != nil {
			return err
		}
		if err := b.record(r.Context(), p.name, "data_subject_erase", reason); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}))
	mux.HandleFunc("GET /backoffice/api/break-glass/{id}", b.guarded(rolePrivacy, func(w http.ResponseWriter, r *http.Request, p principal) error {
		var sub int64
		var reason string
		err := b.pool.QueryRow(r.Context(), `SELECT subscriber_id, reason FROM break_glass_grants WHERE id = $1 AND officer = $2 AND expires_at > $3`,
			r.PathValue("id"), p.name, b.clk.Now()).Scan(&sub, &reason)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpErr{http.StatusForbidden, "no active break-glass access"}
		}
		var v *subscriberView
		if err == nil {
			v, err = b.subs.load(r.Context(), sub)
		}
		if err == nil {
			err = b.record(r.Context(), p.name, "break_glass_view", reason)
		}
		if err != nil {
			return err
		}
		return reply(w, v)
	}))
}

// privacy runs fn for a Privacy officer on the Subscriber named by the body's email, once a reason is given.
func (b *backoffice) privacy(fn func(http.ResponseWriter, *http.Request, principal, int64, string) error) http.HandlerFunc {
	return b.guarded(rolePrivacy, func(w http.ResponseWriter, r *http.Request, p principal) error {
		var in struct{ Email, Reason string }
		email, valid := "", false
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&in) == nil {
			email, valid = parseEmail(in.Email)
		}
		reason := strings.TrimSpace(in.Reason)
		if !valid || reason == "" {
			return httpErr{http.StatusBadRequest, "email and reason are required"}
		}
		var sub int64
		err := b.pool.QueryRow(r.Context(), `SELECT subscriber_id FROM contact_channels WHERE blind_idx = $1`, blindIndex(b.subs.kp, email)).Scan(&sub)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpErr{http.StatusNotFound, "not found"}
		}
		if err != nil {
			return err
		}
		return fn(w, r, p, sub, reason)
	})
}

func (b *backoffice) record(ctx context.Context, actor, action, reason string) error {
	_, err := b.pool.Exec(ctx, `INSERT INTO audit_log (at, actor, action, reason) VALUES ($1, $2, $3, $4)`, b.clk.Now(), actor, action, reason)
	return err
}

func reply(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(v)
}
