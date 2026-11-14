package app

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
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

func (s *subscriptions) deliveryMetrics(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `SELECT channel, event, count(*) FROM delivery_events GROUP BY 1, 2 ORDER BY 1, 2`)
	if err != nil {
		return
	}
	defer rows.Close()
	fmt.Fprint(w, "# TYPE delivery_failures_total counter\n")
	for rows.Next() {
		var ch, ev string
		var n int
		if rows.Scan(&ch, &ev, &n) == nil {
			fmt.Fprintf(w, "delivery_failures_total{channel=%q,event=%q} %d\n", ch, ev, n)
		}
	}
}
