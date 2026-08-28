// Package mailer sends transactional emails over SMTP (Gmail by default).
// It's a no-op — Send just returns nil without dialing out — whenever
// SMTP_USER/SMTP_PASS aren't configured, so the app runs fine without them.
package mailer

import (
	"fmt"
	"net/smtp"
)

type Mailer struct {
	host     string
	port     string
	user     string
	pass     string
	fromName string
}

func New(host, port, user, pass, fromName string) *Mailer {
	return &Mailer{host: host, port: port, user: user, pass: pass, fromName: fromName}
}

func (m *Mailer) Configured() bool {
	return m.user != "" && m.pass != ""
}

// Send fires a plain-text email. Callers that don't want a failed send to
// block a request (e.g. registration) should run this in a goroutine.
func (m *Mailer) Send(to, subject, body string) error {
	if !m.Configured() {
		return nil
	}
	addr := fmt.Sprintf("%s:%s", m.host, m.port)
	auth := smtp.PlainAuth("", m.user, m.pass, m.host)
	from := fmt.Sprintf("%s <%s>", m.fromName, m.user)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		from, to, subject, body)
	return smtp.SendMail(addr, auth, m.user, []string{to}, []byte(msg))
}
