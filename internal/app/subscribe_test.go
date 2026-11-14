package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alisa/outage-radar/internal/testkit"
	"github.com/jackc/pgx/v5"
)

type addr struct {
	StreetID int    `json:"street_id"`
	Number   string `json:"number"`
}

func post(t *testing.T, url string, body any) (int, string) {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(out)
}

func signup(t *testing.T, s *testkit.System, email, lang string, addrs ...addr) (int, string) {
	return post(t, s.URL+"/api/subscriptions", map[string]any{"email": email, "language": lang, "addresses": addrs, "turnstile_token": testkit.ValidTurnstileToken})
}

type mailMsg struct{ To, Subject, Body string }

// waitMail waits until the sink holds n messages and returns them decoded.
func waitMail(t *testing.T, s *testkit.System, n int) []mailMsg {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for len(s.SMTP.Messages()) < n && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond) // let an unwanted extra mail show up
	var out []mailMsg
	for _, raw := range s.SMTP.Messages() {
		m, err := mail.ReadMessage(strings.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		subj, _ := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject"))
		body, _ := io.ReadAll(quotedprintable.NewReader(m.Body))
		out = append(out, mailMsg{m.Header.Get("To"), subj, string(body)})
	}
	if len(out) != n {
		t.Fatalf("want %d mails, got %d", n, len(out))
	}
	return out
}

var tokenRe = regexp.MustCompile(`token=([A-Za-z0-9._-]+)`)

func token(t *testing.T, m mailMsg) string {
	t.Helper()
	g := tokenRe.FindStringSubmatch(m.Body)
	if g == nil {
		t.Fatalf("no magic link in %q", m.Body)
	}
	return g[1]
}

func verify(t *testing.T, s *testkit.System, tok string) int {
	c, _ := post(t, s.URL+"/api/verify", map[string]any{"token": tok, "consent": true})
	return c
}

func started(t *testing.T) (*testkit.System, int) {
	s := testkit.Start(t, "all", nil)
	importVarna(t, s)
	return s, streets(t, s, "10135", "ferdinand")[0].ID
}

func TestSignupSendsOneVerificationEmailInChosenLanguage(t *testing.T) {
	s, st := started(t)
	for _, c := range []struct{ email, lang string }{
		{"ivan@example.com", "en"},
		{"maria@example.com", "bg"},
	} {
		if code, _ := signup(t, s, c.email, c.lang, addr{st, "5"}); code != 202 {
			t.Fatalf("status %d", code)
		}
	}
	m := waitMail(t, s, 2)
	if m[0].To != "ivan@example.com" || !strings.Contains(m[0].Subject, "Confirm") {
		t.Errorf("en mail: %+v", m[0])
	}
	if m[1].To != "maria@example.com" || !strings.Contains(m[1].Subject, "Потвърдете") {
		t.Errorf("bg mail: %+v", m[1])
	}
}

func TestMagicLinkVerifiesOnceAndExpires(t *testing.T) {
	s, st := started(t)
	signup(t, s, "ivan@example.com", "en", addr{st, "5"})
	signup(t, s, "old@example.com", "en", addr{st, "5"})
	m := waitMail(t, s, 2)
	if c := verify(t, s, token(t, m[0])+"x"); c != 400 {
		t.Errorf("tampered token: %d", c)
	}
	if c, _ := post(t, s.URL+"/api/verify", map[string]any{"token": token(t, m[0])}); c != 400 {
		t.Errorf("verify without consent: %d", c)
	}
	if c := verify(t, s, token(t, m[0])); c != 204 {
		t.Fatalf("first use: %d", c)
	}
	if c := verify(t, s, token(t, m[0])); c != 400 {
		t.Errorf("reuse: %d", c)
	}
	s.Clock.Advance(2 * time.Hour)
	if c := verify(t, s, token(t, m[1])); c != 400 {
		t.Errorf("expired: %d", c)
	}
}

func TestKnownEmailGetsSameResponseAndNoMail(t *testing.T) {
	s, st := started(t)
	c1, b1 := signup(t, s, "ivan@example.com", "en", addr{st, "5"})
	verify(t, s, token(t, waitMail(t, s, 1)[0]))
	c2, b2 := signup(t, s, "Ivan@Example.com", "bg", addr{st, "9"})
	if c1 != c2 || b1 != b2 {
		t.Errorf("responses differ: %d %q vs %d %q", c1, b1, c2, b2)
	}
	waitMail(t, s, 1) // still just the first
}

func TestSixthWatchedAddressIsRefused(t *testing.T) {
	s, st := started(t)
	five := []addr{{st, "1"}, {st, "2"}, {st, "3"}, {st, "4"}, {st, "5"}}
	if c, _ := signup(t, s, "a@example.com", "en", five...); c != 202 {
		t.Errorf("five: %d", c)
	}
	if c, _ := signup(t, s, "b@example.com", "en", append(five, addr{st, "6"})...); c != 400 {
		t.Errorf("six: %d", c)
	}
}

func dump(t *testing.T, s *testkit.System) string {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), s.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	var sb strings.Builder
	for _, tbl := range []string{"subscribers", "contact_channels", "watched_addresses", "consents", "magic_links", "jobs"} {
		rows, err := conn.Query(context.Background(), "SELECT * FROM "+tbl)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			vals, _ := rows.Values()
			for _, v := range vals {
				if b, ok := v.([]byte); ok {
					sb.Write(b)
				} else {
					fmt.Fprint(&sb, v)
				}
				sb.WriteByte('|')
			}
		}
		rows.Close()
	}
	return sb.String()
}

func count(t *testing.T, s *testkit.System, q string) (n int) {
	conn, err := pgx.Connect(context.Background(), s.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	if err := conn.QueryRow(context.Background(), q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return
}

func TestPersonalDataIsEncryptedAtRestAndAbsentFromLogs(t *testing.T) {
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	s, st := started(t)
	b := number(t, s, st, "5")
	signup(t, s, "zorro.unique@example.com", "en", addr{st, "5"})
	verify(t, s, token(t, waitMail(t, s, 1)[0]))
	signup(t, s, "zorro.unique@example.com", "en", addr{st, "5"})
	verify(t, s, "garbage")

	if count(t, s, "SELECT count(*) FROM contact_channels WHERE verified_at IS NOT NULL") != 1 ||
		count(t, s, "SELECT count(*) FROM consents WHERE notice_version <> '' AND at IS NOT NULL") != 1 {
		t.Fatal("expected one verified channel and one consent record")
	}
	db := dump(t, s)
	for _, secret := range []string{"zorro", "example.com", fmt.Sprint(b.Lat), fmt.Sprint(b.Lon), "Фердинанд"} {
		if strings.Contains(db, secret) {
			t.Errorf("database holds plaintext %q", secret)
		}
	}
	for _, secret := range []string{"zorro", "example.com", fmt.Sprint(b.Lat)} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("logs hold %q", secret)
		}
	}
}

func TestUnverifiedSignupsArePurgedAfter24h(t *testing.T) {
	s, st := started(t)
	signup(t, s, "ivan@example.com", "en", addr{st, "5"})
	waitMail(t, s, 1)
	s.Clock.Advance(23 * time.Hour)
	time.Sleep(500 * time.Millisecond)
	if count(t, s, "SELECT count(*) FROM subscribers") != 1 {
		t.Fatal("purged too early")
	}
	s.Clock.Advance(2 * time.Hour)
	deadline := time.Now().Add(5 * time.Second)
	for count(t, s, "SELECT count(*) FROM subscribers") != 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	for _, q := range []string{"subscribers", "contact_channels", "watched_addresses", "magic_links"} {
		if n := count(t, s, "SELECT count(*) FROM "+q); n != 0 {
			t.Errorf("%s has %d rows after purge", q, n)
		}
	}
}
