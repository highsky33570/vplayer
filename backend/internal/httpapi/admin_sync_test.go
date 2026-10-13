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

const (
	syncSensitiveError   = "SENSITIVE_SYNC_ERROR_MARKER mysql://user:pass@host/db https://cdn.example/secret?token=abc"
	syncSensitiveSummary = `{"error":"SENSITIVE_SUMMARY_MARKER","dsn":"mysql://x","actions":[{"detail":"https://evil.test/path"}]}`
)

type memAdminSync struct {
	runs                []store.AdminSyncRunSummary
	pendingEnrichment   int
	videosSyncError     int
	categoriesSyncError int
	videosActive        int
	videosInactive      int
}

func (m *memAdminSync) AdminSyncOverviewStats() (*store.AdminSyncOverview, error) {
	out := &store.AdminSyncOverview{
		PendingEnrichmentCount:   m.pendingEnrichment,
		VideosSyncErrorCount:     m.videosSyncError,
		CategoriesSyncErrorCount: m.categoriesSyncError,
		VideosActive:             m.videosActive,
		VideosInactive:           m.videosInactive,
	}
	if len(m.runs) > 0 {
		cp := m.runs[0]
		out.LatestSync = &cp
	}
	for i := range m.runs {
		if m.runs[i].TriggerType == "incremental" {
			cp := m.runs[i]
			out.LatestIncremental = &cp
			break
		}
	}
	for i := range m.runs {
		if m.runs[i].TriggerType != "incremental" {
			cp := m.runs[i]
			out.LatestFull = &cp
			break
		}
	}
	return out, nil
}

func (m *memAdminSync) ListAdminSyncRunsFiltered(triggerType string, limit, offset int) ([]store.AdminSyncRunSummary, error) {
	filtered := m.filter(triggerType)
	if offset > len(filtered) {
		return nil, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	out := make([]store.AdminSyncRunSummary, end-offset)
	copy(out, filtered[offset:end])
	return out, nil
}

func (m *memAdminSync) CountAdminSyncRunsFiltered(triggerType string) (int, error) {
	return len(m.filter(triggerType)), nil
}

func (m *memAdminSync) GetAdminSyncRunByID(id uint64) (*store.AdminSyncRunSummary, error) {
	for i := range m.runs {
		if m.runs[i].ID == id {
			cp := m.runs[i]
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *memAdminSync) filter(triggerType string) []store.AdminSyncRunSummary {
	t := strings.TrimSpace(triggerType)
	var out []store.AdminSyncRunSummary
	for _, r := range m.runs {
		if t != "" && r.TriggerType != t {
			continue
		}
		out = append(out, r)
	}
	return out
}

func sampleAdminSyncRuns() []store.AdminSyncRunSummary {
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	fin := now
	return []store.AdminSyncRunSummary{
		{
			ID: 3, SourceSystem: "olehdtv", TriggerType: "incremental",
			StartedAt: now, FinishedAt: &fin,
			Scanned: 5, CreatedCount: 1, UpdatedCount: 2, UnchangedCount: 2,
			FailedCount: 0, HasError: false,
		},
		{
			ID: 2, SourceSystem: "olehdtv", TriggerType: "manual",
			StartedAt: now.Add(-time.Hour), FinishedAt: &fin,
			Scanned: 100, CreatedCount: 10, UpdatedCount: 20, UnchangedCount: 60,
			FailedCount: 3, HasError: true,
		},
		{
			ID: 1, SourceSystem: "olehdtv", TriggerType: "schedule",
			StartedAt: now.Add(-2 * time.Hour), FinishedAt: &fin,
			Scanned: 50, CreatedCount: 0, UpdatedCount: 5, UnchangedCount: 45,
			FailedCount: 0, HasError: false,
		},
	}
}

func testAdminSyncRouter(users store.UserRepo, sync adminSyncRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	api := &API{
		Cfg:       config.Config{AuthSecret: "test-auth-secret"},
		Users:     users,
		AdminSync: sync,
	}
	r := gin.New()
	v1 := r.Group("/api/v1")
	{
		v1.GET("/admin/sync", api.RequireUser, api.RequireAdmin, api.AdminSyncOverview)
		v1.GET("/admin/sync/runs", api.RequireUser, api.RequireAdmin, api.AdminListSyncRuns)
		v1.GET("/admin/sync/runs/:id", api.RequireUser, api.RequireAdmin, api.AdminGetSyncRun)
	}
	return r
}

func assertNoForbiddenSyncFields(t *testing.T, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	// Prefer JSON key / exact markers so count field names like videos_sync_error_count
	// are not false positives for sync_error.
	forbidden := []string{
		`"error_text"`, `"summary_json"`, `"sync_error"`, `"migration_error"`,
		"password", "auth_secret", "admin_api_token", "cdn_sign",
		"mysql_dsn", "r2_secret", "x-admin-token",
		"sensitive_sync_error_marker", "sensitive_summary_marker",
		"mysql://user:pass", "https://cdn.example/secret", "https://evil.test",
		"dev-auth-secret", "should-not-leak",
	}
	for _, needle := range forbidden {
		if strings.Contains(lower, needle) {
			t.Fatalf("response must not contain %q; body=%s", needle, body)
		}
	}
}

func TestAdminSyncOverviewAuth(t *testing.T) {
	users := store.NewMemoryUsers()
	mem := &memAdminSync{runs: sampleAdminSyncRuns(), pendingEnrichment: 7, videosActive: 9, videosInactive: 2}
	r := testAdminSyncRouter(users, mem)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d", w.Code)
	}

	tokUser := userToken(t, users, "sync-user@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync", nil)
	req.Header.Set("Authorization", "Bearer "+tokUser)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("user status=%d", w.Code)
	}

	tok := adminToken(t, users, "sync-admin@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			LatestSync          map[string]any `json:"latest_sync"`
			LatestIncremental   map[string]any `json:"latest_incremental"`
			LatestFull          map[string]any `json:"latest_full"`
			PendingEnrichment   int            `json:"pending_enrichment_count"`
			VideosSyncError     int            `json:"videos_sync_error_count"`
			CategoriesSyncError int            `json:"categories_sync_error_count"`
			VideosActive        int            `json:"videos_active"`
			VideosInactive      int            `json:"videos_inactive"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Data.PendingEnrichment != 7 || resp.Data.VideosActive != 9 || resp.Data.VideosInactive != 2 {
		t.Fatalf("unexpected overview: %+v", resp.Data)
	}
	if resp.Data.LatestSync == nil || resp.Data.LatestIncremental == nil || resp.Data.LatestFull == nil {
		t.Fatalf("expected latest_* populated: %+v", resp.Data)
	}
	if resp.Data.LatestIncremental["trigger_type"] != "incremental" {
		t.Fatalf("latest_incremental wrong: %+v", resp.Data.LatestIncremental)
	}
	if resp.Data.LatestFull["trigger_type"] == "incremental" {
		t.Fatalf("latest_full must not be incremental: %+v", resp.Data.LatestFull)
	}
	assertNoForbiddenSyncFields(t, w.Body.String())
}

func TestAdminSyncOverviewOmitsSensitiveMarkers(t *testing.T) {
	// DTO path never carries error_text/summary_json; markers document what must stay out.
	if syncSensitiveError == "" || syncSensitiveSummary == "" {
		t.Fatal("expected sensitive marker constants")
	}
	users := store.NewMemoryUsers()
	tok := adminToken(t, users, "sync-safe@example.com")
	r := testAdminSyncRouter(users, &memAdminSync{runs: sampleAdminSyncRuns()})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	body := w.Body.String()
	assertNoForbiddenSyncFields(t, body)
	if strings.Contains(body, "SENSITIVE_") || strings.Contains(body, syncSensitiveError) || strings.Contains(body, syncSensitiveSummary) {
		t.Fatal("sensitive markers leaked")
	}
}

func TestAdminListSyncRunsAuthFiltersPagination(t *testing.T) {
	users := store.NewMemoryUsers()
	mem := &memAdminSync{runs: sampleAdminSyncRuns()}
	r := testAdminSyncRouter(users, mem)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/runs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth=%d", w.Code)
	}

	tokUser := userToken(t, users, "sync-viewer@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/runs", nil)
	req.Header.Set("Authorization", "Bearer "+tokUser)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("user=%d", w.Code)
	}

	tok := adminToken(t, users, "sync-ops@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/runs?limit=2&offset=0", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		OK      bool             `json:"ok"`
		Data    []map[string]any `json:"data"`
		Total   int              `json:"total"`
		Limit   int              `json:"limit"`
		Offset  int              `json:"offset"`
		HasMore bool             `json:"has_more"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Total != 3 || resp.Limit != 2 || !resp.HasMore || len(resp.Data) != 2 {
		t.Fatalf("pagination unexpected: %+v", resp)
	}
	if resp.Data[0]["id"] != float64(3) {
		t.Fatalf("expected newest-first id=3, got %+v", resp.Data[0])
	}
	assertNoForbiddenSyncFields(t, w.Body.String())

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/runs?trigger_type=incremental", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 || len(resp.Data) != 1 || resp.Data[0]["trigger_type"] != "incremental" {
		t.Fatalf("trigger filter unexpected: %+v", resp)
	}
}

func TestAdminListSyncRunsInvalidFilters(t *testing.T) {
	users := store.NewMemoryUsers()
	tok := adminToken(t, users, "sync-bad@example.com")
	r := testAdminSyncRouter(users, &memAdminSync{runs: sampleAdminSyncRuns()})
	for _, path := range []string{
		"/api/v1/admin/sync/runs?limit=abc",
		"/api/v1/admin/sync/runs?offset=-1",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d", path, w.Code)
		}
	}
}

func TestAdminGetSyncRunAuthAndDetail(t *testing.T) {
	users := store.NewMemoryUsers()
	r := testAdminSyncRouter(users, &memAdminSync{runs: sampleAdminSyncRuns()})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/runs/2", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth=%d", w.Code)
	}

	tokUser := userToken(t, users, "sync-d@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/runs/2", nil)
	req.Header.Set("Authorization", "Bearer "+tokUser)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("user=%d", w.Code)
	}

	tok := adminToken(t, users, "sync-detail@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/runs/2", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Data["id"] != float64(2) || resp.Data["has_error"] != true {
		t.Fatalf("unexpected detail: %+v", resp.Data)
	}
	assertNoForbiddenSyncFields(t, w.Body.String())

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/runs/999", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing=%d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/runs/abc", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("malformed=%d", w.Code)
	}
}

func TestSyncRunJSONNeverIncludesErrorTextOrSummary(t *testing.T) {
	fin := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	raw, err := json.Marshal(syncRunJSON(&store.AdminSyncRunSummary{
		ID: 9, SourceSystem: "olehdtv", TriggerType: "manual",
		StartedAt: fin, FinishedAt: &fin, HasError: true, FailedCount: 1,
	}))
	if err != nil {
		t.Fatal(err)
	}
	assertNoForbiddenSyncFields(t, string(raw))
	if !strings.Contains(string(raw), `"has_error":true`) {
		t.Fatalf("expected has_error: %s", raw)
	}
}
