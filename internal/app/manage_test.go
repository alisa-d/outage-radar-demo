package app_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alisa/outage-radar/internal/testkit"
)

func do(t *testing.T, method, url, session string, body any) (int, string) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, r)
	if session != "" {
		req.Header.Set("Authorization", "Bearer "+session)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(out)
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// subscribed signs up and verifies ivan@example.com; one mail has been sent.
func subscribed(t *testing.T) (*testkit.System, int) {
	s, st := started(t)
	signup(t, s, "ivan@example.com", "en", addr{st, "5"})
	verify(t, s, token(t, waitMail(t, s, 1)[0]))
	return s, st
}

func requestLink(t *testing.T, s *testkit.System, email string) (int, string) {
	return post(t, s.URL+"/api/manage/request", map[string]any{"email": email})
}

func openSession(t *testing.T, s *testkit.System, tok string) (int, string) {
	c, b := post(t, s.URL+"/api/manage/session", map[string]any{"token": tok})
	if c != 200 {
		return c, ""
	}
	var o struct{ Session string }
	json.Unmarshal([]byte(b), &o)
	return c, o.Session
}

// manageSession requests a link (the nth mail overall) and opens a session.
func manageSession(t *testing.T, s *testkit.System, email string, nth int) string {
	t.Helper()
	requestLink(t, s, email)
	c, sess := openSession(t, s, token(t, waitMail(t, s, nth)[nth-1]))
	if c != 200 || sess == "" {
		t.Fatalf("session: %d", c)
	}
	return sess
}

func TestManagementLinkIsNeutralSingleUseAndExpires(t *testing.T) {
	s, st := subscribed(t)
	signup(t, s, "pending@example.com", "en", addr{st, "6"})
	waitMail(t, s, 2)
	c1, b1 := requestLink(t, s, "ivan@example.com")
	c2, b2 := requestLink(t, s, "nobody@example.com")
	c3, b3 := requestLink(t, s, "pending@example.com")
	if c1 != c2 || b1 != b2 || c1 != c3 || b1 != b3 {
		t.Errorf("responses differ: %d %q / %d %q / %d %q", c1, b1, c2, b2, c3, b3)
	}
	m := waitMail(t, s, 3) // only the known, verified email got one
	if m[2].To != "ivan@example.com" {
		t.Fatalf("to %q", m[2].To)
	}
	tok := token(t, m[2])
	if c, _ := post(t, s.URL+"/api/verify", map[string]any{"token": tok, "consent": true}); c != 400 {
		t.Errorf("management link accepted as verify link: %d", c)
	}
	if c, sess := openSession(t, s, tok); c != 200 || sess == "" {
		t.Fatalf("first use: %d", c)
	}
	if c, _ := openSession(t, s, tok); c != 400 {
		t.Errorf("reuse: %d", c)
	}
	requestLink(t, s, "ivan@example.com")
	late := token(t, waitMail(t, s, 4)[3])
	s.Clock.Advance(2 * time.Hour)
	if c, _ := openSession(t, s, late); c != 400 {
		t.Errorf("expired: %d", c)
	}
	if c, _ := do(t, "GET", s.URL+"/api/manage", "garbage", nil); c != 401 {
		t.Errorf("bad session: %d", c)
	}
}

func TestManageAddressesLanguageAndChannels(t *testing.T) {
	s, st := subscribed(t)
	sess := manageSession(t, s, "ivan@example.com", 2)
	api := s.URL + "/api/manage"

	for _, n := range []string{"6", "7", "8", "9"} {
		if c, _ := do(t, "POST", api+"/addresses", sess, addr{st, n}); c != 201 {
			t.Fatalf("add %s: %d", n, c)
		}
	}
	if c, _ := do(t, "POST", api+"/addresses", sess, addr{st, "10"}); c != 400 {
		t.Errorf("sixth address: %d", c)
	}
	var view struct {
		Language  string
		Addresses []struct{ ID int64 }
		Channels  []struct {
			ID       int64
			Email    string
			Verified bool
		}
	}
	get := func() {
		c, b := do(t, "GET", api, sess, nil)
		if c != 200 {
			t.Fatalf("get: %d", c)
		}
		view.Addresses, view.Channels = nil, nil
		json.Unmarshal([]byte(b), &view)
	}
	get()
	if len(view.Addresses) != 5 {
		t.Fatalf("addresses: %+v", view.Addresses)
	}
	if c, _ := do(t, "DELETE", api+"/addresses/"+itoa(view.Addresses[0].ID), sess, nil); c != 204 {
		t.Errorf("remove address: %d", c)
	}
	if c, _ := do(t, "PUT", api+"/language", sess, map[string]any{"language": "bg"}); c != 204 {
		t.Errorf("language: %d", c)
	}
	if c, _ := do(t, "PUT", api+"/language", sess, map[string]any{"language": "xx"}); c != 400 {
		t.Errorf("bad language: %d", c)
	}
	get()
	if len(view.Addresses) != 4 || view.Language != "bg" {
		t.Errorf("view: %+v", view)
	}

	// A new channel needs its own verification mail, in the new language.
	if c, _ := do(t, "POST", api+"/channels", sess, map[string]any{"email": "ivan2@example.com"}); c != 202 {
		t.Fatalf("add channel: %d", c)
	}
	m := waitMail(t, s, 3)
	if m[2].To != "ivan2@example.com" || !strings.Contains(m[2].Subject, "Потвърдете") {
		t.Fatalf("channel mail: %+v", m[2])
	}
	// An existing email gets the same answer and no mail.
	if c, _ := do(t, "POST", api+"/channels", sess, map[string]any{"email": "ivan@example.com"}); c != 202 {
		t.Errorf("duplicate channel: %d", c)
	}
	waitMail(t, s, 3)
	if verify(t, s, token(t, m[2])) != 204 {
		t.Fatal("verify channel")
	}
	get()
	if len(view.Channels) != 2 || !view.Channels[1].Verified || view.Channels[1].Email != "ivan2@example.com" {
		t.Fatalf("channels: %+v", view.Channels)
	}
	if count(t, s, "SELECT count(*) FROM consents") != 1 {
		t.Error("channel verification must not add a consent record")
	}
	// Removing one of two keeps the Subscriber.
	if c, _ := do(t, "DELETE", api+"/channels/"+itoa(view.Channels[1].ID), sess, nil); c != 204 {
		t.Errorf("remove channel: %d", c)
	}
	if count(t, s, "SELECT count(*) FROM subscribers") != 1 || count(t, s, "SELECT count(*) FROM contact_channels") != 1 {
		t.Error("subscriber should remain with one channel")
	}
	// Removing the last deletes everything.
	get()
	if c, _ := do(t, "DELETE", api+"/channels/"+itoa(view.Channels[0].ID), sess, nil); c != 204 {
		t.Errorf("remove last channel: %d", c)
	}
	assertNothingLeft(t, s)
}

func TestUnverifiedChannelIsPurgedAfter24h(t *testing.T) {
	s, _ := subscribed(t)
	sess := manageSession(t, s, "ivan@example.com", 2)
	do(t, "POST", s.URL+"/api/manage/channels", sess, map[string]any{"email": "ivan2@example.com"})
	s.Clock.Advance(25 * time.Hour)
	deadline := time.Now().Add(5 * time.Second)
	for count(t, s, "SELECT count(*) FROM contact_channels") != 1 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if n := count(t, s, "SELECT count(*) FROM contact_channels"); n != 1 || count(t, s, "SELECT count(*) FROM subscribers") != 1 {
		t.Errorf("channels %d", n)
	}
}

func assertNothingLeft(t *testing.T, s *testkit.System) {
	t.Helper()
	for _, tbl := range []string{"subscribers", "contact_channels", "watched_addresses", "consents", "magic_links"} {
		if n := count(t, s, "SELECT count(*) FROM "+tbl); n != 0 {
			t.Errorf("%s has %d rows", tbl, n)
		}
	}
}

func TestExportHoldsEveryPieceOfPersonalData(t *testing.T) {
	s, st := subscribed(t)
	b := number(t, s, st, "5")
	sess := manageSession(t, s, "ivan@example.com", 2)
	c, body := do(t, "GET", s.URL+"/api/manage/export", sess, nil)
	if c != 200 {
		t.Fatalf("export: %d", c)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	flat := strings.ReplaceAll(body, " ", "")
	for _, want := range []string{"ivan@example.com", `"language":"en"`, `"number":"5"`, `"street_id":` + strconv.Itoa(st), "2026-11", "verified_at", "created_at",
		`"lat":` + strconv.FormatFloat(b.Lat, 'g', -1, 64), `"lon":` + strconv.FormatFloat(b.Lon, 'g', -1, 64)} {
		if !strings.Contains(flat, want) {
			t.Errorf("export lacks %q: %s", want, body)
		}
	}
}

func TestDeleteEverythingLeavesNothing(t *testing.T) {
	s, _ := subscribed(t)
	sess := manageSession(t, s, "ivan@example.com", 2)
	if c, _ := do(t, "DELETE", s.URL+"/api/manage", sess, nil); c != 204 {
		t.Fatalf("delete: %d", c)
	}
	assertNothingLeft(t, s)
	if c, _ := do(t, "GET", s.URL+"/api/manage", sess, nil); c != 401 {
		t.Errorf("session after erasure: %d", c)
	}
}
