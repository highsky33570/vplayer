package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"regexp"
)

var reSixDigits = regexp.MustCompile(`^\d{6}$`)

// GenerateVerificationCode returns a cryptographically random 6-digit decimal code.
func GenerateVerificationCode() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	n := binary.BigEndian.Uint64(b[:]) % 1_000_000
	return fmt.Sprintf("%06d", n), nil
}

// HashVerificationCode returns hex(HMAC-SHA256(code, pepper)).
func HashVerificationCode(code, pepper string) string {
	mac := hmac.New(sha256.New, []byte(pepper))
	_, _ = mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyVerificationCode reports whether code matches the stored HMAC hex hash.
func VerifyVerificationCode(code, codeHash, pepper string) bool {
	if pepper == "" || codeHash == "" || !IsSixDigitCode(code) {
		return false
	}
	expected := HashVerificationCode(code, pepper)
	return hmac.Equal([]byte(expected), []byte(codeHash))
}

// IsSixDigitCode reports whether s is exactly six decimal digits.
func IsSixDigitCode(s string) bool {
	return reSixDigits.MatchString(s)
}
