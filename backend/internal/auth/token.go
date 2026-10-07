package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("expired token")
)

type Claims struct {
	UserID uint64 `json:"uid"`
	Email  string `json:"email"`
	Exp    int64  `json:"exp"`
}

// IssueToken creates a compact HMAC-signed session token.
func IssueToken(secret string, userID uint64, email string, ttl time.Duration) (string, error) {
	if secret == "" {
		return "", errors.New("empty auth secret")
	}
	claims := Claims{
		UserID: userID,
		Email:  email,
		Exp:    time.Now().Add(ttl).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(body))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return body + "." + sig, nil
}

func ParseToken(secret, token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, ErrInvalidToken
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return nil, ErrInvalidToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var claims Claims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, ErrInvalidToken
	}
	if claims.UserID == 0 || claims.Exp < time.Now().Unix() {
		return nil, ErrExpiredToken
	}
	return &claims, nil
}

func BearerToken(header string) string {
	h := strings.TrimSpace(header)
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func FormatUserID(id uint64) string {
	return strconv.FormatUint(id, 10)
}

func DisplayName(nickname, email string) string {
	n := strings.TrimSpace(nickname)
	if n != "" {
		return n
	}
	if i := strings.IndexByte(email, '@'); i > 0 {
		return email[:i]
	}
	if email != "" {
		return email
	}
	return fmt.Sprintf("用户")
}
