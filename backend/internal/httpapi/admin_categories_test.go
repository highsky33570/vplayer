package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/store"
)

type memAdminCategories struct {
	rows []store.AdminCategory
}

func (m *memAdminCategories) ListAdminCategories() ([]store.AdminCategory, error) {
	out := make([]store.AdminCategory, len(m.rows))
	copy(out, m.rows)
	return out, nil
}

func (m *memAdminCategories) GetAdminCategoryByID(id uint64) (*store.AdminCategory, error) {
	for i := range m.rows {
		if m.rows[i].ID == id {
			cp := m.rows[i]
			return &cp, nil
		}
	}
	return nil, nil
}

func sampleAdminCategories() []store.AdminCategory {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	synced := now.Add(-time.Hour)
	return []store.AdminCategory{
		{
			ID: 1, Name: "电影", Slug: "movie", Sort: 10, IsActive: true,
			SourceSystem: "olehdtv", SourceID: "1", SyncStatus: "ok",
			LastSyncedAt: &synced, CreatedAt: now, VideoCount: 5,
		},
		{
			ID: 2, Name: "已合并", Slug: "old-movie", Sort: 99, IsActive: false,
			SourceSystem: "", SourceID: "", SyncStatus: "merged",
			HasError: true, CreatedAt: now.Add(-time.Hour), VideoCount: 0,
		},
		{
			ID: 3, Name: "本地分类", Slug: "local", Sort: 50, IsActive: true,
			CreatedAt: now.Add(-2 * time.Hour), VideoCount: 2, SyncStatus: "ok",
		},
	}
}

func testAdminCategoriesRouter(users store.UserRepo, cats adminCategoryRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	api := &API{
		Cfg:             config.Config{AuthSecret: "test-auth-secret"},
		Users:           users,
		AdminCategories: cats,
	}
	r := gin.New()
	v1 := r.Group("/api/v1")
	{
		v1.GET("/admin/categories", api.RequireUser, api.RequireAdmin, api.AdminListCategories)
		v1.GET("/admin/categories/:id", api.RequireUser, api.RequireAdmin, api.AdminGetCategory)
	}
	return r
}

func assertNoForbiddenCategoryFields(t *testing.T, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	forbidden := []string{
		"sync_error", "password", "auth_secret", "admin_api_token",
		"cdn_sign", "mysql_dsn", "r2_secret", "x-admin-token",
		"should-not-leak",
	}
	for _, needle := range forbidden {
		if strings.Contains(lower, needle) {
			t.Fatalf("response must not contain %q; body=%s", needle, body)
		}
	}
}

func TestAdminListCategoriesUnauthenticated(t *testing.T) {
	r := testAdminCategoriesRouter(store.NewMemoryUsers(), &memAdminCategories{rows: sampleAdminCategories()})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/categories", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", w.Code)
	}
}

func TestAdminListCategoriesForbidden(t *testing.T) {
	users := store.NewMemoryUsers()
	tok := userToken(t, users, "cat-user@example.com")
	r := testAdminCategoriesRouter(users, &memAdminCategories{rows: sampleAdminCategories()})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/categories", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403", w.Code)
	}
}

func TestAdminListCategoriesOK(t *testing.T) {
	users := store.NewMemoryUsers()
	tok := adminToken(t, users, "cat-admin@example.com")
	r := testAdminCategoriesRouter(users, &memAdminCategories{rows: sampleAdminCategories()})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/categories", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		OK   bool               `json:"ok"`
		Data []AdminCategoryDTO `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || len(resp.Data) != 3 {
		t.Fatalf("unexpected payload: %+v", resp)
	}
	var sawInactive, sawHasError, sawCount bool
	for _, c := range resp.Data {
		if !c.IsActive {
			sawInactive = true
		}
		if c.HasError {
			sawHasError = true
		}
		if c.ID == 1 && c.VideoCount == 5 {
			sawCount = true
		}
	}
	if !sawInactive {
		t.Fatal("inactive categories must be visible")
	}
	if !sawHasError {
		t.Fatal("has_error must be exposed when sync had an error")
	}
	if !sawCount {
		t.Fatal("video_count must be correct")
	}
	assertNoForbiddenCategoryFields(t, w.Body.String())
}

func TestAdminGetCategoryAuthAndDetail(t *testing.T) {
	users := store.NewMemoryUsers()
	r := testAdminCategoriesRouter(users, &memAdminCategories{rows: sampleAdminCategories()})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/categories/2", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d", w.Code)
	}

	tokUser := userToken(t, users, "cat-viewer@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/categories/2", nil)
	req.Header.Set("Authorization", "Bearer "+tokUser)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("user status=%d", w.Code)
	}

	tok := adminToken(t, users, "cat-admin2@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/categories/2", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		OK   bool             `json:"ok"`
		Data AdminCategoryDTO `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Data.ID != 2 || resp.Data.IsActive || !resp.Data.HasError {
		t.Fatalf("unexpected detail: %+v", resp.Data)
	}
	assertNoForbiddenCategoryFields(t, w.Body.String())

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/categories/999", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/categories/abc", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("malformed status=%d", w.Code)
	}
}

func TestAdminCategoryDTOOmitsSyncErrorJSON(t *testing.T) {
	raw, err := json.Marshal(adminCategoryDTO(sampleAdminCategories()[1]))
	if err != nil {
		t.Fatal(err)
	}
	assertNoForbiddenCategoryFields(t, string(raw))
	if !strings.Contains(string(raw), `"has_error":true`) {
		t.Fatalf("expected has_error=true: %s", raw)
	}
}
