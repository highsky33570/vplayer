package auth

import "strings"

// NormalizeEmail applies the same normalization used by user create/login:
// trim whitespace and lowercase.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
