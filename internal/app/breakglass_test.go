package app_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alisa/outage-radar/internal/testkit"
)

const breakGlassPath = "/backoffice/api/break-glass"

func officer(s *testkit.System) string {
	return s.OIDC.Token(testkit.Claims{Roles: []string{"privacy_officer"}})
}

func operator(s *testkit.System) string {
	return s.OIDC.Token(testkit.Claims{Roles: []string{"operator"}})
}

type auditEntry struct{ Actor, Action, Reason string }

func auditEntries(t *testing.T, s *testkit.System) []auditEntry {
	t.Helper()
	_, body := do(t, "GET", s.URL+auditPath, officer(s), nil)
	var out []auditEntry
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	return out
}

func TestBreakGlassNeedsAReasonExpiresAndIsAudited(t *testing.T) {
	s, _ := subscribed(t)
	const reason = "ticket 4411: subscriber cannot reach management page"
	open := map[string]any{"email": "ivan@example.com", "reason": reason}

	if c, _ := do(t, "POST", s.URL+breakGlassPath, operator(s), open); c != 403 {
		t.Errorf("operator: %d", c)
	}
	if c, _ := do(t, "POST", s.URL+breakGlassPath, officer(s), map[string]any{"email": "ivan@example.com", "reason": "  "}); c != 400 {
		t.Errorf("no reason: %d", c)
	}
	if c, _ := do(t, "POST", s.URL+breakGlassPath, officer(s), map[string]any{"email": "nobody@example.com", "reason": "x"}); c != 404 {
		t.Errorf("unknown subscriber: %d", c)
	}

	c, body := do(t, "POST", s.URL+breakGlassPath, officer(s), open)
	var grant struct {
		ID      string `json:"id"`
		Expires string `json:"expires_at"`
	}
	if c != 200 || json.Unmarshal([]byte(body), &grant) != nil || grant.ID == "" {
		t.Fatalf("open: %d %s", c, body)
	}
	if strings.Contains(body, "ivan") {
		t.Errorf("opening returns personal data: %s", body)
	}
	if c, _ := do(t, "GET", s.URL+breakGlassPath+"/"+grant.ID, operator(s), nil); c != 403 {
		t.Errorf("operator view: %d", c)
	}
	c, body = do(t, "GET", s.URL+breakGlassPath+"/"+grant.ID, officer(s), nil)
	if c != 200 || !strings.Contains(body, "ivan@example.com") {
		t.Fatalf("view: %d %s", c, body)
	}
	s.Clock.Advance(16 * time.Minute)
	if c, _ := do(t, "GET", s.URL+breakGlassPath+"/"+grant.ID, officer(s), nil); c != 403 {
		t.Errorf("after the time limit: %d", c)
	}

	var got []string
	for _, e := range auditEntries(t, s) {
		if strings.HasPrefix(e.Action, "break_glass") {
			got = append(got, e.Action+"|"+e.Actor+"|"+e.Reason)
		}
	}
	if len(got) != 2 || got[0] != "break_glass_open|oscar|"+reason || got[1] != "break_glass_view|oscar|"+reason {
		t.Errorf("audit = %q", got)
	}
}

func TestPrivacyOfficerExportAndErasureMatchSelfServiceAndAreAudited(t *testing.T) {
	s, _ := subscribed(t)
	const reason = "email request from the subscriber, ref 77"
	body := map[string]any{"email": "ivan@example.com", "reason": reason}
	sess := manageSession(t, s, "ivan@example.com", 2)

	_, self := do(t, "GET", s.URL+"/api/manage/export", sess, nil)
	for _, path := range []string{"/export", "/erase"} {
		if c, _ := do(t, "POST", s.URL+"/backoffice/api/data-subject"+path, operator(s), body); c != 403 {
			t.Errorf("operator %s: %d", path, c)
		}
		if c, _ := do(t, "POST", s.URL+"/backoffice/api/data-subject"+path, officer(s), map[string]any{"email": "ivan@example.com"}); c != 400 {
			t.Errorf("no reason %s: %d", path, c)
		}
	}
	c, behalf := do(t, "POST", s.URL+"/backoffice/api/data-subject/export", officer(s), body)
	var a, b any
	if c != 200 || json.Unmarshal([]byte(self), &a) != nil || json.Unmarshal([]byte(behalf), &b) != nil || !reflect.DeepEqual(a, b) {
		t.Errorf("export on behalf %d differs from self-service:\n%s\n%s", c, behalf, self)
	}

	if c, _ := do(t, "POST", s.URL+"/backoffice/api/data-subject/erase", officer(s), body); c != 204 {
		t.Fatalf("erase: %d", c)
	}
	assertNothingLeft(t, s)
	if c, _ := do(t, "GET", s.URL+"/api/manage", sess, nil); c != 401 {
		t.Errorf("session after erasure: %d", c)
	}

	var got []string
	for _, e := range auditEntries(t, s) {
		if strings.HasPrefix(e.Action, "data_subject") {
			got = append(got, e.Action+"|"+e.Actor+"|"+e.Reason)
		}
	}
	if len(got) != 2 || got[0] != "data_subject_export|oscar|"+reason || got[1] != "data_subject_erase|oscar|"+reason {
		t.Errorf("audit = %q", got)
	}
}
