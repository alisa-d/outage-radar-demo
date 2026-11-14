package app_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alisa/outage-radar/internal/testkit"
)

const queuePath = "/backoffice/api/review-queue"

type queueItem struct {
	NoticeID   int64    `json:"notice_id"`
	Ordinal    int      `json:"ordinal"`
	Notice     string   `json:"notice"`
	Settlement string   `json:"settlement"`
	Severity   string   `json:"severity"`
	Places     []string `json:"places"`
	Unlocated  bool     `json:"unlocated"`
	Unresolved []string `json:"unresolved_places"`
}

func opToken(s *testkit.System) string {
	return s.OIDC.Token(testkit.Claims{Roles: []string{"operator"}})
}

func queue(t *testing.T, s *testkit.System) []queueItem {
	t.Helper()
	code, body := do(t, "GET", s.URL+queuePath, opToken(s), nil)
	var q []queueItem
	if code != 200 || json.Unmarshal([]byte(body), &q) != nil {
		t.Fatalf("queue %d %s", code, body)
	}
	return q
}

func patchOutage(t *testing.T, s *testkit.System, it queueItem, body any) int {
	t.Helper()
	code, resp := do(t, "PATCH", fmt.Sprintf("%s/backoffice/api/outages/%d/%d", s.URL, it.NoticeID, it.Ordinal), opToken(s), body)
	if code >= 500 {
		t.Fatalf("patch %d %s", code, resp)
	}
	return code
}

const unlocatedRow = intro + "клиентите, електрозахранени от ТП 314."

func TestUnlocatedOutageIsQueuedAndPlacingItOnAStreetReleasesTheHeldAlert(t *testing.T) {
	s, down := watcher(t, "en")
	publish(s, down, hrow(1, unlocatedRow, "9:00", "17:00"))
	waitFor(t, func() bool { return len(queue(t, s)) == 1 })
	q := queue(t, s)
	if !q[0].Unlocated || !strings.Contains(q[0].Notice, "ТП 314") || q[0].Settlement != "Варна" || len(q[0].Unresolved) != 1 {
		t.Fatalf("item: %+v", q[0])
	}
	waitMail(t, s, 1) // held back: no Alert yet

	if c := patchOutage(t, s, q[0], map[string]any{"streets": []string{"ул. Дойран"}}); c != 200 {
		t.Fatalf("patch status %d", c)
	}
	if m := waitMail(t, s, 2)[1]; m.To != "ivan@example.com" || !strings.Contains(m.Body, "Doyran 11") {
		t.Errorf("released alert: %+v", m)
	}
	if left := queue(t, s); len(left) != 0 {
		t.Errorf("placed Outage still queued: %+v", left)
	}
}

func TestDrawingAnAreaAroundTheAddressPlacesTheOutage(t *testing.T) {
	s, down := watcher(t, "en")
	publish(s, down, hrow(1, unlocatedRow, "9:00", "17:00"))
	waitFor(t, func() bool { return len(queue(t, s)) == 1 })
	ring := [][2]float64{{27.899, 43.229}, {27.901, 43.229}, {27.901, 43.231}, {27.899, 43.231}}
	if c := patchOutage(t, s, queue(t, s)[0], map[string]any{"ring": ring}); c != 200 {
		t.Fatalf("patch status %d", c)
	}
	waitMail(t, s, 2)
	if left := queue(t, s); len(left) != 0 {
		t.Errorf("still queued: %+v", left)
	}
}

func TestCorrectingTheTimeWindowAlertsOnlyWhenMaterial(t *testing.T) {
	s, down := watcher(t, "en")
	publish(s, down, hrow(1, intro+"ул. Дойран.", "9:00", "17:00"))
	waitMail(t, s, 2)
	it := queueItem{NoticeID: 1}
	if c := patchOutage(t, s, it, map[string]any{"end": "2026-11-14T17:10:00+02:00"}); c != 200 {
		t.Fatalf("patch status %d", c)
	}
	waitMail(t, s, 2) // ten minutes: not material
	patchOutage(t, s, it, map[string]any{"end": "2026-11-14T17:45:00+02:00", "severity": "disturbance"})
	if m := waitMail(t, s, 3)[2]; !strings.Contains(m.Body, "17:45") {
		t.Errorf("alert: %+v", m)
	}
}

func TestNewGazetteerEntryMakesLaterNoticesResolveAutomatically(t *testing.T) {
	s, down := watcher(t, "en")
	const place = intro + "парк Приморски."
	publish(s, down, hrow(1, place, "9:00", "17:00"))
	waitFor(t, func() bool { return len(queue(t, s)) == 1 && scanned(t, s) }) // the held Notice is scanned before the entry exists
	if q := queue(t, s); len(q[0].Unresolved) != 1 || q[0].Unresolved[0] != "парк Приморски" {
		t.Fatalf("queue: %+v", q)
	}
	entry := map[string]any{"settlement": "Варна", "name": "парк Приморски", "kind": "point", "geom": [][2]float64{{27.9, 43.2301}}}
	if c, b := do(t, "POST", s.URL+"/backoffice/api/gazetteer", opToken(s), entry); c != 200 {
		t.Fatalf("post %d %s", c, b)
	}
	if _, b := do(t, "GET", s.URL+"/backoffice/api/gazetteer?settlement=Варна", opToken(s), nil); !strings.Contains(b, "парк Приморски") {
		t.Errorf("list: %s", b)
	}
	serveFeed(s, down, feed(hrow(1, place, "9:00", "17:00"), hrow(2, place, "10:00", "18:00")))
	s.Clock.Advance(16 * time.Minute)
	if m := waitMail(t, s, 2)[1]; !strings.Contains(m.Body, "possibly affecting") || !strings.Contains(m.Body, "10:00") {
		t.Errorf("alert for the later Notice: %+v", m)
	}
}

func TestCorrectionsAreAuditedAndOnlyOperatorsMakeThem(t *testing.T) {
	s, down := watcher(t, "en")
	publish(s, down, hrow(1, unlocatedRow, "9:00", "17:00"))
	waitFor(t, func() bool { return len(queue(t, s)) == 1 })
	it := queue(t, s)[0]
	po := s.OIDC.Token(testkit.Claims{Roles: []string{"privacy_officer"}})
	for tok, want := range map[string]int{"": 401, po: 403} {
		if c, _ := do(t, "PATCH", s.URL+"/backoffice/api/outages/1/0", tok, map[string]any{"severity": "cut"}); c != want {
			t.Errorf("status %d, want %d", c, want)
		}
	}
	if c := patchOutage(t, s, it, map[string]any{"streets": []string{"Няма такава"}}); c != 422 {
		t.Errorf("unknown street: %d", c)
	}
	patchOutage(t, s, it, map[string]any{"streets": []string{"Дойран"}})
	_, body := do(t, "GET", s.URL+queuePath, opToken(s), nil)
	if strings.Contains(body, "ivan") || strings.Contains(body, "example.com") {
		t.Errorf("queue leaks personal data: %s", body)
	}
	_, body = do(t, "GET", s.URL+auditPath, po, nil)
	var entries []struct{ At, Actor, Action string }
	if err := json.Unmarshal([]byte(body), &entries); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Action, "correct_outage notice=1 ordinal=0") {
			found = e.Actor == "oscar" && e.At != ""
		}
	}
	if !found {
		t.Errorf("no audited correction: %s", body)
	}
}
