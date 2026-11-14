package app_test

import (
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/alisa/outage-radar/internal/app"
	"github.com/alisa/outage-radar/internal/testkit"
)

// verifyFrom posts a verify request carrying the given X-Forwarded-For and returns the status.
func verifyFrom(t *testing.T, s *testkit.System, xff string) int {
	t.Helper()
	req, _ := http.NewRequest("POST", s.URL+"/api/verify", strings.NewReader(`{"token":"x.y","consent":true}`))
	req.Header.Set("X-Forwarded-For", xff)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestForwardedForIsIgnoredUnlessThePeerIsATrustedProxy(t *testing.T) {
	s, _ := startLimited(t, func(c *app.Config) { c.IPLimit = 1 })
	verifyFrom(t, s, "1.1.1.1")
	if code := verifyFrom(t, s, "2.2.2.2"); code != 429 {
		t.Fatalf("spoofed header must not give a fresh limit: %d", code)
	}
}

func TestTrustedProxyYieldsTheRightMostUntrustedForwardedAddress(t *testing.T) {
	_, loopback, _ := net.ParseCIDR("127.0.0.0/8")
	s, _ := startLimited(t, func(c *app.Config) {
		c.IPLimit = 1
		c.TrustedProxies = []*net.IPNet{loopback}
	})
	if code := verifyFrom(t, s, "1.1.1.1"); code == 429 {
		t.Fatalf("first: %d", code)
	}
	if code := verifyFrom(t, s, "2.2.2.2"); code == 429 {
		t.Fatalf("a different client behind the proxy shares a limit: %d", code)
	}
	// The left-most entry is client-controlled; only the right-most untrusted one counts.
	if code := verifyFrom(t, s, "9.9.9.9, 2.2.2.2, 127.0.0.2"); code != 429 {
		t.Fatalf("same client with a spoofed prefix: %d, want 429", code)
	}
}
