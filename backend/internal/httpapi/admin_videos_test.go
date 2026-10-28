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
	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/store"
)

type memAdminVideos struct {
	videos []model.Video
}

func (m *memAdminVideos) ListAdminVideosFiltered(categoryID uint64, query string, active, isHot *bool, limit, offset int) ([]model.Video, error) {
	filtered := m.filter(categoryID, query, active, isHot)
	if offset > len(filtered) {
		return nil, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	out := make([]model.Video, end-offset)
	copy(out, filtered[offset:end])
	return out, nil
}

func (m *memAdminVideos) CountAdminVideosFiltered(categoryID uint64, query string, active, isHot *bool) (int, error) {
	return len(m.filter(categoryID, query, active, isHot)), nil
}

func (m *memAdminVideos) GetVideo(id uint64) (*model.Video, error) {
	for i := range m.videos {
		if m.videos[i].ID == id {
			cp := m.videos[i]
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *memAdminVideos) filter(categoryID uint64, query string, active, isHot *bool) []model.Video {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []model.Video
	for _, v := range m.videos {
		if categoryID > 0 && v.CategoryID != categoryID {
			continue
		}
		if active != nil && v.IsActive != *active {
			continue
		}
		if isHot != nil && v.IsHot != *isHot {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(v.Title), q) {
			continue
		}
		out = append(out, v)
	}
	return out
}

func sampleAdminVideos() []model.Video {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	synced := now.Add(time.Hour)
	return []model.Video{
		{
			ID: 1, Title: "Alpha Movie", CategoryID: 2, Status: "ready", IsActive: true,
			SourceSystem: "olehdtv", SourceID: "100", Year: "2024", SyncStatus: "ok",
			CreatedAt: now, LastSyncedAt: &synced, ViewCount: 10, DurationSec: 100,
			Description: "desc", Director: "D", Actors: "A", Rating: 8.1,
			SourceCategoryID: "2", SourceUpdatedAt: &synced,
			PlaybackSource: "m3u8", PlaySID: 1, PlayNID: 1,
			PlaybackURL: "https://secret.example/play.m3u8", HLSMasterKey: "hls/key",
			CoverKey: "posters/a.jpg", CoverR2Key: "posters/a.jpg", PosterSourceURL: "https://origin/a.jpg",
		},
		{
			ID: 2, Title: "Beta Draft", CategoryID: 3, Status: "draft", IsActive: false,
			SourceSystem: "olehdtv", SourceID: "200", Year: "2023", SyncStatus: "ok",
			CreatedAt: now.Add(-time.Hour), ViewCount: 0, DurationSec: 50,
			PlaybackURL: "https://secret.example/beta.m3u8",
		},
		{
			ID: 3, Title: "Gamma Ready", CategoryID: 2, Status: "ready", IsActive: true,
			SourceSystem: "olehdtv", SourceID: "300", Year: "2022", SyncStatus: "ok",
			CreatedAt: now.Add(-2 * time.Hour), ViewCount: 3, DurationSec: 70,
		},
	}
}

func testAdminVideosRouter(users store.UserRepo, videos adminVideoRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	api := &API{
		Cfg:         config.Config{AuthSecret: "test-auth-secret"},
		Users:       users,
		AdminVideos: videos,
	}
	r := gin.New()
	v1 := r.Group("/api/v1")
	{
		v1.GET("/admin/videos", api.RequireUser, api.RequireAdmin, api.AdminListVideos)
		v1.GET("/admin/videos/:id", api.RequireUser, api.RequireAdmin, api.AdminGetVideo)
	}
	return r
}

func adminToken(t *testing.T, users *store.MemoryUsers, email string) string {
	t.Helper()
	u, err := users.CreateUser(email, "password1", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := users.SetIsAdmin(u.ID, true); err != nil {
		t.Fatal(err)
	}
	tok, err := auth.IssueToken("test-auth-secret", u.ID, u.Email, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func userToken(t *testing.T, users *store.MemoryUsers, email string) string {
	t.Helper()
	u, err := users.CreateUser(email, "password1", "User")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := auth.IssueToken("test-auth-secret", u.ID, u.Email, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func assertNoForbiddenVideoFields(t *testing.T, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	forbidden := []string{
		"playback_url", "hls_master_key", "cover_key", "cover_r2_key",
		"poster_source_url", "detail_url", "sync_error", "migration_error",
		"password", "auth_secret", "admin_api_token", "cdn_sign", "mysql_dsn",
		"r2_secret", "x-admin-token", "secret.example",
	}
	for _, needle := range forbidden {
		if strings.Contains(lower, needle) {
			t.Fatalf("response must not contain %q; body=%s", needle, body)
		}
	}
}

func TestAdminListVideosUnauthenticated(t *testing.T) {
	r := testAdminVideosRouter(store.NewMemoryUsers(), &memAdminVideos{videos: sampleAdminVideos()})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/videos", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", w.Code)
	}
}

func TestAdminListVideosForbidden(t *testing.T) {
	users := store.NewMemoryUsers()
	tok := userToken(t, users, "u@example.com")
	r := testAdminVideosRouter(users, &memAdminVideos{videos: sampleAdminVideos()})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/videos", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403", w.Code)
	}
}

func TestAdminListVideosOKPaginationAndFilters(t *testing.T) {
	users := store.NewMemoryUsers()
	tok := adminToken(t, users, "admin@example.com")
	r := testAdminVideosRouter(users, &memAdminVideos{videos: sampleAdminVideos()})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/videos?limit=2&offset=0", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		OK      bool                 `json:"ok"`
		Data    []AdminVideoListItem `json:"data"`
		Total   int                  `json:"total"`
		Limit   int                  `json:"limit"`
		Offset  int                  `json:"offset"`
		HasMore bool                 `json:"has_more"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Total != 3 || resp.Limit != 2 || resp.Offset != 0 || !resp.HasMore || len(resp.Data) != 2 {
		t.Fatalf("unexpected pagination payload: %+v", resp)
	}
	assertNoForbiddenVideoFields(t, w.Body.String())

	// active filter
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/videos?is_active=0", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 || len(resp.Data) != 1 || resp.Data[0].ID != 2 || resp.Data[0].IsActive {
		t.Fatalf("inactive filter failed: %+v", resp)
	}

	// category filter
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/videos?category_id=2", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 2 {
		t.Fatalf("category filter total=%d want 2", resp.Total)
	}

	// q filter (title)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/videos?q=Alpha", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 || resp.Data[0].Title != "Alpha Movie" {
		t.Fatalf("q filter failed: %+v", resp)
	}
}

func TestAdminListVideosInvalidFilters(t *testing.T) {
	users := store.NewMemoryUsers()
	tok := adminToken(t, users, "admin2@example.com")
	r := testAdminVideosRouter(users, &memAdminVideos{videos: sampleAdminVideos()})
	cases := []string{
		"/api/v1/admin/videos?limit=abc",
		"/api/v1/admin/videos?offset=-1",
		"/api/v1/admin/videos?category_id=0",
		"/api/v1/admin/videos?category_id=nope",
		"/api/v1/admin/videos?is_active=maybe",
	}
	for _, path := range cases {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d want 400 body=%s", path, w.Code, w.Body.String())
		}
	}
}

func TestAdminListVideosClampsLimit(t *testing.T) {
	users := store.NewMemoryUsers()
	tok := adminToken(t, users, "admin3@example.com")
	r := testAdminVideosRouter(users, &memAdminVideos{videos: sampleAdminVideos()})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/videos?limit=999", nil)
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

func TestAdminGetVideoAuthAndFields(t *testing.T) {
	users := store.NewMemoryUsers()
	r := testAdminVideosRouter(users, &memAdminVideos{videos: sampleAdminVideos()})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/videos/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d", w.Code)
	}

	tokUser := userToken(t, users, "viewer@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/videos/1", nil)
	req.Header.Set("Authorization", "Bearer "+tokUser)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("user status=%d", w.Code)
	}

	tok := adminToken(t, users, "admin4@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/videos/1", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		OK   bool             `json:"ok"`
		Data AdminVideoDetail `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Data.ID != 1 || resp.Data.Description == "" || resp.Data.PlaybackSource == "" {
		t.Fatalf("unexpected detail: %+v", resp.Data)
	}
	assertNoForbiddenVideoFields(t, w.Body.String())

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/videos/999", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/videos/abc", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("malformed status=%d", w.Code)
	}
}

func TestAdminVideoDTOOmitsSensitiveJSON(t *testing.T) {
	v := sampleAdminVideos()[0]
	v.CoverURL = "https://cdn.example/cover.jpg"
	raw, err := json.Marshal(adminVideoDetailFrom(v))
	if err != nil {
		t.Fatal(err)
	}
	assertNoForbiddenVideoFields(t, string(raw))
	if !strings.Contains(string(raw), `"cover_url"`) {
		t.Fatalf("expected cover_url in DTO: %s", raw)
	}
}
