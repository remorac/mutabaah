package services

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
)

// Mailer sends transactional emails for account flows.
type Mailer interface {
	SendPasswordReset(ctx context.Context, to, resetURL string) error
}

// SMTPConfig contains the minimum SMTP settings needed for password reset mail.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	TLSMode  string
}

// SMTPMailer sends password reset emails via net/smtp.
type SMTPMailer struct {
	cfg SMTPConfig
}

func NewSMTPMailer(cfg SMTPConfig) *SMTPMailer {
	return &SMTPMailer{cfg: cfg}
}

func (m *SMTPMailer) SendPasswordReset(ctx context.Context, to, resetURL string) error {
	addr := net.JoinHostPort(m.cfg.Host, strconv.Itoa(m.cfg.Port))
	msg := passwordResetMessage(m.cfg.From, to, resetURL)

	switch strings.ToLower(strings.TrimSpace(m.cfg.TLSMode)) {
	case "implicit":
		return m.sendImplicitTLS(ctx, addr, to, msg)
	case "none":
		return m.send(ctx, addr, to, msg, false)
	default:
		return m.send(ctx, addr, to, msg, true)
	}
}

func (m *SMTPMailer) sendImplicitTLS(ctx context.Context, addr, to string, msg []byte) error {
	dialer := &tls.Dialer{Config: &tls.Config{ServerName: m.cfg.Host}}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	return m.sendWithClient(client, to, msg)
}

func (m *SMTPMailer) send(ctx context.Context, addr, to string, msg []byte, startTLS bool) error {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		return err
	}
	defer client.Close()

	if startTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: m.cfg.Host}); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("smtp server does not support STARTTLS")
		}
	}
	return m.sendWithClient(client, to, msg)
}

func (m *SMTPMailer) sendWithClient(client *smtp.Client, to string, msg []byte) error {
	if m.cfg.Username != "" {
		auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(m.cfg.From); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

func passwordResetMessage(from, to, resetURL string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: Reset your Mutaba'ah Tracker password\r\n")
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: text/plain; charset=UTF-8\r\n")
	fmt.Fprintf(&b, "\r\n")
	fmt.Fprintf(&b, "Use this link to reset your password:\r\n\r\n%s\r\n\r\n", resetURL)
	fmt.Fprintf(&b, "This link expires in 1 hour. If you did not request it, you can ignore this email.\r\n")
	return b.Bytes()
}
