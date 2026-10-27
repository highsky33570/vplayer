package epusdt

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Sign builds the GMPay/EPUSDT HMAC-SHA256 signature (lowercase hex).
// Non-empty params are sorted by key (ASCII), joined as k=v with &, then HMAC'd.
func Sign(apiToken string, params map[string]string) string {
	filtered := make(map[string]string, len(params))
	for k, v := range params {
		if k == "signature" || strings.TrimSpace(v) == "" {
			continue
		}
		filtered[k] = v
	}
	keys := make([]string, 0, len(filtered))
	for k := range filtered {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+filtered[k])
	}
	canonical := strings.Join(parts, "&")
	mac := hmac.New(sha256.New, []byte(apiToken))
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks pid + HMAC signature against the merchant secret.
func VerifySignature(apiToken, pid string, params map[string]string) bool {
	sig := strings.TrimSpace(params["signature"])
	gotPID := strings.TrimSpace(params["pid"])
	if sig == "" || apiToken == "" || pid == "" {
		return false
	}
	if !hmac.Equal([]byte(gotPID), []byte(strings.TrimSpace(pid))) {
		return false
	}
	expect := Sign(apiToken, params)
	return hmac.Equal([]byte(strings.ToLower(sig)), []byte(expect))
}

// FormatAmount formats a USD amount with two decimal places for the gateway.
func FormatAmount(amount float64) string {
	return strconv.FormatFloat(amount, 'f', 2, 64)
}

// ParseFloatAmount parses a gateway numeric field.
func ParseFloatAmount(raw string) (float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty amount")
	}
	return strconv.ParseFloat(raw, 64)
}
