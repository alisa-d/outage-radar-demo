package app

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/alisa/outage-radar/internal/clock"
	"github.com/alisa/outage-radar/internal/ingest"
	"github.com/jackc/pgx/v5/pgxpool"
)

// backoffice is the Operator and Privacy officer JSON API, separate from the public one (under /backoffice/api,
// which the frontend proxy does not forward). Callers present an OIDC ID token as a Bearer token.
type backoffice struct {
	pool *pgxpool.Pool
	subs *subscriptions
	clk  clock.Clock
	cfg  Config

	mu   sync.Mutex
	keys map[string]*rsa.PublicKey // by kid
}

const (
	roleOperator = "operator"
	rolePrivacy  = "privacy_officer"
)

func (b *backoffice) routes(mux *http.ServeMux) {
	if b.cfg.OIDCIssuer == "" {
		return
	}
	mux.HandleFunc("GET /backoffice/api/providers/health", b.handle(roleOperator, "view_provider_health", b.providerHealth))
	mux.HandleFunc("GET /backoffice/api/stats", b.handle(roleOperator, "view_stats", b.stats))
	mux.HandleFunc("GET /backoffice/api/audit", b.handle(rolePrivacy, "view_audit_log", b.audit))
	b.reviewRoutes(mux)
	b.shadowRoutes(mux)
}

// handle authenticates, checks 2FA and role, runs fn and records the action before answering.
// Every response is built from aggregates or operational data: nothing here returns decrypted personal data.
func (b *backoffice) handle(role, action string, fn func(context.Context) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := b.authenticate(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !p.mfa || !slices.Contains(p.roles, role) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		out, err := fn(r.Context())
		if err == nil {
			_, err = b.pool.Exec(r.Context(), `INSERT INTO audit_log (at, actor, action) VALUES ($1, $2, $3)`, b.clk.Now(), p.name, action)
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	}
}

type principal struct {
	name  string
	mfa   bool // the token says a second factor was used (amr has otp or mfa)
	roles []string
}

// authenticate validates the ID token: RS256 signature against the issuer's JWKS, iss, aud and exp.
func (b *backoffice) authenticate(r *http.Request) (principal, error) {
	var zero principal
	raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	parts := strings.Split(raw, ".")
	if !ok || len(parts) != 3 {
		return zero, errors.New("no token")
	}
	dec := func(s string, v any) error {
		j, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			return err
		}
		return json.Unmarshal(j, v)
	}
	var hdr struct{ Alg, Kid string }
	var c struct {
		Iss      string
		Sub      string
		Aud      any
		Exp      float64
		Amr      []string
		Realm    struct{ Roles []string } `json:"realm_access"`
		Username string                   `json:"preferred_username"`
	}
	if dec(parts[0], &hdr) != nil || dec(parts[1], &c) != nil || hdr.Alg != "RS256" {
		return zero, errors.New("malformed token")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return zero, err
	}
	key, err := b.key(r.Context(), hdr.Kid)
	if err != nil {
		return zero, err
	}
	h := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, h[:], sig) != nil {
		return zero, errors.New("bad signature")
	}
	aud := []any{c.Aud}
	if l, ok := c.Aud.([]any); ok {
		aud = l
	}
	if c.Iss != b.cfg.OIDCIssuer || !slices.Contains(aud, any(b.cfg.OIDCClientID)) || !b.clk.Now().Before(time.Unix(int64(c.Exp), 0)) {
		return zero, errors.New("invalid claims")
	}
	name := c.Username
	if name == "" {
		name = c.Sub
	}
	return principal{name, slices.Contains(c.Amr, "otp") || slices.Contains(c.Amr, "mfa"), c.Realm.Roles}, nil
}

// key returns the issuer's signing key, fetching the JWKS when the kid is unknown.
func (b *backoffice) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if k := b.keys[kid]; k != nil {
		return k, nil
	}
	// shortcut: the JWKS is refetched on every unknown kid, so a flood of garbage kids costs fetches; rate-limit if that matters.
	var disc struct {
		JWKS string `json:"jwks_uri"`
	}
	var set struct {
		Keys []struct{ Kid, Kty, N, E string }
	}
	get := func(u string, out any) error {
		req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return json.NewDecoder(resp.Body).Decode(out)
	}
	if err := get(b.cfg.OIDCIssuer+"/.well-known/openid-configuration", &disc); err != nil {
		return nil, err
	}
	if err := get(disc.JWKS, &set); err != nil {
		return nil, err
	}
	b.keys = map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if k.Kty == "RSA" && err1 == nil && err2 == nil {
			b.keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
		}
	}
	if b.keys[kid] == nil {
		return nil, fmt.Errorf("no key %q", kid)
	}
	return b.keys[kid], nil
}

func (b *backoffice) providerHealth(ctx context.Context) (any, error) {
	hs, err := ingest.Healths(ctx, b.pool, b.clk.Now())
	if err != nil {
		return nil, err
	}
	rows, err := b.pool.Query(ctx, `SELECT name, utility FROM providers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var name, utility string
		if err := rows.Scan(&name, &utility); err != nil {
			return nil, err
		}
		h := hs[name]
		errs := []string{}
		if h.LastError != "" {
			errs = append(errs, h.LastError) // shortcut: only the latest error is kept, not a history
		}
		var last any
		if h.LastSuccess != nil {
			last = h.LastSuccess.In(sofia).Format(time.RFC3339)
		}
		out = append(out, map[string]any{"name": name, "utility": utility, "state": h.State, "last_success": last, "recent_errors": errs})
	}
	return out, rows.Err()
}

func (b *backoffice) stats(ctx context.Context) (any, error) {
	// Subscribers per Settlement: addresses are decrypted in memory only to be counted, never returned.
	rows, err := b.pool.Query(ctx, `SELECT w.subscriber_id, w.data_enc FROM watched_addresses w
		WHERE EXISTS (SELECT 1 FROM contact_channels c WHERE c.subscriber_id = w.subscriber_id AND c.verified_at IS NOT NULL)`)
	if err != nil {
		return nil, err
	}
	streetsBySub := map[int64][]int{}
	for rows.Next() {
		var sub int64
		var enc []byte
		var wa watchedAddress
		if err := rows.Scan(&sub, &enc); err != nil {
			rows.Close()
			return nil, err
		}
		plain, err := unseal(b.subs.kp, enc)
		if err == nil {
			err = json.Unmarshal(plain, &wa)
		}
		if err != nil {
			rows.Close()
			return nil, err
		}
		streetsBySub[sub] = append(streetsBySub[sub], wa.StreetID)
	}
	rows.Close()
	names := map[int]string{}
	srows, err := b.pool.Query(ctx, `SELECT st.id, s.name FROM streets st JOIN settlements s ON s.id = st.settlement_id`)
	if err != nil {
		return nil, err
	}
	for srows.Next() {
		var id int
		var n string
		if err := srows.Scan(&id, &n); err != nil {
			srows.Close()
			return nil, err
		}
		names[id] = n
	}
	srows.Close()
	perSettlement := map[string]int{}
	for _, streets := range streetsBySub {
		in := map[string]bool{}
		for _, st := range streets {
			in[names[st]] = true
		}
		for n := range in {
			perSettlement[n]++
		}
	}
	subs := []map[string]any{}
	for n, c := range perSettlement {
		subs = append(subs, map[string]any{"settlement": n, "count": c})
	}
	slices.SortFunc(subs, func(a, c map[string]any) int {
		return strings.Compare(a["settlement"].(string), c["settlement"].(string))
	})

	arows, err := b.pool.Query(ctx, `SELECT (created_at AT TIME ZONE 'Europe/Sofia')::date::text, count(*), count(*) FILTER (WHERE delivery = 'delivered')
		FROM alerts WHERE NOT shadow GROUP BY 1 ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer arows.Close()
	days := []map[string]any{}
	var sent, delivered int
	for arows.Next() {
		var d string
		var s, dl int
		if err := arows.Scan(&d, &s, &dl); err != nil {
			return nil, err
		}
		days = append(days, map[string]any{"date": d, "sent": s, "delivered": dl})
		sent, delivered = sent+s, delivered+dl
	}
	success := 0.0
	if sent > 0 {
		success = float64(delivered) / float64(sent)
	}
	return map[string]any{"subscribers_per_settlement": subs, "alerts_per_day": days, "delivery_success": success}, arows.Err()
}

func (b *backoffice) audit(ctx context.Context) (any, error) {
	rows, err := b.pool.Query(ctx, `SELECT at, actor, action FROM audit_log ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var at time.Time
		var actor, action string
		if err := rows.Scan(&at, &actor, &action); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"at": at.In(sofia).Format(time.RFC3339), "actor": actor, "action": action})
	}
	return out, rows.Err()
}
