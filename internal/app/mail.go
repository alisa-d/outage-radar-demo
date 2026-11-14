package app

import (
	"bytes"
	"mime"
	"mime/quotedprintable"
	"strings"
	"time"
)

// baseURL is PublicURL without a trailing slash, the base of every link in an email.
func (s *subscriptions) baseURL() string { return strings.TrimRight(s.cfg.PublicURL, "/") }

// composeMail builds one raw plain-text message: the subject is Q-encoded and the body is
// quoted-printable UTF-8. extra holds further header lines (without CRLF), placed after Date.
func (s *subscriptions) composeMail(to, subject string, extra []string, body string) string {
	var enc bytes.Buffer
	qp := quotedprintable.NewWriter(&enc)
	qp.Write([]byte(body))
	qp.Close()
	headers := append([]string{
		"From: " + s.cfg.MailFrom,
		"To: " + to,
		"Subject: " + mime.QEncoding.Encode("utf-8", subject),
		"Date: " + s.clk.Now().Format(time.RFC1123Z),
	}, extra...)
	headers = append(headers, "MIME-Version: 1.0", "Content-Type: text/plain; charset=UTF-8", "Content-Transfer-Encoding: quoted-printable")
	return strings.Join(headers, "\r\n") + "\r\n\r\n" + enc.String()
}
