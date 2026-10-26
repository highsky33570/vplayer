package mediasign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// TTLMin and TTLMax bound how long a signed media URL may live.
	TTLMin = 30
	TTLMax = 3600
)

var errDenied = errors.New("media denied")

// Sign builds an HMAC-SHA256 over the canonical object key and expiry.
// The signature covers the key, so it cannot be reused for a different path.
func Sign(secret, objectKey string, exp int64) (string, error) {
	key, err := CanonicalObjectKey(objectKey)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(secret) == "" {
		return "", errDenied
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedPayload(key, exp)))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// Verify checks signature, expiry, and that exp is not beyond the configured max TTL.
func Verify(secret, objectKey, sig string, exp, now int64, maxTTL int) error {
	key, err := CanonicalObjectKey(objectKey)
	if err != nil {
		return errDenied
	}
	if strings.TrimSpace(secret) == "" || exp <= 0 || now > exp {
		return errDenied
	}
	if maxTTL < TTLMin {
		maxTTL = TTLMin
	}
	if maxTTL > TTLMax {
		maxTTL = TTLMax
	}
	if exp-now > int64(maxTTL) {
		return errDenied
	}
	expect, err := Sign(secret, key, exp)
	if err != nil {
		return errDenied
	}
	got, err := hex.DecodeString(strings.TrimSpace(sig))
	if err != nil || len(got) != sha256.Size {
		return errDenied
	}
	want, err := hex.DecodeString(expect)
	if err != nil || !hmac.Equal(got, want) {
		return errDenied
	}
	return nil
}

func signedPayload(objectKey string, exp int64) string {
	return objectKey + "\n" + strconv.FormatInt(exp, 10)
}

// SignedMediaURL is the browser-facing protected URL for one object.
func SignedMediaURL(publicBase, objectKey, secret string, ttlSec int, now time.Time) (string, error) {
	if ttlSec < TTLMin {
		ttlSec = TTLMin
	}
	if ttlSec > TTLMax {
		ttlSec = TTLMax
	}
	key, err := CanonicalObjectKey(objectKey)
	if err != nil {
		return "", err
	}
	exp := now.Add(time.Duration(ttlSec) * time.Second).Unix()
	sig, err := Sign(secret, key, exp)
	if err != nil {
		return "", err
	}
	base := strings.TrimRight(strings.TrimSpace(publicBase), "/")
	var b strings.Builder
	if base != "" {
		b.WriteString(base)
	}
	b.WriteString("/api/v1/media/")
	parts := strings.Split(key, "/")
	for i, p := range parts {
		if i > 0 {
			b.WriteByte('/')
		}
		b.WriteString(url.PathEscape(p))
	}
	b.WriteString("?exp=")
	b.WriteString(strconv.FormatInt(exp, 10))
	b.WriteString("&sig=")
	b.WriteString(sig)
	return b.String(), nil
}

// CanonicalObjectKey accepts only a relative key under videos/ with no traversal.
func CanonicalObjectKey(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "\\") || strings.Contains(raw, "%") || strings.ContainsAny(raw, "\x00?#\r\n") {
		return "", errDenied
	}
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return "", errDenied
	}
	parts := strings.Split(raw, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" || p == "." || p == ".." || !segmentOK(p) {
			return "", errDenied
		}
		out = append(out, p)
	}
	key := strings.Join(out, "/")
	if !strings.HasPrefix(key, "videos/") || key == "videos" {
		return "", errDenied
	}
	return key, nil
}

func segmentOK(s string) bool {
	if s == "" || len(s) > 240 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-' || r == '~':
		default:
			return false
		}
	}
	return true
}
