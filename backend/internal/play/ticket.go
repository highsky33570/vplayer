package play

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SignURL builds a short-lived signed CDN path for m3u8 / covers.
func SignURL(cdnBase, secret, objectKey string, ttlSec int) string {
	exp := time.Now().Add(time.Duration(ttlSec) * time.Second).Unix()
	path := "/" + strings.TrimLeft(objectKey, "/")
	msg := fmt.Sprintf("%s%d", path, exp)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s%s?exp=%d&sig=%s", strings.TrimRight(cdnBase, "/"), path, exp, sig)
}

func NewTicket(videoID uint64, secret string, ttlSec int) string {
	exp := time.Now().Add(time.Duration(ttlSec) * time.Second).Unix()
	raw := fmt.Sprintf("%d:%d", videoID, exp)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(raw))
	return base64.RawURLEncoding.EncodeToString([]byte(raw)) + "." + hex.EncodeToString(mac.Sum(nil))
}

func ParseTicket(ticket, secret string) (videoID uint64, ok bool) {
	parts := strings.Split(ticket, ".")
	if len(parts) != 2 {
		return 0, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(raw)
	if hex.EncodeToString(mac.Sum(nil)) != parts[1] {
		return 0, false
	}
	fields := strings.Split(string(raw), ":")
	if len(fields) != 2 {
		return 0, false
	}
	exp, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return 0, false
	}
	id, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

func CoverPlaceholder(title string) string {
	return "https://picsum.photos/seed/" + url.PathEscape(title) + "/640/360"
}
