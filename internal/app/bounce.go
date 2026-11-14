package app

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"io"
	"net/http"
	"strings"
)

// brevoEvents maps Brevo's transactional webhook events to ours; others are ignored.
var brevoEvents = map[string]string{
	"hard_bounce": "hard_bounce", "invalid_email": "hard_bounce", "blocked": "hard_bounce",
	"soft_bounce": "soft_bounce", "spam": "complaint",
}

// brevoWebhook records a delivery failure and disables the Contact Channel on a hard bounce or complaint.
func (s *subscriptions) brevoWebhook(w http.ResponseWriter, r *http.Request) {
	want := "Bearer " + s.cfg.BrevoWebhookToken
	if s.cfg.BrevoWebhookToken == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(want)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var ev struct{ Event, Email string }
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if json.Unmarshal(body, &ev) != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	kind, ok := brevoEvents[ev.Event]
	if !ok {
		return // delivered, opened and the like: nothing to do
	}
	if _, err := s.pool.Exec(r.Context(), `INSERT INTO delivery_events (at, channel, event) VALUES ($1, 'email', $2)`, s.clk.Now(), kind); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if kind != "soft_bounce" {
		idx := blindIndex(s.kp, strings.ToLower(strings.TrimSpace(ev.Email)))
		if _, err := s.pool.Exec(r.Context(), `UPDATE contact_channels SET disabled_at = $2 WHERE blind_idx = $1 AND disabled_at IS NULL`, idx, s.clk.Now()); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}
}

// registerDeliveryMetrics exports delivery_events as delivery_failures_total; the table is the counter.
func (s *subscriptions) registerDeliveryMetrics(m metric.Meter) error {
	_, err := m.Int64ObservableCounter("delivery_failures", metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
		rows, err := s.pool.Query(ctx, `SELECT channel, event, count(*) FROM delivery_events GROUP BY 1, 2`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ch, ev string
			var n int64
			if err := rows.Scan(&ch, &ev, &n); err != nil {
				return err
			}
			o.Observe(n, metric.WithAttributes(attribute.String("channel", ch), attribute.String("event", ev)))
		}
		return rows.Err()
	}))
	return err
}
