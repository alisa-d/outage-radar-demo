package app_test

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alisa/outage-radar/internal/app"
	"github.com/alisa/outage-radar/internal/testkit"
)

func TestOperatorListsShadowAlertsWithoutPersonalDataAndSwitchesProviderLiveAudited(t *testing.T) {
	s, down := watcher(t, "en")
	ctx := context.Background()
	if err := app.SetProviderMode(ctx, s.DSN, erp, "shadow", "cli"); err != nil {
		t.Fatal(err)
	}
	publish(s, down, hrow(1, intro+"ул. Дойран.", "9:00", "17:00"))
	waitFor(t, func() bool { return scanned(t, s) })

	op := s.OIDC.Token(testkit.Claims{Roles: []string{"operator"}})
	po := s.OIDC.Token(testkit.Claims{Roles: []string{"privacy_officer"}})
	list := s.URL + "/backoffice/api/shadow-alerts?provider=" + url.QueryEscape(erp)
	if c, _ := do(t, "GET", list, po, nil); c != 403 {
		t.Errorf("privacy officer listing: %d", c)
	}
	c, body := do(t, "GET", list, op, nil)
	if c != 200 {
		t.Fatalf("list %d %s", c, body)
	}
	var rows []struct {
		Provider, Settlement string
		MatchResult          string `json:"match_result"`
		Count                int
	}
	if err := json.Unmarshal([]byte(body), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("%v: %s", err, body)
	}
	if r := rows[0]; r.Provider != erp || r.Settlement != "Варна" || r.MatchResult != "affected" || r.Count != 1 {
		t.Errorf("row %+v", r)
	}
	for _, pii := range []string{"ivan", "Дойран", "Doyran"} {
		if strings.Contains(body, pii) {
			t.Errorf("personal data %q in %s", pii, body)
		}
	}

	mode := s.URL + "/backoffice/api/providers/" + url.PathEscape(erp) + "/mode"
	if c, _ := do(t, "POST", mode, po, map[string]string{"mode": "live"}); c != 403 {
		t.Errorf("privacy officer switch: %d", c)
	}
	if c, _ := do(t, "POST", mode, op, map[string]string{"mode": "bogus"}); c != 400 {
		t.Errorf("bogus mode: %d", c)
	}
	if c, _ := do(t, "POST", s.URL+"/backoffice/api/providers/nope/mode", op, map[string]string{"mode": "live"}); c != 404 {
		t.Errorf("unknown provider: %d", c)
	}
	if c, b := do(t, "POST", mode, op, map[string]string{"mode": "live"}); c != 200 {
		t.Fatalf("switch %d %s", c, b)
	}
	s.Clock.Advance(16 * time.Minute)
	waitMail(t, s, 1) // no backlog
	publish(s, down, hrow(1, intro+"ул. Дойран.", "9:00", "17:00"), hrow(2, intro+"ул. Дойран.", "10:00", "18:00"))
	waitMail(t, s, 2)

	_, audit := do(t, "GET", s.URL+"/backoffice/api/audit", po, nil)
	if !strings.Contains(audit, `"actor":"oscar"`) || !strings.Contains(audit, "provider_mode "+erp+" live") {
		t.Errorf("audit lacks the switch: %s", audit)
	}
}
