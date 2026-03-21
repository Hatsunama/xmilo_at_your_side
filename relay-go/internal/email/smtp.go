package email

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

// Sender sends transactional emails via SMTP.
// When Host is empty (dev/CI) it logs to stdout instead of sending.
type Sender struct {
	From     string
	Host     string
	Port     string
	Username string
	Password string
	UseTLS   bool
}

func (s *Sender) Configured() bool {
	return s.Host != "" && s.Username != "" && s.Password != ""
}

// SendVerification emails a verification link to the user.
// verifyURL is the full https://xmiloatyourside.com/auth/verify?token=... URL.
func (s *Sender) SendVerification(toEmail, verifyURL string) error {
	subject := "Verify your xMilo email"
	body := fmt.Sprintf(
		"Tap the link to verify your email and activate your free xMilo trial:\n\n%s\n\n"+
			"This link expires in 24 hours. If you did not request this, ignore it.\n\n— xMilo",
		verifyURL,
	)
	return s.send(toEmail, subject, body)
}

func (s *Sender) send(to, subject, body string) error {
	if !s.Configured() {
		// Dev mode: print to stdout so you can copy the link during local testing
		fmt.Printf("\n[email-dev] ─────────────────────────────\nTo:      %s\nSubject: %s\n\n%s\n─────────────────────────────\n\n", to, subject, body)
		return nil
	}

	addr := net.JoinHostPort(s.Host, s.Port)
	auth := smtp.PlainAuth("", s.Username, s.Password, s.Host)
	msg := strings.Join([]string{
		"From: " + s.From,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"",
		body,
	}, "\r\n")

	if s.UseTLS {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: s.Host})
		if err != nil {
			return fmt.Errorf("smtp tls dial: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, s.Host)
		if err != nil {
			return fmt.Errorf("smtp client: %w", err)
		}
		defer client.Close()

		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
		if err := client.Mail(s.From); err != nil {
			return err
		}
		if err := client.Rcpt(to); err != nil {
			return err
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		if _, err := fmt.Fprint(w, msg); err != nil {
			return err
		}
		return w.Close()
	}

	return smtp.SendMail(addr, auth, s.From, []string{to}, []byte(msg))
}
