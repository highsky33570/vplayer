package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/auth"
	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/store"
)

func testAuthAdminRouter(users store.UserRepo) (*gin.Engine, *API) {
	gin.SetMode(gin.TestMode)
	api := &API{
		Cfg:   config.Config{AuthSecret: "test-auth-secret"},
		Users: users,
	}
	r := gin.New()
	v1 := r.Group("/api/v1")
	{
		v1.POST("/auth/register", api.Register)
		v1.POST("/auth/login", api.Login)
		v1.GET("/auth/me", api.Me)
		v1.GET("/admin/me", api.RequireUser, api.RequireAdmin, api.AdminMe)
	}
	return r, api
}

func TestAdminMeUnauthenticated(t *testing.T) {
	r, _ := testAuthAdminRouter(store.NewMemoryUsers())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 body=%s", w.Code, w.Body.String())
	}
}

func TestAdminMeForbiddenForNormalUser(t *testing.T) {
	users := store.NewMemoryUsers()
	u, err := users.CreateUser("user@example.com", "password1", "User")
	if err != nil {
		t.Fatal(err)
	}
	r, api := testAuthAdminRouter(users)
	tok, err := auth.IssueToken(api.authSecret(), u.ID, u.Email, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/me", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%s", w.Code, w.Body.String())
	}
}

func TestAdminMeOKForAdmin(t *testing.T) {
	users := store.NewMemoryUsers()
	u, err := users.CreateUser("admin@example.com", "password1", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := users.SetIsAdmin(u.ID, true); err != nil {
		t.Fatal(err)
	}
	r, api := testAuthAdminRouter(users)
	tok, err := auth.IssueToken(api.authSecret(), u.ID, u.Email, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/me", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			ID      uint64 `json:"id"`
			Email   string `json:"email"`
			IsAdmin bool   `json:"is_admin"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || !resp.Data.IsAdmin || resp.Data.Email != "admin@example.com" {
		t.Fatalf("unexpected admin/me payload: %+v", resp)
	}
}

func TestRegisterCannotSelfPromoteToAdmin(t *testing.T) {
	users := store.NewMemoryUsers()
	r, api := testAuthAdminRouter(users)
	// Legacy register must not create accounts (verification required).
	body := []byte(`{"email":"newbie@example.com","password":"password1","nickname":"N","is_admin":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == http.StatusOK {
		t.Fatalf("legacy register must not succeed: %s", w.Body.String())
	}
	u, err := users.CreateUser("newbie@example.com", "password1", "N")
	if err != nil {
		t.Fatal(err)
	}
	if u.IsAdmin {
		t.Fatal("CreateUser must not grant is_admin")
	}
	tok, err := auth.IssueToken(api.authSecret(), u.ID, u.Email, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	reqMe := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	reqMe.Header.Set("Authorization", "Bearer "+tok)
	wMe := httptest.NewRecorder()
	r.ServeHTTP(wMe, reqMe)
	if wMe.Code != http.StatusOK {
		t.Fatalf("me status=%d body=%s", wMe.Code, wMe.Body.String())
	}
	var me struct {
		Data struct {
			IsAdmin bool `json:"is_admin"`
		} `json:"data"`
	}
	if err := json.Unmarshal(wMe.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if me.Data.IsAdmin {
		t.Fatal("/auth/me must report is_admin=false")
	}
	reqAdmin := httptest.NewRequest(http.MethodGet, "/api/v1/admin/me", nil)
	reqAdmin.Header.Set("Authorization", "Bearer "+tok)
	wAdmin := httptest.NewRecorder()
	r.ServeHTTP(wAdmin, reqAdmin)
	if wAdmin.Code != http.StatusForbidden {
		t.Fatalf("admin/me status=%d want 403", wAdmin.Code)
	}
}

func TestLoginAndMeExposeIsAdmin(t *testing.T) {
	users := store.NewMemoryUsers()
	u, err := users.CreateUser("flag@example.com", "password1", "Flag")
	if err != nil {
		t.Fatal(err)
	}
	r, _ := testAuthAdminRouter(users)

	loginBody := []byte(`{"email":"flag@example.com","password":"password1"}`)
	reqLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	reqLogin.Header.Set("Content-Type", "application/json")
	wLogin := httptest.NewRecorder()
	r.ServeHTTP(wLogin, reqLogin)
	if wLogin.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", wLogin.Code, wLogin.Body.String())
	}
	var loginResp struct {
		Data struct {
			Token string `json:"token"`
			User  struct {
				IsAdmin bool `json:"is_admin"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(wLogin.Body.Bytes(), &loginResp); err != nil {
		t.Fatal(err)
	}
	if loginResp.Data.User.IsAdmin {
		t.Fatal("login must expose is_admin=false for normal user")
	}

	reqMe := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	reqMe.Header.Set("Authorization", "Bearer "+loginResp.Data.Token)
	wMe := httptest.NewRecorder()
	r.ServeHTTP(wMe, reqMe)
	if wMe.Code != http.StatusOK {
		t.Fatalf("me status=%d", wMe.Code)
	}
	var me struct {
		Data struct {
			IsAdmin bool `json:"is_admin"`
		} `json:"data"`
	}
	if err := json.Unmarshal(wMe.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if me.Data.IsAdmin {
		t.Fatal("me must expose is_admin=false for normal user")
	}

	if err := users.SetIsAdmin(u.ID, true); err != nil {
		t.Fatal(err)
	}
	reqLogin2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	reqLogin2.Header.Set("Content-Type", "application/json")
	wLogin2 := httptest.NewRecorder()
	r.ServeHTTP(wLogin2, reqLogin2)
	if err := json.Unmarshal(wLogin2.Body.Bytes(), &loginResp); err != nil {
		t.Fatal(err)
	}
	if !loginResp.Data.User.IsAdmin {
		t.Fatal("login must expose is_admin=true after promotion")
	}
	reqMe2 := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	reqMe2.Header.Set("Authorization", "Bearer "+loginResp.Data.Token)
	wMe2 := httptest.NewRecorder()
	r.ServeHTTP(wMe2, reqMe2)
	if err := json.Unmarshal(wMe2.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if !me.Data.IsAdmin {
		t.Fatal("me must expose is_admin=true after promotion")
	}
}

func TestRequireAdminReChecksDatabaseNotClientClaim(t *testing.T) {
	users := store.NewMemoryUsers()
	u, err := users.CreateUser("check@example.com", "password1", "X")
	if err != nil {
		t.Fatal(err)
	}
	if err := users.SetIsAdmin(u.ID, true); err != nil {
		t.Fatal(err)
	}
	r, api := testAuthAdminRouter(users)
	tok, err := auth.IssueToken(api.authSecret(), u.ID, u.Email, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// Demote in DB after token issued — RequireAdmin must re-read DB.
	if err := users.SetIsAdmin(u.ID, false); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/me", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("demoted user must get 403, got %d body=%s", w.Code, w.Body.String())
	}
}
