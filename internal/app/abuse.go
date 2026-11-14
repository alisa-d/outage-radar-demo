package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	hour = time.Hour
	day  = 24 * time.Hour
)

// allow records a hit on key and reports whether fewer than limit hits happened in the
// last window. Keys are HMACs, so the table holds no personal data.
// shortcut: check-then-insert is not atomic, so a burst can overshoot a limit slightly; use an advisory lock if that matters.
func (s *subscriptions) allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	m := hmac.New(sha256.New, s.kp.Derive("rate-limit"))
	m.Write([]byte(key))
	k, now := m.Sum(nil), s.clk.Now()
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM rate_hits WHERE key = $1 AND at > $2`, k, now.Add(-window)).Scan(&n); err != nil || n >= limit {
		return false, err
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO rate_hits (key, at) VALUES ($1, $2)`, k, now)
	return err == nil, err
}

// clientIP is the TCP peer.
// shortcut: behind a reverse proxy every request shares one IP; honour a trusted X-Forwarded-For before deploying behind one.
func clientIP(r *http.Request) string {
	h, _, _ := net.SplitHostPort(r.RemoteAddr)
	return h
}

// turnstileOK verifies the token with Cloudflare (or the stub in tests); it fails closed.
func (s *subscriptions) turnstileOK(r *http.Request, token string) bool {
	if token == "" {
		return false
	}
	resp, err := http.PostForm(s.cfg.TurnstileURL, url.Values{"secret": {s.cfg.TurnstileSecret}, "response": {token}, "remoteip": {clientIP(r)}})
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var out struct{ Success bool }
	return json.NewDecoder(resp.Body).Decode(&out) == nil && out.Success
}

type limit struct {
	key    string
	max    int
	window time.Duration
}

// limitSignup runs the rate limits, the daily cap and the circuit breaker. It writes
// the response and returns false when the signup must not go on.
func (s *subscriptions) limitSignup(w http.ResponseWriter, r *http.Request, email string, addrs []string) bool {
	ch := string(blindIndex(s.kp, email))
	checks := []limit{
		{"signup-ip:" + clientIP(r), s.cfg.IPLimit, hour},
		{"channel:" + ch, s.cfg.ChannelLimit, hour},
		{"channel-day:" + ch, s.cfg.ChannelDailyCap, day},
	}
	for _, a := range addrs {
		checks = append(checks, limit{"address:" + a, s.cfg.AddressLimit, hour})
	}
	for _, c := range checks {
		if ok, err := s.allow(r.Context(), c.key, c.max, c.window); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return false
		} else if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(c.window.Seconds())))
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return false
		}
	}
	// Circuit breaker: only new verifications pause; verified Subscribers are not touched here.
	ok, err := s.allow(r.Context(), "budget:email", s.cfg.EmailDailyBudget, day)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return false
	}
	if !ok {
		if !s.tripped.Swap(true) {
			s.trips.Add(1)
		}
		w.Header().Set("Retry-After", strconv.Itoa(int(hour.Seconds())))
		http.Error(w, "signups are temporarily paused", http.StatusServiceUnavailable)
		return false
	}
	s.tripped.Store(false)
	return true
}

// limitVerify applies the per-IP limit to verification requests.
func (s *subscriptions) limitVerify(w http.ResponseWriter, r *http.Request) bool {
	ok, err := s.allow(r.Context(), "verify-ip:"+clientIP(r), s.cfg.IPLimit, hour)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
	} else if !ok {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
	}
	return ok
}

func (s *subscriptions) metrics(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "# TYPE circuit_breaker_trips_total counter\ncircuit_breaker_trips_total{channel=\"email\"} %d\n", s.trips.Load())
}

func purgeRateHits(ctx context.Context, pool *pgxpool.Pool, now time.Time) {
	pool.Exec(ctx, `DELETE FROM rate_hits WHERE at <= $1`, now.Add(-day))
}
