package app_test

import (
	"testing"

	"github.com/alisa/outage-radar/internal/app"
)

func TestManagementLinkRequestsAreRateLimitedPerIPAndPerChannel(t *testing.T) {
	for _, c := range []struct {
		name   string
		tweak  func(*app.Config)
		second string
	}{
		{"per IP", func(c *app.Config) { c.IPLimit = 1 }, "other@example.com"},
		{"per channel", func(c *app.Config) { c.ChannelLimit = 1 }, "ivan@example.com"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, _ := startLimited(t, c.tweak)
			if code, _ := requestLink(t, s, "ivan@example.com"); code != 202 {
				t.Fatalf("first: %d", code)
			}
			if code, _ := requestLink(t, s, c.second); code != 429 {
				t.Fatalf("second: %d, want 429", code)
			}
		})
	}
}
