package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// SMTPConfig holds SMTP submission settings (never expose via API).
type SMTPConfig struct {
	Host      string
	Port      int
	Username  string
	Password  string
	FromEmail string
	FromName  string
}

// SMTPMailer sends mail via SMTP (STARTTLS on 587, implicit TLS on 465).
// Timeout bounds dial and subsequent reads/writes. Zero means 20s.
type SMTPMailer struct {
	Cfg     SMTPConfig
	Timeout time.Duration
	// dialContext overrides the TCP/TLS dial. Tests use it; production leaves it nil.
	dialContext func(ctx context.Context, network, addr string) (net.Conn, error)
}

func (m *SMTPMailer) timeout() time.Duration {
	if m.Timeout > 0 {
		return m.Timeout
	}
	return 20 * time.Second
}

func (m *SMTPMailer) Send(ctx context.Context, msg Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	host := strings.TrimSpace(m.Cfg.Host)
	port := m.Cfg.Port
	from := strings.TrimSpace(m.Cfg.FromEmail)
	to := strings.TrimSpace(msg.To)
	if host == "" || port <= 0 || from == "" || to == "" {
		return fmt.Errorf("smtp: incomplete configuration or recipient")
	}
	fromHeader := from
	if name := strings.TrimSpace(m.Cfg.FromName); name != "" {
		fromHeader = fmt.Sprintf("%s <%s>", sanitizeHeader(name), from)
	}
	subject := sanitizeHeader(msg.Subject)
	body := buildMIME(fromHeader, to, subject, msg.TextBody, msg.HTMLBody)
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	dialer := &net.Dialer{Timeout: m.timeout()}
	tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	var (
		conn net.Conn
		err  error
	)
	if m.dialContext != nil {
		conn, err = m.dialContext(ctx, "tcp", addr)
	} else if port == 465 {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsCfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(m.timeout())
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)
	// Close on context cancel or deadline so a stalled read cannot outlive the request
	// even if the platform does not wake a blocked read from SetDeadline alone.
	stopWatch := make(chan struct{})
	defer close(stopWatch)
	go func() {
		timer := time.NewTimer(time.Until(deadline))
		defer timer.Stop()
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-timer.C:
			_ = conn.Close()
		case <-stopWatch:
		}
	}()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer func() { _ = client.Close() }()

	if port != 465 {
		ok, _ := client.Extension("STARTTLS")
		if !ok {
			return fmt.Errorf("smtp: STARTTLS required")
		}
		if err := client.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	user := strings.TrimSpace(m.Cfg.Username)
	pass := m.Cfg.Password
	if user != "" {
		// Auth runs only after implicit TLS (465) or successful STARTTLS.
		auth := smtp.PlainAuth("", user, pass, host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	return client.Quit()
}

func sanitizeHeader(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return strings.TrimSpace(s)
}

func buildMIME(from, to, subject, text, html string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	boundary := "vplayer_boundary_001"
	b.WriteString("Content-Type: multipart/alternative; boundary=" + boundary + "\r\n")
	b.WriteString("\r\n")
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	b.WriteString(text)
	b.WriteString("\r\n")
	if strings.TrimSpace(html) != "" {
		b.WriteString("--" + boundary + "\r\n")
		b.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
		b.WriteString(html)
		b.WriteString("\r\n")
	}
	b.WriteString("--" + boundary + "--\r\n")
	return []byte(b.String())
}
