package media

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// S3APIError is a sanitized S3/R2 HTTP error (no credentials in Message).
type S3APIError struct {
	Op         string
	Key        string
	StatusCode int
	Message    string
}

func (e *S3APIError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("s3 %s %s: http_%d: %s", e.Op, e.Key, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("s3 %s %s: %s", e.Op, e.Key, e.Message)
}

// AsS3APIError unwraps a typed S3 error if present.
func AsS3APIError(err error) (*S3APIError, bool) {
	var api *S3APIError
	if errors.As(err, &api) {
		return api, true
	}
	return nil, false
}

// IsRetryableS3Error reports whether a Put/Head should be retried.
func IsRetryableS3Error(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "temporary failure") {
		return true
	}
	if api, ok := AsS3APIError(err); ok {
		if api.StatusCode == 429 {
			return true
		}
		if api.StatusCode >= 500 && api.StatusCode <= 599 {
			return true
		}
		return false
	}
	// Ambiguous transport errors without typed status.
	if strings.Contains(msg, "i/o timeout") || strings.Contains(msg, "no such host") {
		return true
	}
	return false
}

// SanitizeS3Message redacts tokens and URLs from error text for logs.
func SanitizeS3Message(raw string) string {
	s := raw
	s = regexp.MustCompile(`(?i)(Credential|Signature|Authorization)=[^,\s]+`).ReplaceAllString(s, "$1=<redacted>")
	s = regexp.MustCompile(`(?i)(AccessKeyId|SecretAccessKey|AWSAccessKeyId)>[^<]+`).ReplaceAllString(s, "$1><redacted>")
	s = regexp.MustCompile(`https?://[^\s"'<>]+`).ReplaceAllStringFunc(s, func(u string) string {
		parsed, err := url.Parse(u)
		if err != nil {
			return "<url>"
		}
		return parsed.Scheme + "://" + parsed.Host + parsed.EscapedPath()
	})
	if len(s) > 240 {
		s = s[:240] + "..."
	}
	return s
}

func parseStatusFromLegacyPutErr(msg string) int {
	// legacy: "s3 put key: 503 Service Unavailable: body"
	if i := strings.Index(msg, "http_"); i >= 0 {
		rest := msg[i+5:]
		n := 0
		for _, c := range rest {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		if n > 0 {
			return n
		}
	}
	parts := strings.Fields(msg)
	for _, p := range parts {
		if len(p) == 3 && p[0] >= '0' && p[0] <= '9' {
			if code, err := strconv.Atoi(p); err == nil && code >= 100 && code < 600 {
				return code
			}
		}
	}
	return 0
}
