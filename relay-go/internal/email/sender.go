package email

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Sender sends verification emails.
//
// Production uses SMTP credentials. If Host is empty, the sender runs in dev mode
// and logs the verification link instead of sending.
type Sender struct {
	From     string
	Host     string
	Port     string
	Username string
	Password string
	UseTLS   bool
}

func (s *Sender) SendVerification(toEmail, verifyURL string) error {
	toEmail = strings.TrimSpace(toEmail)
	verifyURL = strings.TrimSpace(verifyURL)
	if toEmail == "" {
		return errors.New("missing recipient email")
	}
	if verifyURL == "" {
		return errors.New("missing verify url")
	}

	if strings.TrimSpace(s.Host) == "" {
		// Dev mode: avoid blocking local tests when SMTP isn't configured.
		log.Printf("SMTP disabled (dev mode). Verification for %s: %s", toEmail, verifyURL)
		return nil
	}

	from := strings.TrimSpace(s.From)
	if from == "" {
		from = "noreply@xmiloatyourside.com"
	}
	port := strings.TrimSpace(s.Port)
	if port == "" {
		port = "587"
	}

	subject := "xMilo — Verify your email"
	body := fmt.Sprintf(
		"Open this link to verify your email for xMilo:\n\n%s\n\nIf you did not request this, you can ignore this message.\n",
		verifyURL,
	)

	msg := strings.Join([]string{
		"From: " + from,
		"To: " + toEmail,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=\"UTF-8\"",
		"",
		body,
	}, "\r\n")

	addr := net.JoinHostPort(strings.TrimSpace(s.Host), port)
	auth := smtp.PlainAuth("", s.Username, s.Password, strings.TrimSpace(s.Host))

	if s.UseTLS {
		// Treat 465 as implicit TLS, everything else as STARTTLS.
		if port == "465" {
			return s.sendImplicitTLS(addr, from, toEmail, msg, auth)
		}
		return s.sendStartTLS(addr, from, toEmail, msg, auth)
	}

	return smtp.SendMail(addr, auth, from, []string{toEmail}, []byte(msg))
}

func (s *Sender) sendStartTLS(addr, from, toEmail, msg string, auth smtp.Auth) error {
	c, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer c.Close()

	_ = c.Hello("xmilo-relay")

	// Upgrade to TLS if offered by the server.
	host, _, _ := net.SplitHostPort(addr)
	tlsConfig := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(tlsConfig); err != nil {
			return err
		}
	}

	// Authenticate if credentials are present.
	if strings.TrimSpace(s.Username) != "" {
		if ok, _ := c.Extension("AUTH"); ok {
			if err := c.Auth(auth); err != nil {
				return err
			}
		}
	}

	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(toEmail); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	_ = c.Quit()
	return nil
}

func (s *Sender) sendImplicitTLS(addr, from, toEmail, msg string, auth smtp.Auth) error {
	host, _, _ := net.SplitHostPort(addr)
	dialer := &net.Dialer{Timeout: 10 * time.Second}

	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return err
	}
	defer conn.Close()

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Close()

	_ = c.Hello("xmilo-relay")

	if strings.TrimSpace(s.Username) != "" {
		if ok, _ := c.Extension("AUTH"); ok {
			if err := c.Auth(auth); err != nil {
				return err
			}
		}
	}

	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(toEmail); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	_ = c.Quit()
	return nil
}
