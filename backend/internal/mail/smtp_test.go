package mail

import (
	"bufio"
	"context"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSMTP_failsClosedWithoutSTARTTLS(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	sawAuth := make(chan struct{}, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(2 * time.Second))
		_, _ = c.Write([]byte("220 localhost\r\n"))
		r := bufio.NewReader(c)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			up := strings.ToUpper(line)
			switch {
			case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
				_, _ = c.Write([]byte("250-localhost\r\n250 HELP\r\n"))
			case strings.HasPrefix(up, "AUTH"):
				select {
				case sawAuth <- struct{}{}:
				default:
				}
				_, _ = c.Write([]byte("535 no\r\n"))
				return
			default:
				_, _ = c.Write([]byte("250 ok\r\n"))
			}
		}
	}()

	_, port, _ := net.SplitHostPort(ln.Addr().String())
	var p int
	for _, ch := range port {
		p = p*10 + int(ch-'0')
	}
	m := &SMTPMailer{Cfg: SMTPConfig{
		Host: "127.0.0.1", Port: p, Username: "u", Password: "secret-not-logged",
		FromEmail: "from@example.com", FromName: "V",
	}, Timeout: time.Second}
	err = m.Send(context.Background(), Message{To: "to@example.com", Subject: "s", TextBody: "code 000000"})
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("err=%v", err)
	}
	select {
	case <-sawAuth:
		t.Fatal("authenticated before TLS")
	default:
	}
}

func TestSMTP_canceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := &SMTPMailer{Cfg: SMTPConfig{
		Host: "127.0.0.1", Port: 2525, FromEmail: "from@example.com",
	}, Timeout: 200 * time.Millisecond}
	if err := m.Send(ctx, Message{To: "to@example.com", Subject: "s", TextBody: "x"}); err == nil {
		t.Fatal("expected context error")
	}
}

// stallConn blocks Read until Close or the deadline set by the mailer.
type stallConn struct {
	mu       sync.Mutex
	deadline time.Time
	closed   chan struct{}
	once     sync.Once
}

func newStallConn() *stallConn {
	return &stallConn{closed: make(chan struct{})}
}

func (c *stallConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	dl := c.deadline
	c.mu.Unlock()
	var timer <-chan time.Time
	if !dl.IsZero() {
		wait := time.Until(dl)
		if wait <= 0 {
			return 0, os.ErrDeadlineExceeded
		}
		t := time.NewTimer(wait)
		defer t.Stop()
		timer = t.C
	}
	select {
	case <-c.closed:
		return 0, net.ErrClosed
	case <-timer:
		return 0, os.ErrDeadlineExceeded
	}
}

func (c *stallConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *stallConn) LocalAddr() net.Addr         { return stubAddr("local") }
func (c *stallConn) RemoteAddr() net.Addr        { return stubAddr("remote") }
func (c *stallConn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	c.deadline = t
	c.mu.Unlock()
	return nil
}
func (c *stallConn) SetReadDeadline(t time.Time) error  { return c.SetDeadline(t) }
func (c *stallConn) SetWriteDeadline(t time.Time) error { return nil }
func (c *stallConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

type stubAddr string

func (s stubAddr) Network() string { return "tcp" }
func (s stubAddr) String() string  { return string(s) }

func TestSMTP_deadlineClosesStalledRead(t *testing.T) {
	m := &SMTPMailer{
		Cfg: SMTPConfig{
			Host: "smtp.test", Port: 587, FromEmail: "from@example.com",
		},
		Timeout: 150 * time.Millisecond,
		dialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return newStallConn(), nil
		},
	}
	start := time.Now()
	err := m.Send(context.Background(), Message{To: "to@example.com", Subject: "s", TextBody: "x"})
	if err == nil {
		t.Fatal("expected timeout")
	}
	if time.Since(start) > time.Second {
		t.Fatalf("stalled read was not bounded: %s err=%v", time.Since(start), err)
	}
}
