package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	scanAlertsJob = "scan-alerts" // payload: notice id; enqueued by ingest in the transaction that stores the Outages
	sendAlertJob  = "send-alert"  // payload: sendAlertPayload; one per Alert and Contact Channel, no personal data
	maxAttempts   = 5
	materialShift = 30 * time.Minute
)

type sendAlertPayload struct{ Alert, Channel int64 }

// scanAlerts creates the Alerts a Notice's Outages call for. It is idempotent: the
// deduplication key and the comparison with the last Alert make a re-run a no-op.
// shortcut: decrypts every verified Subscriber's Watched Addresses per scan; index by street if Subscribers outgrow a demo.
func (s *subscriptions) scanAlerts(ctx context.Context, payload []byte) error {
	noticeID, err := strconv.ParseInt(string(payload), 10, 64)
	if err != nil {
		return nil
	}
	now := s.clk.Now()
	var shadow bool
	if err := s.pool.QueryRow(ctx, `SELECT p.shadow FROM notices n JOIN providers p ON p.id = n.provider_id WHERE n.id = $1`, noticeID).Scan(&shadow); err != nil {
		return err
	}
	type outage struct {
		ordinal    int
		kind       string
		severity   string
		start, end time.Time
		settlement string
		places     []string
	}
	var outages []outage
	rows, err := s.pool.Query(ctx, `SELECT kind, severity, start_at, end_at, settlement, places FROM outages WHERE notice_id = $1 ORDER BY id`, noticeID)
	if err != nil {
		return err
	}
	for rows.Next() {
		o := outage{ordinal: len(outages)}
		if err := rows.Scan(&o.kind, &o.severity, &o.start, &o.end, &o.settlement, &o.places); err != nil {
			rows.Close()
			return err
		}
		outages = append(outages, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	was, err := s.pool.Query(ctx, `SELECT wa.id, wa.subscriber_id, wa.data_enc FROM watched_addresses wa
		WHERE EXISTS (SELECT 1 FROM contact_channels c WHERE c.subscriber_id = wa.subscriber_id AND c.verified_at IS NOT NULL AND c.disabled_at IS NULL)`)
	if err != nil {
		return err
	}
	type watch struct {
		id, sub int64
		enc     []byte
	}
	var watches []watch
	for was.Next() {
		var w watch
		if err := was.Scan(&w.id, &w.sub, &w.enc); err != nil {
			was.Close()
			return err
		}
		watches = append(watches, w)
	}
	was.Close()
	if err := was.Err(); err != nil {
		return err
	}

	resolvers := map[string]*resolver{}
	for _, w := range watches {
		plain, err := unseal(s.kp, w.enc)
		if err != nil {
			return err
		}
		var a watchedAddress
		if err := json.Unmarshal(plain, &a); err != nil {
			return err
		}
		var street, settlement string
		if err := s.pool.QueryRow(ctx, `SELECT st.name, s.name FROM streets st JOIN settlements s ON s.id = st.settlement_id WHERE st.id = $1`, a.StreetID).Scan(&street, &settlement); err != nil {
			if err == pgx.ErrNoRows {
				continue
			}
			return err
		}
		rs, ok := resolvers[settlement]
		if !ok {
			if rs, err = loadResolver(ctx, s.pool, settlement); err != nil {
				return err
			}
			resolvers[settlement] = rs
		}
		b, err := lookupNumber(ctx, s.pool, strconv.Itoa(a.StreetID), a.Number)
		if err != nil {
			if errors.Is(err, errNotFound) {
				continue
			}
			return err
		}
		at := &target{street: street, number: a.Number, pt: [2]float64{b.Lon, b.Lat}}
		for _, o := range outages {
			if !o.end.After(now) {
				continue
			}
			// An Unlocated Outage is Not affected here: it creates no Alerts (ADR 0003).
			m, _, err := matchOutage(rs, o.settlement, o.places, at)
			if err != nil {
				return err
			}
			if m == notAffected {
				continue
			}
			sum := sha256.Sum256([]byte(fmt.Sprint(noticeID, o.ordinal, w.id, m, o.start.Unix(), o.end.Unix())))
			if err := s.createAlert(ctx, hex.EncodeToString(sum[:]), w.sub, w.id, noticeID, o.ordinal, m, o.kind, o.severity, o.start, o.end, shadow); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *subscriptions) createAlert(ctx context.Context, key string, sub, wa, notice int64, ordinal int, match, kind, severity string, start, end time.Time, shadow bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Material change: the Match Result changed or a window edge moved by 30 minutes or more.
	var pm string
	var ps, pe time.Time
	switch err := tx.QueryRow(ctx, `SELECT match_result, start_at, end_at FROM alerts WHERE notice_id = $1 AND ordinal = $2 AND watched_address_id = $3 ORDER BY id DESC LIMIT 1`,
		notice, ordinal, wa).Scan(&pm, &ps, &pe); {
	case err == nil:
		if pm == match && start.Sub(ps).Abs() < materialShift && end.Sub(pe).Abs() < materialShift {
			return nil
		}
	case err != pgx.ErrNoRows:
		return err
	}
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO alerts (created_at, delivery, dedup_key, subscriber_id, watched_address_id, notice_id, ordinal, match_result, severity, start_at, end_at, shadow)
		VALUES ($1, 'pending', $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) ON CONFLICT (dedup_key) DO NOTHING RETURNING id`,
		s.clk.Now(), key, sub, wa, notice, ordinal, match, severity, start, end, shadow).Scan(&id)
	if err == pgx.ErrNoRows {
		return nil
	} else if err != nil {
		return err
	}
	if shadow { // recorded only: no delivery jobs, so going live later sends no backlog
		return tx.Commit(ctx)
	}
	crows, err := tx.Query(ctx, `SELECT id FROM contact_channels WHERE subscriber_id = $1 AND verified_at IS NOT NULL AND disabled_at IS NULL ORDER BY id`, sub)
	if err != nil {
		return err
	}
	var channels []int64
	for crows.Next() {
		var c int64
		if err := crows.Scan(&c); err != nil {
			crows.Close()
			return err
		}
		channels = append(channels, c)
	}
	crows.Close()
	release := quietUntil(s.clk.Now(), kind, start)
	for _, c := range channels {
		p, _ := json.Marshal(sendAlertPayload{id, c})
		if _, err := tx.Exec(ctx, `INSERT INTO jobs (kind, payload, run_at) VALUES ($1, $2, $3)`, sendAlertJob, p, release); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// quietUntil is when an Alert created at now may be sent: nil (at once) outside Quiet Hours
// (22:00-08:00 Europe/Sofia), for emergency Outages, and for Outages starting before 09:00 on the release morning.
// Wall-clock arithmetic in Sofia keeps it right across daylight-saving changes.
func quietUntil(now time.Time, kind string, start time.Time) *time.Time {
	n := now.In(sofia)
	if kind == "emergency" || (n.Hour() >= 8 && n.Hour() < 22) {
		return nil
	}
	day := n.Day()
	if n.Hour() >= 22 {
		day++
	}
	release := time.Date(n.Year(), n.Month(), day, 8, 0, 0, 0, sofia)
	if start.Before(time.Date(n.Year(), n.Month(), day, 9, 0, 0, 0, sofia)) {
		return nil
	}
	return &release
}

// shortcut: wording is inline per language; move to files if a third language or a second channel arrives.
var alertText = map[string]struct {
	utility     map[string]string
	headline    map[string]string // by severity and match, see headline()
	addressWord string
	windowWord  string
	noticeWord  string
	unsub       string
}{
	"en": {map[string]string{"water": "Water", "power": "Power"},
		map[string]string{
			"cut/affected":                  "%s outage expected at your address",
			"cut/possibly_affected":         "%s outage possibly affecting your address",
			"disturbance/affected":          "Possible %s disturbances at your address",
			"disturbance/possibly_affected": "Possible %s disturbances that may affect your address",
		}, "Address", "Time", "Original notice (in Bulgarian)", "Unsubscribe from alerts for this email address"},
	"bg": {map[string]string{"water": "водата", "power": "тока"},
		map[string]string{
			"cut/affected":                  "Очаквано спиране на %s за адреса ви",
			"cut/possibly_affected":         "Възможно спиране на %s, което може да засегне адреса ви",
			"disturbance/affected":          "Възможни смущения в подаването на %s за адреса ви",
			"disturbance/possibly_affected": "Възможни смущения в подаването на %s, които може да засегнат адреса ви",
		}, "Адрес", "Време", "Оригинално съобщение", "Отписване от известията за този имейл"},
}

func (s *subscriptions) unsubToken(channel int64) string {
	id := strconv.FormatInt(channel, 10)
	m := hmac.New(sha256.New, s.kp.Derive("unsubscribe"))
	m.Write([]byte(id))
	return id + "." + hex.EncodeToString(m.Sum(nil))
}

// sendAlert delivers one Alert to one Contact Channel; an error retries the job with backoff.
func (s *subscriptions) sendAlert(ctx context.Context, payload []byte) error {
	var p sendAlertPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil
	}
	var lang, match, severity, noticeText, utility, street string
	var start, end time.Time
	var waEnc, chEnc []byte
	err := s.pool.QueryRow(ctx, `SELECT sub.language, a.match_result, a.severity, a.start_at, a.end_at, n.text, pr.utility, wa.data_enc, c.value_enc
		FROM alerts a JOIN subscribers sub ON sub.id = a.subscriber_id JOIN notices n ON n.id = a.notice_id JOIN providers pr ON pr.id = n.provider_id
		JOIN watched_addresses wa ON wa.id = a.watched_address_id
		JOIN contact_channels c ON c.id = $2 AND c.subscriber_id = a.subscriber_id AND c.verified_at IS NOT NULL AND c.disabled_at IS NULL
		WHERE a.id = $1`, p.Alert, p.Channel).Scan(&lang, &match, &severity, &start, &end, &noticeText, &utility, &waEnc, &chEnc)
	if err == pgx.ErrNoRows {
		return nil // erased, unsubscribed or never verified: nothing to send
	} else if err != nil {
		return err
	}
	wplain, err := unseal(s.kp, waEnc)
	if err != nil {
		return err
	}
	var a watchedAddress
	if err := json.Unmarshal(wplain, &a); err != nil {
		return err
	}
	if err := s.pool.QueryRow(ctx, `SELECT name FROM streets WHERE id = $1`, a.StreetID).Scan(&street); err != nil {
		return err
	}
	to, err := unseal(s.kp, chEnc)
	if err != nil {
		return err
	}
	t := alertText[lang]
	if lang == "en" {
		street = latin(street)
	}
	addr := street + " " + a.Number
	head := fmt.Sprintf(t.headline[severity+"/"+match], t.utility[utility])
	loc := sofia
	window := start.In(loc).Format("2006-01-02 15:04") + " - " + end.In(loc).Format("2006-01-02 15:04")
	unsubAPI := strings.TrimRight(s.cfg.PublicURL, "/") + "/api/unsubscribe?token=" + s.unsubToken(p.Channel)
	unsubPage := strings.TrimRight(s.cfg.PublicURL, "/") + "/unsubscribe?token=" + s.unsubToken(p.Channel)
	var body bytes.Buffer
	qp := quotedprintable.NewWriter(&body)
	fmt.Fprintf(qp, "%s\r\n\r\n%s: %s\r\n%s: %s\r\n\r\n%s:\r\n%s\r\n\r\n-- \r\n%s: %s\r\n", head, t.addressWord, addr, t.windowWord, window, t.noticeWord, plainNotice(noticeText), t.unsub, unsubPage)
	qp.Close()
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nList-Unsubscribe: <%s>\r\nList-Unsubscribe-Post: List-Unsubscribe=One-Click\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n%s",
		s.cfg.MailFrom, to, mime.QEncoding.Encode("utf-8", head), s.clk.Now().Format(time.RFC1123Z), unsubAPI, body.String())

	sendErr := s.sendMail(string(to), msg)
	outcome := "sent"
	if sendErr != nil {
		outcome = "failed"
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO delivery_attempts (alert_id, channel_id, at, outcome) VALUES ($1, $2, $3, $4)`, p.Alert, p.Channel, s.clk.Now(), outcome); err != nil {
		return err
	}
	if sendErr == nil {
		_, err = s.pool.Exec(ctx, `UPDATE alerts SET delivery = 'delivered' WHERE id = $1`, p.Alert)
		return err
	}
	var failed int
	s.pool.QueryRow(ctx, `SELECT count(*) FROM delivery_attempts WHERE alert_id = $1 AND channel_id = $2 AND outcome = 'failed'`, p.Alert, p.Channel).Scan(&failed)
	if failed >= maxAttempts {
		slog.Error("alert delivery gave up")
		s.pool.Exec(ctx, `UPDATE alerts SET delivery = 'failed' WHERE id = $1 AND delivery = 'pending'`, p.Alert)
		return nil
	}
	slog.Warn("alert delivery failed; will retry")
	return sendErr
}

// plainNotice drops the markup the Provider's feed carries.
func plainNotice(t string) string {
	t = strings.NewReplacer("<br/>", "\n", "<br>", "\n", "<br />", "\n").Replace(t)
	var b strings.Builder
	in := false
	for _, r := range t {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return strings.ReplaceAll(strings.TrimSpace(b.String()), "\n", "\r\n")
}

// unsubscribe is the RFC 8058 one-click endpoint: a POST removes the Contact Channel at once
// (and the Subscriber with it when it was the last one), exactly like removal in management.
func (s *subscriptions) unsubscribe(w http.ResponseWriter, r *http.Request) {
	tok := r.URL.Query().Get("token")
	id, _, ok := strings.Cut(tok, ".")
	ch, err := strconv.ParseInt(id, 10, 64)
	if !ok || err != nil || !hmac.Equal([]byte(tok), []byte(s.unsubToken(ch))) {
		http.Error(w, "invalid link", http.StatusBadRequest)
		return
	}
	var sub int64
	err = s.pool.QueryRow(r.Context(), `SELECT subscriber_id FROM contact_channels WHERE id = $1`, ch).Scan(&sub)
	if err == nil {
		_, err = s.deleteChannel(r.Context(), ch, sub)
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) { // already gone: unsubscribing again is fine
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
