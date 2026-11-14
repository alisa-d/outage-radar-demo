package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

// clientIP is the TCP peer, unless the peer is a trusted proxy: then it is the right-most
// X-Forwarded-For entry that is not itself trusted (entries further left are client-controlled).
func (s *subscriptions) clientIP(r *http.Request) string {
	peer, _, _ := net.SplitHostPort(r.RemoteAddr)
	trusted := func(ip string) bool {
		p := net.ParseIP(ip)
		for _, n := range s.cfg.TrustedProxies {
			if p != nil && n.Contains(p) {
				return true
			}
		}
		return false
	}
	if !trusted(peer) {
		return peer
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		ip := strings.TrimSpace(hops[i])
		if net.ParseIP(ip) == nil {
			return peer // malformed: do not guess
		}
		if !trusted(ip) {
			return ip
		}
	}
	return peer
}

// turnstileOK verifies the token with Cloudflare (or the stub in tests); it fails closed.
func (s *subscriptions) turnstileOK(r *http.Request, token string) bool {
	if token == "" {
		return false
	}
	resp, err := http.PostForm(s.cfg.TurnstileURL, url.Values{"secret": {s.cfg.TurnstileSecret}, "response": {token}, "remoteip": {s.clientIP(r)}})
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

// enforce records a hit for every limit; it writes the 429 or 500 and returns false when one is exceeded.
func (s *subscriptions) enforce(w http.ResponseWriter, r *http.Request, checks []limit) bool {
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
	return true
}

// limitManageRequest applies the signup's per-IP and per-channel limits to management-link requests,
// so the endpoint cannot be used to flood a mailbox. The answer does not depend on whether the email is known.
func (s *subscriptions) limitManageRequest(w http.ResponseWriter, r *http.Request, email string) bool {
	return s.enforce(w, r, []limit{
		{"manage-ip:" + s.clientIP(r), s.cfg.IPLimit, hour},
		{"manage-channel:" + string(blindIndex(s.kp, email)), s.cfg.ChannelLimit, hour},
	})
}

// limitSignup runs the rate limits, the daily cap and the circuit breaker. It writes
// the response and returns false when the signup must not go on.
func (s *subscriptions) limitSignup(w http.ResponseWriter, r *http.Request, email string, addrs []string) bool {
	ch := string(blindIndex(s.kp, email))
	checks := []limit{
		{"signup-ip:" + s.clientIP(r), s.cfg.IPLimit, hour},
		{"channel:" + ch, s.cfg.ChannelLimit, hour},
		{"channel-day:" + ch, s.cfg.ChannelDailyCap, day},
	}
	for _, a := range addrs {
		checks = append(checks, limit{"address:" + a, s.cfg.AddressLimit, hour})
	}
	if !s.enforce(w, r, checks) {
		return false
	}
	// Circuit breaker: only new verifications pause; verified Subscribers are not touched here.
	ok, err := s.allow(r.Context(), "budget:email", s.cfg.EmailDailyBudget, day)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return false
	}
	if !ok {
		if !s.tripped.Swap(true) {
			s.trips.Add(r.Context(), 1, metric.WithAttributes(attribute.String("channel", "email")))
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
	ok, err := s.allow(r.Context(), "verify-ip:"+s.clientIP(r), s.cfg.IPLimit, hour)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
	} else if !ok {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
	}
	return ok
}

func purgeRateHits(ctx context.Context, pool *pgxpool.Pool, now time.Time) {
	pool.Exec(ctx, `DELETE FROM rate_hits WHERE at <= $1`, now.Add(-day))
}
