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

type memAdminUsers struct {
	rows []store.AdminUser
}

func (m *memAdminUsers) ListAdminUsersFiltered(query string, isAdmin *bool, limit, offset int) ([]store.AdminUser, error) {
	filtered := m.filter(query, isAdmin)
	if offset > len(filtered) {
		return nil, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	out := make([]store.AdminUser, end-offset)
	copy(out, filtered[offset:end])
	return out, nil
}

func (m *memAdminUsers) CountAdminUsersFiltered(query string, isAdmin *bool) (int, error) {
	return len(m.filter(query, isAdmin)), nil
}

func (m *memAdminUsers) GetAdminUserByID(id uint64) (*store.AdminUser, error) {
	for i := range m.rows {
		if m.rows[i].ID == id {
			cp := m.rows[i]
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *memAdminUsers) filter(query string, isAdmin *bool) []store.AdminUser {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []store.AdminUser
	for _, u := range m.rows {
		if isAdmin != nil && u.IsAdmin != *isAdmin {
			continue
		}
		if q != "" {
			email := strings.ToLower(u.Email)
			nick := strings.ToLower(u.Nickname)
			if !strings.Contains(email, q) && !strings.Contains(nick, q) {
				continue
			}
		}
		out = append(out, u)
	}
	return out
}

func sampleAdminUsers() []store.AdminUser {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	return []store.AdminUser{
		{ID: 3, Email: "admin@example.com", Nickname: "Root", IsAdmin: true, CreatedAt: now},
		{ID: 2, Email: "bob@example.com", Nickname: "Bob", IsAdmin: false, CreatedAt: now.Add(-time.Hour)},
		{ID: 1, Email: "alice@example.com", Nickname: "Alice", IsAdmin: false, CreatedAt: now.Add(-2 * time.Hour)},
	}
}

func testAdminUsersRouter(users store.UserRepo, adminUsers adminUserRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	api := &API{
		Cfg:        config.Config{AuthSecret: "test-auth-secret"},
		Users:      users,
		AdminUsers: adminUsers,
	}
	r := gin.New()
	v1 := r.Group("/api/v1")
	{
		v1.GET("/admin/users", api.RequireUser, api.RequireAdmin, api.AdminListUsers)
		v1.GET("/admin/users/:id", api.RequireUser, api.RequireAdmin, api.AdminGetUser)
	}
	return r
}

func assertNoForbiddenUserFields(t *testing.T, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	forbidden := []string{
		"password_hash", "password", "auth_secret", "admin_api_token",
		"cdn_sign", "mysql_dsn", "r2_secret", "x-admin-token",
		"\"token\"", "dev-auth-secret", "should-not-leak",
	}
	for _, needle := range forbidden {
		if strings.Contains(lower, needle) {
			t.Fatalf("response must not contain %q; body=%s", needle, body)
		}
	}
}

func TestAdminListUsersUnauthenticated(t *testing.T) {
	r := testAdminUsersRouter(store.NewMemoryUsers(), &memAdminUsers{rows: sampleAdminUsers()})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", w.Code)
	}
}

func TestAdminListUsersForbidden(t *testing.T) {
	users := store.NewMemoryUsers()
	tok := userToken(t, users, "viewer@example.com")
	r := testAdminUsersRouter(users, &memAdminUsers{rows: sampleAdminUsers()})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403", w.Code)
	}
}

func TestAdminListUsersOKFiltersAndPagination(t *testing.T) {
	users := store.NewMemoryUsers()
	tok := adminToken(t, users, "ops@example.com")
	r := testAdminUsersRouter(users, &memAdminUsers{rows: sampleAdminUsers()})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?limit=2&offset=0", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		OK      bool           `json:"ok"`
		Data    []AdminUserDTO `json:"data"`
		Total   int            `json:"total"`
		Limit   int            `json:"limit"`
		Offset  int            `json:"offset"`
		HasMore bool           `json:"has_more"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Total != 3 || resp.Limit != 2 || resp.Offset != 0 || !resp.HasMore || len(resp.Data) != 2 {
		t.Fatalf("unexpected pagination payload: %+v", resp)
	}
	var sawAdmin, sawUser bool
	for _, u := range resp.Data {
		if u.IsAdmin {
			sawAdmin = true
		} else {
			sawUser = true
		}
	}
	// First page may be only admins depending on sample order; check full list below.
	_ = sawAdmin
	_ = sawUser
	assertNoForbiddenUserFields(t, w.Body.String())

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	sawAdmin, sawUser = false, false
	for _, u := range resp.Data {
		if u.IsAdmin {
			sawAdmin = true
		} else {
			sawUser = true
		}
	}
	if !sawAdmin || !sawUser {
		t.Fatal("admin and normal users must both be visible")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?q=alice", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 || len(resp.Data) != 1 || resp.Data[0].Email != "alice@example.com" {
		t.Fatalf("q filter unexpected: %+v", resp)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?is_admin=1", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 || len(resp.Data) != 1 || !resp.Data[0].IsAdmin {
		t.Fatalf("is_admin=1 unexpected: %+v", resp)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?is_admin=0", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 2 {
		t.Fatalf("is_admin=0 unexpected total=%d", resp.Total)
	}
	for _, u := range resp.Data {
		if u.IsAdmin {
			t.Fatalf("is_admin=0 returned admin: %+v", u)
		}
	}
}

func TestAdminListUsersInvalidFilters(t *testing.T) {
	users := store.NewMemoryUsers()
	tok := adminToken(t, users, "ops2@example.com")
	r := testAdminUsersRouter(users, &memAdminUsers{rows: sampleAdminUsers()})
	cases := []string{
		"/api/v1/admin/users?limit=abc",
		"/api/v1/admin/users?offset=-1",
		"/api/v1/admin/users?is_admin=maybe",
	}
	for _, path := range cases {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d want 400", path, w.Code)
		}
	}
}

func TestAdminListUsersClampsLimit(t *testing.T) {
	users := store.NewMemoryUsers()
	tok := adminToken(t, users, "ops3@example.com")
	r := testAdminUsersRouter(users, &memAdminUsers{rows: sampleAdminUsers()})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?limit=999", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp struct {
		Limit int `json:"limit"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if w.Code != http.StatusOK || resp.Limit != 100 {
		t.Fatalf("limit clamp failed status=%d limit=%d", w.Code, resp.Limit)
	}
}

func TestAdminGetUserAuthAndDetail(t *testing.T) {
	users := store.NewMemoryUsers()
	r := testAdminUsersRouter(users, &memAdminUsers{rows: sampleAdminUsers()})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d", w.Code)
	}

	tokUser := userToken(t, users, "u@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/1", nil)
	req.Header.Set("Authorization", "Bearer "+tokUser)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("user status=%d", w.Code)
	}

	tok := adminToken(t, users, "ops4@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/1", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		OK   bool         `json:"ok"`
		Data AdminUserDTO `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Data.ID != 1 || resp.Data.IsAdmin || resp.Data.Email != "alice@example.com" {
		t.Fatalf("unexpected normal user detail: %+v", resp.Data)
	}
	assertNoForbiddenUserFields(t, w.Body.String())

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/3", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || !resp.Data.IsAdmin {
		t.Fatalf("unexpected admin detail: status=%d %+v", w.Code, resp.Data)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/999", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/abc", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("malformed status=%d", w.Code)
	}
}

func TestAdminUserDTOOmitsSensitiveJSON(t *testing.T) {
	raw, err := json.Marshal(adminUserDTO(sampleAdminUsers()[0]))
	if err != nil {
		t.Fatal(err)
	}
	assertNoForbiddenUserFields(t, string(raw))
	if !strings.Contains(string(raw), `"is_admin":true`) {
		t.Fatalf("expected is_admin=true: %s", raw)
	}
}
