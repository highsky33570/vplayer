package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/auth"
	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/play"
)

func testPlayRouter(authSecret, cdnSecret string) (*gin.Engine, *API) {
	gin.SetMode(gin.TestMode)
	api := &API{
		Cfg: config.Config{
			AuthSecret:    authSecret,
			CDNSignSecret: cdnSecret,
		},
		Resolver: &play.Resolver{Cfg: config.Config{CDNSignSecret: cdnSecret}},
	}
	r := gin.New()
	v1 := r.Group("/api/v1")
	{
		v1.GET("/categories", api.ListCategories)
		v1.GET("/videos", api.ListVideos)
		v1.GET("/videos/:id", api.GetVideo)
		v1.GET("/videos/:id/play", api.RequireUser, api.ResolvePlay)
		v1.POST("/videos/:id/play", api.RequireUser, api.CreatePlaySession)
	}
	return r, api
}

func TestPlayRequiresAuth_GET(t *testing.T) {
	r, _ := testPlayRouter("test-auth-secret", "test-cdn-secret")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos/1/play", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestPlayRequiresAuth_POST(t *testing.T) {
	r, _ := testPlayRouter("test-auth-secret", "test-cdn-secret")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/videos/1/play", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestPlayRejectsMalformedToken(t *testing.T) {
	r, _ := testPlayRouter("test-auth-secret", "test-cdn-secret")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos/1/play", nil)
	req.Header.Set("Authorization", "Bearer not-a-valid-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestPlayRejectsExpiredToken(t *testing.T) {
	const authSecret = "test-auth-secret"
	r, _ := testPlayRouter(authSecret, "test-cdn-secret")
	tok, err := auth.IssueToken(authSecret, 1, "u@example.com", -time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos/1/play", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestPlayRejectsWrongSecretToken(t *testing.T) {
	r, _ := testPlayRouter("test-auth-secret", "test-cdn-secret")
	tok, err := auth.IssueToken("other-secret", 1, "u@example.com", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos/1/play", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestAdminRouteStillRequiresUserWhenAuthNotRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := &API{Cfg: config.Config{AuthSecret: "test-auth-secret", AuthRequired: false}}
	r := gin.New()
	r.GET("/api/v1/admin/me", api.RequireUser, api.RequireAdmin, api.AdminMe)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestPlayGuestWhenAuthNotRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := &API{
		Cfg:      config.Config{AuthSecret: "test-auth-secret", AuthRequired: false},
		Resolver: &play.Resolver{Cfg: config.Config{CDNSignSecret: "test-cdn-secret"}},
	}
	r := gin.New()
	v1 := r.Group("/api/v1")
	v1.GET("/videos/:id/play", api.RequireUserWhenAuthRequired, api.ResolvePlay)
	v1.POST("/videos/:id/play", api.RequireUserWhenAuthRequired, api.CreatePlaySession)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req := httptest.NewRequest(method, "/api/v1/videos/1/play", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", method, w.Code, w.Body.String())
		}
	}
}

func TestPlayStillRequiresUserWhenAuthRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := &API{
		Cfg:      config.Config{AuthSecret: "test-auth-secret", AuthRequired: true},
		Resolver: &play.Resolver{Cfg: config.Config{CDNSignSecret: "test-cdn-secret"}},
	}
	r := gin.New()
	r.GET("/api/v1/videos/:id/play", api.RequireUserWhenAuthRequired, api.ResolvePlay)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos/1/play", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestPlayAllowsValidToken(t *testing.T) {
	const authSecret = "test-auth-secret"
	r, _ := testPlayRouter(authSecret, "test-cdn-secret")
	tok, err := auth.IssueToken(authSecret, 42, "u@example.com", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos/1/play", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["ok"] != true {
		t.Fatalf("body=%v", body)
	}
	data, _ := body["data"].(map[string]any)
	url, _ := data["url"].(string)
	if url == "" {
		t.Fatalf("expected playback url in data: %v", data)
	}
}

func TestPublicCatalogEndpointsUnauthenticated(t *testing.T) {
	r, _ := testPlayRouter("test-auth-secret", "test-cdn-secret")
	for _, path := range []string{"/api/v1/categories", "/api/v1/videos", "/api/v1/videos/1"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
}

func TestPublicVideoJSONHidesPlaybackURL(t *testing.T) {
	r, _ := testPlayRouter("test-auth-secret", "test-cdn-secret")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	raw := w.Body.String()
	if strings.Contains(raw, `"playback_url"`) {
		t.Fatalf("public video JSON must not expose playback_url: %s", raw)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	data, _ := body["data"].(map[string]any)
	if _, ok := data["playback_url"]; ok {
		t.Fatal("playback_url key present in data")
	}
	if _, ok := data["url"]; ok {
		t.Fatal("stream url key must not appear on video detail")
	}
}
