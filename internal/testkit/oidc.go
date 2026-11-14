package testkit

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const OIDCClientID = "backoffice"

// OIDC is a stub OIDC provider at HTTP level: discovery, JWKS and a token signer.
type OIDC struct {
	URL string
	key *rsa.PrivateKey
	now func() time.Time
}

func newOIDC(t testing.TB, now func() time.Time) *OIDC {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	o := &OIDC{key: key, now: now}
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	o.URL = srv.URL
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"issuer": o.URL, "jwks_uri": o.URL + "/jwks"})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": "k1", "alg": "RS256", "use": "sig",
			"n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes())}}})
	})
	return o
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// Claims overrides what Token puts in a token; zero values mean a valid, 2FA-backed one.
type Claims struct {
	Roles    []string
	NoMFA    bool
	Audience string
	Expires  time.Duration
	Signer   *rsa.PrivateKey // sign with a key the JWKS does not publish
}

// Token returns a signed ID token for operator "oscar" holding the given realm roles.
func (o *OIDC) Token(c Claims) string {
	if c.Audience == "" {
		c.Audience = OIDCClientID
	}
	if c.Expires == 0 {
		c.Expires = time.Hour
	}
	if c.Signer == nil {
		c.Signer = o.key
	}
	amr := []string{"pwd", "otp"}
	if c.NoMFA {
		amr = []string{"pwd"}
	}
	hdr, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "k1", "typ": "JWT"})
	body, _ := json.Marshal(map[string]any{"iss": o.URL, "aud": c.Audience, "sub": "oscar", "preferred_username": "oscar",
		"exp": o.now().Add(c.Expires).Unix(), "amr": amr, "realm_access": map[string]any{"roles": c.Roles}})
	in := b64(hdr) + "." + b64(body)
	h := sha256.Sum256([]byte(in))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.Signer, crypto.SHA256, h[:])
	if err != nil {
		panic(err)
	}
	return in + "." + b64(sig)
}
