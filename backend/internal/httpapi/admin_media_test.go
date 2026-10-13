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
	mediaSensitivePlayback = "https://SENSITIVE_PLAYBACK.example/stream.m3u8?token=SECRET"
	mediaSensitivePlayPage = "https://SENSITIVE_PLAYPAGE.example/play/1"
	mediaSensitiveHLSKey   = "videos/1/2/SENSITIVE_HLS_OBJECT_KEY.m3u8"
	mediaSensitiveMigErr   = "SENSITIVE_MIGRATION_ERROR mysql://user:pass@host/db"
	mediaSensitiveSyncErr  = "SENSITIVE_SYNC_ERROR https://evil.test/x"
)

type memAdminMedia struct {
	overview store.AdminMediaOverview
	rows     []store.AdminMediaEpisode
}

func (m *memAdminMedia) AdminMediaOverviewStats() (*store.AdminMediaOverview, error) {
	cp := m.overview
	return &cp, nil
}

func (m *memAdminMedia) ListAdminMediaEpisodesFiltered(migrationStatus, storageProvider, query string, limit, offset int) ([]store.AdminMediaEpisode, error) {
	filtered := m.filter(migrationStatus, storageProvider, query)
	if offset > len(filtered) {
		return nil, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	out := make([]store.AdminMediaEpisode, end-offset)
	copy(out, filtered[offset:end])
	return out, nil
}

func (m *memAdminMedia) CountAdminMediaEpisodesFiltered(migrationStatus, storageProvider, query string) (int, error) {
	return len(m.filter(migrationStatus, storageProvider, query)), nil
}

func (m *memAdminMedia) GetAdminMediaEpisodeByID(id uint64) (*store.AdminMediaEpisode, error) {
	for i := range m.rows {
		if m.rows[i].EpisodeID == id {
			cp := m.rows[i]
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *memAdminMedia) filter(migrationStatus, storageProvider, query string) []store.AdminMediaEpisode {
	ms := strings.ToLower(strings.TrimSpace(migrationStatus))
	sp := strings.ToLower(strings.TrimSpace(storageProvider))
	q := strings.ToLower(strings.TrimSpace(query))
	var out []store.AdminMediaEpisode
	for _, ep := range m.rows {
		st := strings.ToLower(ep.MigrationStatus)
		switch ms {
		case "done":
			if st != "done" && st != "migrated" {
				continue
			}
		case "failed":
			if st != "failed" {
				continue
			}
		case "pending":
			if st == "done" || st == "migrated" || st == "failed" {
				continue
			}
		}
		if sp != "" && strings.ToLower(ep.StorageProvider) != sp {
			continue
		}
		if q != "" {
			vt := strings.ToLower(ep.VideoTitle)
			et := strings.ToLower(ep.EpisodeTitle)
			if !strings.Contains(vt, q) && !strings.Contains(et, q) {
				continue
			}
		}
		out = append(out, ep)
	}
	return out
}

func sampleAdminMedia() *memAdminMedia {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	migrated := now.Add(-time.Hour)
	return &memAdminMedia{
		overview: store.AdminMediaOverview{
			EpisodesTotal: 4, MigratedCount: 1, FailedCount: 1, NotMigratedCount: 2,
			EligibleCount: 2, StorageR2Count: 1, LatestMigratedAt: &migrated,
		},
		rows: []store.AdminMediaEpisode{
			{
				EpisodeID: 4, VideoID: 10, VideoTitle: "Alpha Movie", SID: 1, NID: 2,
				EpisodeTitle: "EP2", StorageProvider: "", MigrationStatus: "",
				HasError: false, PlaybackStatus: "ok", HasHLSObject: false,
			},
			{
				EpisodeID: 3, VideoID: 10, VideoTitle: "Alpha Movie", SID: 1, NID: 1,
				EpisodeTitle: "EP1", StorageProvider: "r2", MigrationStatus: "done",
				MigratedAt: &migrated, HasError: false, PlaybackStatus: "ok", HasHLSObject: true,
			},
			{
				EpisodeID: 2, VideoID: 11, VideoTitle: "Beta Show", SID: 1, NID: 1,
				EpisodeTitle: "Pilot", StorageProvider: "", MigrationStatus: "failed",
				HasError: true, PlaybackStatus: "ok", HasHLSObject: false,
			},
			{
				EpisodeID: 1, VideoID: 12, VideoTitle: "Gamma Film", SID: 1, NID: 1,
				EpisodeTitle: "Part1", StorageProvider: "", MigrationStatus: "",
				HasError: false, PlaybackStatus: "pending_enrichment", HasHLSObject: false,
			},
		},
	}
}

func testAdminMediaRouter(users store.UserRepo, media adminMediaRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	api := &API{
		Cfg:        config.Config{AuthSecret: "test-auth-secret"},
		Users:      users,
		AdminMedia: media,
	}
	r := gin.New()
	v1 := r.Group("/api/v1")
	{
		v1.GET("/admin/media", api.RequireUser, api.RequireAdmin, api.AdminMediaOverview)
		v1.GET("/admin/media/episodes", api.RequireUser, api.RequireAdmin, api.AdminListMediaEpisodes)
		v1.GET("/admin/media/episodes/:id", api.RequireUser, api.RequireAdmin, api.AdminGetMediaEpisode)
	}
	return r
}

func assertNoForbiddenMediaFields(t *testing.T, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	forbidden := []string{
		`"playback_url"`, `"play_page_url"`, `"hls_master_key"`, `"hls_object_key"`,
		`"cover_key"`, `"cover_r2_key"`, `"poster_source_url"`, `"detail_url"`,
		`"migration_error"`, `"sync_error"`,
		"sensitive_playback", "sensitive_playpage", "sensitive_hls_object_key",
		"sensitive_migration_error", "sensitive_sync_error",
		"password", "auth_secret", "admin_api_token", "cdn_sign",
		"mysql_dsn", "r2_secret", "x-admin-token", "dev-auth-secret",
		"mysql://user:pass", "https://evil.test",
	}
	for _, needle := range forbidden {
		if strings.Contains(lower, needle) {
			t.Fatalf("response must not contain %q; body=%s", needle, body)
		}
	}
	_ = mediaSensitivePlayback
	_ = mediaSensitivePlayPage
	_ = mediaSensitiveHLSKey
	_ = mediaSensitiveMigErr
	_ = mediaSensitiveSyncErr
}

func TestAdminMediaOverviewAuthAndCounts(t *testing.T) {
	users := store.NewMemoryUsers()
	mem := sampleAdminMedia()
	r := testAdminMediaRouter(users, mem)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/media", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth=%d", w.Code)
	}

	tokUser := userToken(t, users, "media-user@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/media", nil)
	req.Header.Set("Authorization", "Bearer "+tokUser)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("user=%d", w.Code)
	}

	tok := adminToken(t, users, "media-admin@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/media", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			EpisodesTotal    int     `json:"episodes_total"`
			MigratedCount    int     `json:"migrated_count"`
			FailedCount      int     `json:"failed_count"`
			NotMigratedCount int     `json:"not_migrated_count"`
			EligibleCount    int     `json:"eligible_count"`
			StorageR2Count   int     `json:"storage_r2_count"`
			LatestMigratedAt *string `json:"latest_migrated_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Data.EpisodesTotal != 4 || resp.Data.MigratedCount != 1 ||
		resp.Data.FailedCount != 1 || resp.Data.NotMigratedCount != 2 ||
		resp.Data.EligibleCount != 2 || resp.Data.StorageR2Count != 1 ||
		resp.Data.LatestMigratedAt == nil {
		t.Fatalf("unexpected overview: %+v", resp.Data)
	}
	assertNoForbiddenMediaFields(t, w.Body.String())
}

func TestAdminListMediaEpisodesFiltersAndPagination(t *testing.T) {
	users := store.NewMemoryUsers()
	tok := adminToken(t, users, "media-ops@example.com")
	r := testAdminMediaRouter(users, sampleAdminMedia())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/episodes", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth=%d", w.Code)
	}

	tokUser := userToken(t, users, "media-viewer@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/episodes", nil)
	req.Header.Set("Authorization", "Bearer "+tokUser)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("user=%d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/episodes?limit=2&offset=0", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		OK      bool                   `json:"ok"`
		Data    []AdminMediaEpisodeDTO `json:"data"`
		Total   int                    `json:"total"`
		Limit   int                    `json:"limit"`
		Offset  int                    `json:"offset"`
		HasMore bool                   `json:"has_more"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Total != 4 || resp.Limit != 2 || !resp.HasMore || len(resp.Data) != 2 {
		t.Fatalf("pagination unexpected: %+v", resp)
	}
	if resp.Data[0].EpisodeID != 4 {
		t.Fatalf("expected newest-first id=4, got %d", resp.Data[0].EpisodeID)
	}
	assertNoForbiddenMediaFields(t, w.Body.String())

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/episodes?migration_status=failed", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 1 || !resp.Data[0].HasError {
		t.Fatalf("failed filter: %+v", resp)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/episodes?migration_status=done", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 1 || resp.Data[0].MigrationStatus != "done" {
		t.Fatalf("done filter: %+v", resp)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/episodes?migration_status=pending", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 2 {
		t.Fatalf("pending filter total=%d", resp.Total)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/episodes?storage_provider=r2", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 1 || resp.Data[0].StorageProvider != "r2" {
		t.Fatalf("provider filter: %+v", resp)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/episodes?q=Beta", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 1 || resp.Data[0].VideoTitle != "Beta Show" {
		t.Fatalf("q filter: %+v", resp)
	}
}

func TestAdminListMediaEpisodesInvalidFilters(t *testing.T) {
	users := store.NewMemoryUsers()
	tok := adminToken(t, users, "media-bad@example.com")
	r := testAdminMediaRouter(users, sampleAdminMedia())
	for _, path := range []string{
		"/api/v1/admin/media/episodes?limit=abc",
		"/api/v1/admin/media/episodes?offset=-1",
		"/api/v1/admin/media/episodes?migration_status=weird",
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

func TestAdminGetMediaEpisodeAuthAndDetail(t *testing.T) {
	users := store.NewMemoryUsers()
	r := testAdminMediaRouter(users, sampleAdminMedia())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/episodes/3", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth=%d", w.Code)
	}

	tokUser := userToken(t, users, "media-d@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/episodes/3", nil)
	req.Header.Set("Authorization", "Bearer "+tokUser)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("user=%d", w.Code)
	}

	tok := adminToken(t, users, "media-detail@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/episodes/3", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		OK   bool                 `json:"ok"`
		Data AdminMediaEpisodeDTO `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Data.EpisodeID != 3 || resp.Data.HasHLSObject == nil || !*resp.Data.HasHLSObject {
		t.Fatalf("unexpected detail: %+v", resp.Data)
	}
	assertNoForbiddenMediaFields(t, w.Body.String())
	if strings.Contains(w.Body.String(), mediaSensitiveHLSKey) {
		t.Fatal("hls key leaked")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/episodes/999", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing=%d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/episodes/abc", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("malformed=%d", w.Code)
	}
}

func TestAdminMediaDTOOmitsSensitiveJSON(t *testing.T) {
	hls := true
	raw, err := json.Marshal(AdminMediaEpisodeDTO{
		EpisodeID: 1, VideoID: 2, VideoTitle: "T", HasError: true, HasHLSObject: &hls,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNoForbiddenMediaFields(t, string(raw))
}
