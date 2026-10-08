package mediamigrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSourceMaxAttempts = 5
	defaultSourceHTTPTimeout = 60 * time.Second
	defaultSourceRetryBase   = 500 * time.Millisecond
)

// httpStatusError is a non-OK HTTP response from a source download.
type httpStatusError struct {
	Code int
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("http %d", e.Code)
}

// IsRetryableSourceError reports whether a source GET should be retried.
func IsRetryableSourceError(err error) bool {
	if err == nil {
		return false
	}
	var st *httpStatusError
	if errors.As(err, &st) {
		switch {
		case st.Code == http.StatusRequestTimeout, // 408
			st.Code == http.StatusTooManyRequests: // 429
			return true
		case st.Code >= 500 && st.Code <= 599:
			return true
		default:
			return false
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timeout"),
		strings.Contains(msg, "connection reset"),
		strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "broken pipe"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "temporary failure"),
		strings.Contains(msg, "server misbehaving"),
		strings.Contains(msg, "tls handshake timeout"),
		strings.Contains(msg, "eof"):
		return true
	default:
		return false
	}
}

func sourceErrorCategory(err error) (category string, status int) {
	var st *httpStatusError
	if errors.As(err, &st) {
		status = st.Code
		if IsRetryableSourceError(err) {
			return "transient_http", status
		}
		return "permanent_http", status
	}
	if IsRetryableSourceError(err) {
		return "transient_network", 0
	}
	return "permanent", 0
}

func (m *Migrator) sourceMaxAttempts() int {
	if m.SourceMaxAttempts > 0 {
		return m.SourceMaxAttempts
	}
	return defaultSourceMaxAttempts
}

func (m *Migrator) sourceRetryBase() time.Duration {
	if m.SourceRetryBase > 0 {
		return m.SourceRetryBase
	}
	return defaultSourceRetryBase
}

func (m *Migrator) sourceHTTPTimeout() time.Duration {
	if m.SourceHTTPTimeout > 0 {
		return m.SourceHTTPTimeout
	}
	return defaultSourceHTTPTimeout
}

func (m *Migrator) httpClient() HTTPDoer {
	if m.HTTP != nil {
		return m.HTTP
	}
	return &http.Client{Timeout: m.sourceHTTPTimeout()}
}

// fetchBytes downloads a source URL with retries (label=source).
func (m *Migrator) fetchBytes(ctx context.Context, rawURL string) ([]byte, error) {
	return m.fetchBytesLabeled(ctx, rawURL, "source", -1, "")
}

// fetchBytesLabeled downloads with per-attempt timeout and transient retries.
// Successfully completed downloads earlier in the same attempt are unaffected
// because callers store results before requesting the next URL.
func (m *Migrator) fetchBytesLabeled(ctx context.Context, rawURL, kind string, index int, name string) ([]byte, error) {
	maxAttempts := m.sourceMaxAttempts()
	base := m.sourceRetryBase()
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		b, err := m.fetchBytesOnce(ctx, rawURL)
		if err == nil {
			return b, nil
		}
		lastErr = err
		category, status := sourceErrorCategory(err)
		m.logSourceAttempt(kind, index, name, attempt, status, category)
		if !IsRetryableSourceError(err) || attempt == maxAttempts {
			return nil, err
		}
		backoff := base * time.Duration(1<<(attempt-1))
		if backoff > 8*time.Second {
			backoff = 8 * time.Second
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
	}
	return nil, lastErr
}

func (m *Migrator) logSourceAttempt(kind string, index int, name string, attempt, status int, category string) {
	parts := []string{
		"source_download_attempt",
		"kind=" + kind,
		"attempt=" + strconv.Itoa(attempt),
		"http_status=" + strconv.Itoa(status),
		"category=" + category,
	}
	if index >= 0 {
		parts = append(parts, "index="+strconv.Itoa(index))
	}
	if name != "" {
		parts = append(parts, "name="+name)
	}
	m.logger().Printf("%s", strings.Join(parts, " "))
}

func (m *Migrator) fetchBytesOnce(ctx context.Context, rawURL string) ([]byte, error) {
	reqCtx, cancel := context.WithTimeout(ctx, m.sourceHTTPTimeout())
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "VPlayerMediaMigrate/1.0")
	resp, err := m.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, &httpStatusError{Code: resp.StatusCode}
	}
	limited := io.LimitReader(resp.Body, m.maxBytes()+1)
	b, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > m.maxBytes() {
		return nil, fmt.Errorf("object too large")
	}
	return b, nil
}
