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
	"go.opentelemetry.io/otel/metric"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net/http"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/alisa/outage-radar/internal/clock"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	privacyNoticeVersion = "2026-11"
	magicLinkTTL         = time.Hour
	unverifiedTTL        = 24 * time.Hour
	maxWatchedAddresses  = 5
	verifyEmailJob       = "verify-email"
)

type subscriptions struct {
	pool *pgxpool.Pool
	kp   KeyProvider
	clk  clock.Clock
	cfg  Config

	trips   metric.Int64Counter // circuit-breaker trips
	tripped atomic.Bool
}

type verifyEmailPayload struct{ To, Language, Token, Kind string } // Kind "" = verify, "manage"

func (s *subscriptions) sign(id string) string {
	m := hmac.New(sha256.New, s.kp.Derive("magic-link"))
	m.Write([]byte(id))
	return hex.EncodeToString(m.Sum(nil))
}

type watchedAddress struct {
	StreetID int     `json:"street_id"`
	Number   string  `json:"number"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
}

// subscribe always answers 202 with the same body for valid input, whether or not
// the email is already known.
func (s *subscriptions) subscribe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Turnstile string `json:"turnstile_token"`
		Email     string `json:"email"`
		Language  string `json:"language"`
		Addresses []struct {
			StreetID int    `json:"street_id"`
			Number   string `json:"number"`
		} `json:"addresses"`
	}
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in)
	var a *mail.Address
	if err == nil {
		a, err = mail.ParseAddress(in.Email)
	}
	if err != nil || (in.Language != "bg" && in.Language != "en") ||
		len(in.Addresses) < 1 || len(in.Addresses) > maxWatchedAddresses {
		http.Error(w, "invalid subscription", http.StatusBadRequest)
		return
	}
	if !s.turnstileOK(r, in.Turnstile) {
		http.Error(w, "captcha failed", http.StatusBadRequest)
		return
	}
	email := strings.ToLower(strings.TrimSpace(a.Address))
	var wa [][]byte
	var addrKeys []string
	for _, ad := range in.Addresses {
		b, err := lookupNumber(r.Context(), s.pool, strconv.Itoa(ad.StreetID), ad.Number)
		if errors.Is(err, errNotFound) {
			http.Error(w, "unknown address", http.StatusBadRequest)
			return
		} else if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		j, _ := json.Marshal(watchedAddress{ad.StreetID, b.Number, b.Lat, b.Lon})
		enc, err := seal(s.kp, j)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		wa = append(wa, enc)
		addrKeys = append(addrKeys, fmt.Sprintf("%d:%s", ad.StreetID, b.Number))
	}
	if !s.limitSignup(w, r, email, addrKeys) {
		return
	}
	if err := s.create(r.Context(), email, in.Language, wa); err != nil {
		slog.Error("signup failed", "err_kind", fmt.Sprintf("%T", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"if the address is valid, a verification email is on its way"}`))
}

func (s *subscriptions) create(ctx context.Context, email, lang string, wa [][]byte) error {
	idx := blindIndex(s.kp, email)
	enc, err := seal(s.kp, []byte(email))
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var existing, channel int64
	var pendingLang string
	var verified, established bool // established: the Subscriber already has another verified channel
	switch err := tx.QueryRow(ctx, `SELECT s.id, c.id, s.language, c.verified_at IS NOT NULL,
		EXISTS (SELECT 1 FROM contact_channels o WHERE o.subscriber_id = s.id AND o.verified_at IS NOT NULL)
		FROM contact_channels c JOIN subscribers s ON s.id = c.subscriber_id WHERE c.blind_idx = $1`, idx).
		Scan(&existing, &channel, &pendingLang, &verified, &established); {
	case err == nil && verified:
		return nil
	case err == nil:
		// Anyone can repeat a pending email, so the pending signup is kept as it is and only the mail is sent again.
		var forChannel *int64
		if established { // a channel added through management: verifying it must not record consent again
			forChannel = &channel
		}
		if err := s.newLink(ctx, tx, existing, "verify", forChannel, email, pendingLang); err != nil {
			return err
		}
		return tx.Commit(ctx)
	case !errors.Is(err, pgx.ErrNoRows):
		return err
	}
	now := s.clk.Now()
	var sub int64
	if err := tx.QueryRow(ctx, `INSERT INTO subscribers (language, created_at) VALUES ($1, $2) RETURNING id`, lang, now).Scan(&sub); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO contact_channels (subscriber_id, kind, value_enc, blind_idx) VALUES ($1, 'email', $2, $3)`, sub, enc, idx); err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" { // concurrent signup for the same email: stay neutral
			return nil
		}
		return err
	}
	for _, d := range wa {
		if _, err := tx.Exec(ctx, `INSERT INTO watched_addresses (subscriber_id, data_enc) VALUES ($1, $2)`, sub, d); err != nil {
			return err
		}
	}
	if err := s.newLink(ctx, tx, sub, "verify", nil, email, lang); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// verify consumes a magic link, verifying the Contact Channel and recording consent.
func (s *subscriptions) verify(w http.ResponseWriter, r *http.Request) {
	if !s.limitVerify(w, r) {
		return
	}
	var in struct {
		Token   string `json:"token"`
		Consent bool   `json:"consent"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&in) != nil || !in.Consent {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	id, sig, ok := strings.Cut(in.Token, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(s.sign(id))) {
		http.Error(w, "invalid or expired link", http.StatusBadRequest)
		return
	}
	err := s.consume(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "invalid or expired link", http.StatusBadRequest)
		return
	} else if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *subscriptions) consume(ctx context.Context, linkID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	now := s.clk.Now()
	var sub int64
	var channel *int64
	if err := tx.QueryRow(ctx, `UPDATE magic_links SET used_at = $2 WHERE id = $1 AND purpose = 'verify' AND used_at IS NULL AND expires_at > $2 RETURNING subscriber_id, channel_id`, linkID, now).Scan(&sub, &channel); err != nil {
		return err
	}
	if channel != nil { // an added channel: consent was already recorded at first verification
		if _, err := tx.Exec(ctx, `UPDATE contact_channels SET verified_at = $2 WHERE id = $1`, *channel, now); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if _, err := tx.Exec(ctx, `UPDATE contact_channels SET verified_at = $2 WHERE subscriber_id = $1`, sub, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO consents (subscriber_id, at, notice_version) VALUES ($1, $2, $3)`, sub, now, privacyNoticeVersion); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

var manageMail = map[string]struct{ subject, body string }{
	"en": {"Manage your Outage Radar subscription", "Open this link within one hour to manage, export or delete your data:\r\n\r\n%s\r\n\r\nIf you did not ask for this, ignore this message.\r\n"},
	"bg": {"Управлявайте абонамента си за Outage Radar", "Отворете тази връзка до един час, за да управлявате, изтеглите или изтриете данните си:\r\n\r\n%s\r\n\r\nАко не сте го поискали, игнорирайте това съобщение.\r\n"},
}

var verifyMail = map[string]struct{ subject, body string }{
	"en": {"Confirm your Outage Radar subscription", "Confirm your email and agree to the privacy notice (version %s) by opening this link within one hour:\r\n\r\n%s\r\n\r\nIf you did not ask for this, ignore this message.\r\n"},
	"bg": {"Потвърдете абонамента си за Outage Radar", "Потвърдете имейла си и приемете политиката за поверителност (версия %s), като отворите тази връзка до един час:\r\n\r\n%s\r\n\r\nАко не сте го поискали, игнорирайте това съобщение.\r\n"},
}

// sendVerifyEmail is the job handler; failures retry through the queue.
func (s *subscriptions) sendVerifyEmail(ctx context.Context, sealed []byte) error {
	plain, err := unseal(s.kp, sealed)
	if err != nil {
		return err
	}
	var p verifyEmailPayload
	if err := json.Unmarshal(plain, &p); err != nil {
		return err
	}
	tpl := verifyMail[p.Language]
	var body bytes.Buffer
	qp := quotedprintable.NewWriter(&body)
	base := strings.TrimRight(s.cfg.PublicURL, "/")
	if p.Kind == "manage" {
		tpl = manageMail[p.Language]
		fmt.Fprintf(qp, tpl.body, base+"/manage?token="+p.Token)
	} else {
		fmt.Fprintf(qp, tpl.body, privacyNoticeVersion, base+"/verify?token="+p.Token)
	}
	qp.Close()
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n%s",
		s.cfg.MailFrom, p.To, mime.QEncoding.Encode("utf-8", tpl.subject), s.clk.Now().Format(time.RFC1123Z), body.String())
	if err := s.sendMail(p.To, msg); err != nil {
		slog.Warn("verification email failed; will retry")
		return err
	}
	slog.Info("verification email sent")
	return nil
}

// purgeUnverified deletes signups never verified within 24 hours (cascades to their data).
func purgeUnverified(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock) {
	_, err := pool.Exec(ctx, `DELETE FROM subscribers s WHERE created_at <= $1
		AND NOT EXISTS (SELECT 1 FROM contact_channels c WHERE c.subscriber_id = s.id AND c.verified_at IS NOT NULL)`, clk.Now().Add(-unverifiedTTL))
	if err == nil { // channels added through management and never verified
		_, err = pool.Exec(ctx, `DELETE FROM contact_channels WHERE verified_at IS NULL AND created_at <= $1`, clk.Now().Add(-unverifiedTTL))
	}
	if err != nil && ctx.Err() == nil {
		slog.Error("purge unverified failed")
	}
	purgeRateHits(ctx, pool, clk.Now())
}

// sendMail hands a raw message to the SMTP adapter. The SMTP error text can echo the
// recipient, so it is replaced by a fixed one.
func (s *subscriptions) sendMail(to, msg string) error {
	var auth smtp.Auth
	if s.cfg.SMTPUser != "" {
		host, _, _ := strings.Cut(s.cfg.SMTPAddr, ":")
		auth = smtp.PlainAuth("", s.cfg.SMTPUser, s.cfg.SMTPPass, host)
	}
	from, _ := mail.ParseAddress(s.cfg.MailFrom)
	if err := smtp.SendMail(s.cfg.SMTPAddr, auth, from.Address, []string{to}, []byte(msg)); err != nil {
		return errors.New("smtp send failed")
	}
	return nil
}
