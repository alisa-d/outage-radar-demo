package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const sessionTTL = 30 * time.Minute

func (s *subscriptions) routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/manage/request", s.requestManageLink)
	mux.HandleFunc("POST /api/manage/session", s.openSession)
	mux.HandleFunc("GET /api/manage", s.managed(s.view))
	mux.HandleFunc("DELETE /api/manage", s.managed(s.erase))
	mux.HandleFunc("GET /api/manage/export", s.managed(s.export))
	mux.HandleFunc("PUT /api/manage/language", s.managed(s.setLanguage))
	mux.HandleFunc("POST /api/manage/addresses", s.managed(s.addAddress))
	mux.HandleFunc("DELETE /api/manage/addresses/{id}", s.managed(s.removeAddress))
	mux.HandleFunc("POST /api/manage/channels", s.managed(s.addChannel))
	mux.HandleFunc("DELETE /api/manage/channels/{id}", s.managed(s.removeChannel))
}

func (s *subscriptions) sessionSig(body string) string {
	m := hmac.New(sha256.New, s.kp.Derive("manage-session"))
	m.Write([]byte(body))
	return hex.EncodeToString(m.Sum(nil))
}

// managed authenticates a stateless, short-lived session token ("sub.expiry.sig").
// An erased Subscriber's token stops working because every handler scopes by id.
func (s *subscriptions) managed(h func(http.ResponseWriter, *http.Request, int64)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		i := strings.LastIndex(tok, ".")
		parts := strings.Split(tok, ".")
		if i < 0 || len(parts) != 3 || !hmac.Equal([]byte(tok[i+1:]), []byte(s.sessionSig(tok[:i]))) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		sub, err1 := strconv.ParseInt(parts[0], 10, 64)
		exp, err2 := strconv.ParseInt(parts[1], 10, 64)
		var ok bool
		if err1 == nil && err2 == nil && s.clk.Now().Unix() < exp {
			err := s.pool.QueryRow(r.Context(), `SELECT true FROM subscribers WHERE id = $1`, sub).Scan(&ok)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h(w, r, sub)
	}
}

func fail(w http.ResponseWriter, err error) {
	slog.Error("management request failed", "err_kind", fmt.Sprintf("%T", err))
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func parseEmail(v string) (string, bool) {
	a, err := mail.ParseAddress(v)
	if err != nil {
		return "", false
	}
	return strings.ToLower(strings.TrimSpace(a.Address)), true
}

// newLink records a magic link and queues its email, inside tx.
func (s *subscriptions) newLink(ctx context.Context, tx pgx.Tx, sub int64, purpose string, channel *int64, to, lang string) error {
	idb := make([]byte, 16)
	rand.Read(idb)
	id := hex.EncodeToString(idb)
	kind := ""
	if purpose == "manage" {
		kind = "manage"
	}
	payload, err := json.Marshal(verifyEmailPayload{to, lang, id + "." + s.sign(id), kind})
	if err != nil {
		return err
	}
	sealed, err := seal(s.kp, payload)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO magic_links (id, subscriber_id, expires_at, purpose, channel_id) VALUES ($1, $2, $3, $4, $5)`, id, sub, s.clk.Now().Add(magicLinkTTL), purpose, channel); err != nil {
		return err
	}
	return Enqueue(ctx, tx, verifyEmailJob, sealed)
}

const neutralLink = `{"status":"if the email is subscribed, a management link is on its way"}`

// requestManageLink answers identically whether or not the email is known.
func (s *subscriptions) requestManageLink(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email string }
	email, ok := "", false
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&in) == nil {
		email, ok = parseEmail(in.Email)
	}
	if !ok {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if !s.limitManageRequest(w, r, email) {
		return
	}
	err := func() error {
		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			return err
		}
		defer tx.Rollback(r.Context())
		var sub int64
		var lang string
		err = tx.QueryRow(r.Context(), `SELECT s.id, s.language FROM contact_channels c JOIN subscribers s ON s.id = c.subscriber_id
			WHERE c.blind_idx = $1 AND c.verified_at IS NOT NULL AND c.disabled_at IS NULL`, blindIndex(s.kp, email)).Scan(&sub, &lang)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		if err := s.newLink(r.Context(), tx, sub, "manage", nil, email, lang); err != nil {
			return err
		}
		return tx.Commit(r.Context())
	}()
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(neutralLink))
}

// openSession consumes a management link (once) and returns a session token.
func (s *subscriptions) openSession(w http.ResponseWriter, r *http.Request) {
	var in struct{ Token string }
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&in)
	id, sig, ok := strings.Cut(in.Token, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(s.sign(id))) {
		http.Error(w, "invalid or expired link", http.StatusBadRequest)
		return
	}
	now := s.clk.Now()
	var sub int64
	err := s.pool.QueryRow(r.Context(), `UPDATE magic_links SET used_at = $2 WHERE id = $1 AND purpose = 'manage' AND used_at IS NULL AND expires_at > $2 RETURNING subscriber_id`, id, now).Scan(&sub)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "invalid or expired link", http.StatusBadRequest)
		return
	} else if err != nil {
		fail(w, err)
		return
	}
	body := fmt.Sprintf("%d.%d", sub, now.Add(sessionTTL).Unix())
	json.NewEncoder(w).Encode(map[string]string{"session": body + "." + s.sessionSig(body)})
}

type channelView struct {
	ID         int64      `json:"id"`
	Email      string     `json:"email"`
	Verified   bool       `json:"verified"`
	VerifiedAt *time.Time `json:"verified_at"`
	DisabledAt *time.Time `json:"disabled_at"`
	CreatedAt  *time.Time `json:"created_at"`
}

type addressView struct {
	ID int64 `json:"id"`
	watchedAddress
}

type consentView struct {
	At            time.Time `json:"at"`
	NoticeVersion string    `json:"notice_version"`
}

type subscriberView struct {
	Language  string        `json:"language"`
	CreatedAt time.Time     `json:"created_at"`
	Channels  []channelView `json:"channels"`
	Addresses []addressView `json:"addresses"`
	Consents  []consentView `json:"consents"`
}

func (s *subscriptions) load(ctx context.Context, sub int64) (*subscriberView, error) {
	v := &subscriberView{Channels: []channelView{}, Addresses: []addressView{}, Consents: []consentView{}}
	if err := s.pool.QueryRow(ctx, `SELECT language, created_at FROM subscribers WHERE id = $1`, sub).Scan(&v.Language, &v.CreatedAt); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id, value_enc, verified_at, disabled_at, created_at FROM contact_channels WHERE subscriber_id = $1 ORDER BY id`, sub)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c channelView
		var enc []byte
		if err := rows.Scan(&c.ID, &enc, &c.VerifiedAt, &c.DisabledAt, &c.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		plain, err := unseal(s.kp, enc)
		if err != nil {
			rows.Close()
			return nil, err
		}
		c.Email, c.Verified = string(plain), c.VerifiedAt != nil
		v.Channels = append(v.Channels, c)
	}
	rows.Close()
	rows, err = s.pool.Query(ctx, `SELECT id, data_enc FROM watched_addresses WHERE subscriber_id = $1 ORDER BY id`, sub)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var a addressView
		var enc []byte
		if err := rows.Scan(&a.ID, &enc); err != nil {
			rows.Close()
			return nil, err
		}
		plain, err := unseal(s.kp, enc)
		if err == nil {
			err = json.Unmarshal(plain, &a.watchedAddress)
		}
		if err != nil {
			rows.Close()
			return nil, err
		}
		v.Addresses = append(v.Addresses, a)
	}
	rows.Close()
	rows, err = s.pool.Query(ctx, `SELECT at, notice_version FROM consents WHERE subscriber_id = $1 ORDER BY id`, sub)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c consentView
		if err := rows.Scan(&c.At, &c.NoticeVersion); err != nil {
			rows.Close()
			return nil, err
		}
		v.Consents = append(v.Consents, c)
	}
	rows.Close()
	return v, rows.Err()
}

func (s *subscriptions) view(w http.ResponseWriter, r *http.Request, sub int64) {
	v, err := s.load(r.Context(), sub)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// shortcut: the export is the same document as the management view, which holds every personal field; add fields to both if a table gains any.
func (s *subscriptions) export(w http.ResponseWriter, r *http.Request, sub int64) {
	w.Header().Set("Content-Disposition", `attachment; filename="outage-radar-export.json"`)
	s.view(w, r, sub)
}

func (s *subscriptions) erase(w http.ResponseWriter, r *http.Request, sub int64) {
	if err := s.eraseSubscriber(r.Context(), sub); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// eraseSubscriber is the one erasure path, shared by self-service and the Privacy officer.
func (s *subscriptions) eraseSubscriber(ctx context.Context, sub int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM subscribers WHERE id = $1`, sub)
	return err
}

func (s *subscriptions) setLanguage(w http.ResponseWriter, r *http.Request, sub int64) {
	var in struct{ Language string }
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in) != nil || (in.Language != "bg" && in.Language != "en") {
		http.Error(w, "invalid language", http.StatusBadRequest)
		return
	}
	if _, err := s.pool.Exec(r.Context(), `UPDATE subscribers SET language = $2 WHERE id = $1`, sub, in.Language); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *subscriptions) addAddress(w http.ResponseWriter, r *http.Request, sub int64) {
	var in struct {
		StreetID int    `json:"street_id"`
		Number   string `json:"number"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in) != nil {
		http.Error(w, "invalid address", http.StatusBadRequest)
		return
	}
	b, err := lookupNumber(r.Context(), s.pool, strconv.Itoa(in.StreetID), in.Number)
	if errors.Is(err, errNotFound) {
		http.Error(w, "unknown address", http.StatusBadRequest)
		return
	} else if err != nil {
		fail(w, err)
		return
	}
	j, _ := json.Marshal(watchedAddress{in.StreetID, b.Number, b.Lat, b.Lon})
	enc, err := seal(s.kp, j)
	if err != nil {
		fail(w, err)
		return
	}
	// One statement so concurrent adds cannot pass the cap together.
	tag, err := s.pool.Exec(r.Context(), `INSERT INTO watched_addresses (subscriber_id, data_enc)
		SELECT $1, $2 WHERE (SELECT count(*) FROM watched_addresses WHERE subscriber_id = $1) < $3`, sub, enc, maxWatchedAddresses)
	if err != nil {
		fail(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "too many addresses", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *subscriptions) removeAddress(w http.ResponseWriter, r *http.Request, sub int64) {
	tag, err := s.pool.Exec(r.Context(), `DELETE FROM watched_addresses WHERE id = $1 AND subscriber_id = $2`, r.PathValue("id"), sub)
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "22P02" { // non-numeric id
			http.NotFound(w, r)
			return
		}
		fail(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// addChannel always answers 202, and sends mail only for a new email.
func (s *subscriptions) addChannel(w http.ResponseWriter, r *http.Request, sub int64) {
	var in struct{ Email string }
	email, ok := "", false
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&in) == nil {
		email, ok = parseEmail(in.Email)
	}
	if !ok {
		http.Error(w, "invalid email", http.StatusBadRequest)
		return
	}
	enc, err := seal(s.kp, []byte(email))
	if err == nil {
		err = func() error {
			tx, err := s.pool.Begin(r.Context())
			if err != nil {
				return err
			}
			defer tx.Rollback(r.Context())
			var ch int64
			var lang string
			err = tx.QueryRow(r.Context(), `INSERT INTO contact_channels (subscriber_id, kind, value_enc, blind_idx, created_at)
				VALUES ($1, 'email', $2, $3, $4) ON CONFLICT (blind_idx) DO NOTHING RETURNING id,
				(SELECT language FROM subscribers WHERE id = $1)`, sub, enc, blindIndex(s.kp, email), s.clk.Now()).Scan(&ch, &lang)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			} else if err != nil {
				return err
			}
			if err := s.newLink(r.Context(), tx, sub, "verify", &ch, email, lang); err != nil {
				return err
			}
			return tx.Commit(r.Context())
		}()
	}
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"if the email is new, a verification email is on its way"}`))
}

// deleteChannel removes a Contact Channel, and the whole Subscriber when no verified channel would remain.
// It reports whether the channel existed.
func (s *subscriptions) deleteChannel(ctx context.Context, id, sub int64) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `DELETE FROM contact_channels WHERE id = $1 AND subscriber_id = $2`, id, sub)
	if err != nil || tag.RowsAffected() == 0 {
		return false, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM subscribers WHERE id = $1
		AND NOT EXISTS (SELECT 1 FROM contact_channels WHERE subscriber_id = $1 AND verified_at IS NOT NULL)`, sub); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (s *subscriptions) removeChannel(w http.ResponseWriter, r *http.Request, sub int64) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	found, err := s.deleteChannel(r.Context(), id, sub)
	if err != nil {
		fail(w, err)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
