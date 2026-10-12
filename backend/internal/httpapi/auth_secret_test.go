package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tycdn/vplayer/internal/auth"
	"github.com/tycdn/vplayer/internal/config"
)

func TestAuthSecretUsesAuthSecretNotCDN(t *testing.T) {
	api := &API{Cfg: config.Config{AuthSecret: "auth-only", CDNSignSecret: "cdn-only"}}
	if got := api.authSecret(); got != "auth-only" {
		t.Fatalf("authSecret()=%q want auth-only", got)
	}
}

func TestAuthSecretDefaultWhenEmpty(t *testing.T) {
	api := &API{Cfg: config.Config{AuthSecret: "", CDNSignSecret: "cdn-only"}}
	if got := api.authSecret(); got != "dev-auth-secret" {
		t.Fatalf("authSecret()=%q want dev-auth-secret", got)
	}
}

func TestPlayRejectsTokenSignedWithCDNSecretOnly(t *testing.T) {
	const authSecret = "user-auth-key"
	const cdnSecret = "cdn-sign-key"
	r, _ := testPlayRouter(authSecret, cdnSecret)
	tok, err := auth.IssueToken(cdnSecret, 1, "u@example.com", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos/1/play", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("CDN_SIGN_SECRET must not validate auth token: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestPlayAcceptsTokenSignedWithAuthSecret(t *testing.T) {
	const authSecret = "user-auth-key"
	const cdnSecret = "cdn-sign-key"
	r, _ := testPlayRouter(authSecret, cdnSecret)
	tok, err := auth.IssueToken(authSecret, 42, "u@example.com", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos/1/play", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("AUTH_SECRET token should allow play: status=%d body=%s", w.Code, w.Body.String())
	}
}
