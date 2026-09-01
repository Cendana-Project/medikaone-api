package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type Sender interface {
	Send(to, subject, htmlBody string) error
	SendWithContext(ctx context.Context, to, subject, htmlBody string) error
}

var (
	ErrDeliveryDisabled = errors.New("smtp delivery is disabled")
	ErrDeliveryBusy     = errors.New("smtp delivery capacity is busy")
	smtpDeliverySlots   = make(chan struct{}, 4)
)

type SMTPSender struct {
	cfg *Config
}

func NewSMTPSender(cfg *Config) *SMTPSender {
	return &SMTPSender{cfg: cfg}
}

func (s *SMTPSender) Send(to, subject, htmlBody string) error {
	return s.SendWithContext(context.Background(), to, subject, htmlBody)
}

func (s *SMTPSender) SendWithContext(ctx context.Context, to, subject, htmlBody string) error {
	if s.cfg == nil || !s.cfg.Enabled {
		return ErrDeliveryDisabled
	}
	if s.cfg.Timeout <= 0 {
		return errors.New("smtp timeout must be positive")
	}
	if !s.cfg.UseSTARTTLS {
		return errors.New("smtp STARTTLS is required")
	}
	if err := validateHeaderValue("subject", subject); err != nil {
		return err
	}

	from, err := mail.ParseAddress(s.cfg.FromEmail)
	if err != nil {
		return errors.New("invalid smtp sender address")
	}
	recipient, err := mail.ParseAddress(to)
	if err != nil {
		return errors.New("invalid smtp recipient address")
	}
	fromHeader := (&mail.Address{Name: s.cfg.FromName, Address: from.Address}).String()
	message := buildMessage(fromHeader, recipient.String(), subject, htmlBody)
	select {
	case smtpDeliverySlots <- struct{}{}:
		defer func() { <-smtpDeliverySlots }()
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrDeliveryBusy
	}

	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	dialer := &net.Dialer{Timeout: s.cfg.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("connect to smtp server: %w", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(s.cfg.Timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set smtp deadline: %w", err)
	}
	cancelWatch := make(chan struct{})
	defer close(cancelWatch)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.SetDeadline(time.Now())
		case <-cancelWatch:
		}
	}()

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("initialize smtp session: %w", err)
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); !ok {
		return errors.New("smtp server does not advertise required STARTTLS")
	}
	if err := client.StartTLS(&tls.Config{
		ServerName: s.cfg.Host,
		MinVersion: tls.VersionTLS12,
	}); err != nil {
		return fmt.Errorf("start smtp TLS: %w", err)
	}

	if ok, _ := client.Extension("AUTH"); !ok {
		return errors.New("smtp server does not advertise required authentication")
	}
	if err := client.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
		return fmt.Errorf("authenticate smtp session: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("set smtp sender: %w", err)
	}
	if err := client.Rcpt(recipient.Address); err != nil {
		return fmt.Errorf("set smtp recipient: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("open smtp message: %w", err)
	}
	if _, err := w.Write(message); err != nil {
		_ = w.Close()
		return fmt.Errorf("write smtp message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close smtp message: %w", err)
	}
	// w.Close waits for the server's final DATA response. At that point the
	// message has been accepted; a later QUIT/connection failure must not make
	// callers invalidate a PIN that may already be in the recipient's mailbox.
	_ = client.Quit()
	return nil
}

func buildMessage(from, to, subject, htmlBody string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", subject))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)
	return []byte(b.String())
}

func validateHeaderValue(name, value string) error {
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s contains invalid newline characters", name)
	}
	return nil
}
