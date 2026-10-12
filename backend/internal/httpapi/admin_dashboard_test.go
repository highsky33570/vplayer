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
	"github.com/tycdn/vplayer/internal/store"
)

func testAdminDashboardRouter(users store.UserRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	api := &API{
		Cfg:   config.Config{AuthSecret: "test-auth-secret"},
		Users: users,
		// Store nil: dashboard returns zeroed stats (auth path still enforced).
	}
	r := gin.New()
	v1 := r.Group("/api/v1")
	{
		v1.GET("/admin/dashboard", api.RequireUser, api.RequireAdmin, api.AdminDashboard)
	}
	return r
}

func TestAdminDashboardUnauthenticated(t *testing.T) {
	r := testAdminDashboardRouter(store.NewMemoryUsers())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/dashboard", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 body=%s", w.Code, w.Body.String())
	}
}

func TestAdminDashboardForbiddenForNormalUser(t *testing.T) {
	users := store.NewMemoryUsers()
	u, err := users.CreateUser("user@example.com", "password1", "User")
	if err != nil {
		t.Fatal(err)
	}
	r := testAdminDashboardRouter(users)
	tok, err := auth.IssueToken("test-auth-secret", u.ID, u.Email, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%s", w.Code, w.Body.String())
	}
}

func TestAdminDashboardOKForAdmin(t *testing.T) {
	users := store.NewMemoryUsers()
	u, err := users.CreateUser("admin@example.com", "password1", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := users.SetIsAdmin(u.ID, true); err != nil {
		t.Fatal(err)
	}
	r := testAdminDashboardRouter(users)
	tok, err := auth.IssueToken("test-auth-secret", u.ID, u.Email, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			Status          string `json:"status"`
			UsersTotal      int    `json:"users_total"`
			VideosTotal     int    `json:"videos_total"`
			VideosActive    int    `json:"videos_active"`
			CategoriesTotal int    `json:"categories_total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Data.Status != "ok" {
		t.Fatalf("unexpected payload: %+v body=%s", resp, w.Body.String())
	}
}

func TestAdminDashboardOmitsSensitiveFields(t *testing.T) {
	users := store.NewMemoryUsers()
	u, err := users.CreateUser("admin2@example.com", "password1", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := users.SetIsAdmin(u.ID, true); err != nil {
		t.Fatal(err)
	}
	r := testAdminDashboardRouter(users)
	tok, err := auth.IssueToken("test-auth-secret", u.ID, u.Email, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	body := strings.ToLower(w.Body.String())
	forbidden := []string{
		"password", "password_hash", "auth_secret", "cdn_sign", "admin_api_token",
		"r2_secret", "r2_access", "mysql_dsn", "bearer ", "x-admin-token", "error_text",
	}
	for _, needle := range forbidden {
		if strings.Contains(body, needle) {
			t.Fatalf("dashboard response must not contain %q; body=%s", needle, w.Body.String())
		}
	}
}

func TestSyncRunJSONIncludesHasErrorOmitsErrorText(t *testing.T) {
	now := time.Now().UTC()
	finished := now.Add(time.Minute)
	out := syncRunJSON(&store.AdminSyncRunSummary{
		ID:           9,
		SourceSystem: "olehdtv",
		TriggerType:  "manual",
		StartedAt:    now,
		FinishedAt:   &finished,
		Scanned:      3,
		FailedCount:  1,
		HasError:     true,
	})
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, `"has_error":true`) {
		t.Fatalf("expected has_error=true in %s", body)
	}
	if strings.Contains(strings.ToLower(body), "error_text") {
		t.Fatalf("error_text must be absent from dashboard sync JSON: %s", body)
	}
	if strings.Contains(body, "dsn") || strings.Contains(body, "password") {
		t.Fatalf("unexpected sensitive content: %s", body)
	}
}
