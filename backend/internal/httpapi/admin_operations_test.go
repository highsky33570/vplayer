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

type memAdminOps struct {
	rows []store.OperationalRunRow
}

func (m *memAdminOps) ListAdminOperationalRunsFiltered(jobType, triggerType, status string, limit, offset int) ([]store.OperationalRunRow, error) {
	filtered := m.filter(jobType, triggerType, status)
	if offset > len(filtered) {
		return nil, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	out := make([]store.OperationalRunRow, end-offset)
	copy(out, filtered[offset:end])
	return out, nil
}

func (m *memAdminOps) CountAdminOperationalRunsFiltered(jobType, triggerType, status string) (int, error) {
	return len(m.filter(jobType, triggerType, status)), nil
}

func (m *memAdminOps) GetAdminOperationalRunByID(id uint64) (*store.OperationalRunRow, error) {
	for i := range m.rows {
		if m.rows[i].ID == id {
			cp := m.rows[i]
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *memAdminOps) filter(jobType, triggerType, status string) []store.OperationalRunRow {
	job := strings.TrimSpace(jobType)
	trig := strings.TrimSpace(triggerType)
	st := strings.TrimSpace(status)
	var out []store.OperationalRunRow
	for _, r := range m.rows {
		if job != "" && r.JobType != job {
			continue
		}
		if trig != "" && r.TriggerType != trig {
			continue
		}
		if st != "" && r.Status != st {
			continue
		}
		out = append(out, r)
	}
	return out
}

func intPtr(v int) *int { return &v }

func sampleOperationalRuns(now time.Time) []store.OperationalRunRow {
	startInc := now.Add(-5 * time.Minute)
	finInc := now.Add(-4 * time.Minute)
	startEnrich := now.Add(-10 * time.Minute)
	finEnrich := now.Add(-8 * time.Minute)
	startRejected := now.Add(-1 * time.Hour)
	startStaleInc := now.Add(-25 * time.Minute)
	startStaleEnrich := now.Add(-30 * time.Minute)
	startFresh := now.Add(-2 * time.Minute)

	return []store.OperationalRunRow{
		{
			ID: 5, JobType: store.JobIncrementalSync, TriggerType: store.TriggerTimer,
			Status: store.OpStatusRunning, RequestedAt: startFresh, StartedAt: &startFresh,
			HasError: false,
		},
		{
			ID: 4, JobType: store.JobEnrichment, TriggerType: store.TriggerCLI,
			Status: store.OpStatusSucceeded, RequestedAt: startEnrich, StartedAt: &startEnrich, FinishedAt: &finEnrich,
			SelectedCount: intPtr(10), SucceededCount: intPtr(8), SkippedCount: intPtr(1),
			UnavailableCount: intPtr(1), FailedCount: intPtr(0), HasError: false,
		},
		{
			ID: 3, JobType: store.JobIncrementalSync, TriggerType: store.TriggerTimer,
			Status: store.OpStatusSucceeded, RequestedAt: startInc, StartedAt: &startInc, FinishedAt: &finInc,
			Scanned: intPtr(12), CreatedCount: intPtr(2), UpdatedCount: intPtr(3),
			UnchangedCount: intPtr(7), FailedCount: intPtr(0), HasError: false,
		},
		{
			ID: 2, JobType: store.JobIncrementalSync, TriggerType: store.TriggerCLI,
			Status: store.OpStatusRejected, RequestedAt: startRejected, StartedAt: &startRejected, FinishedAt: &startRejected,
			ErrorCode: store.ErrCodeLockBusy, HasError: true,
		},
		{
			ID: 1, JobType: store.JobEnrichment, TriggerType: store.TriggerTimer,
			Status: store.OpStatusRunning, RequestedAt: startStaleEnrich, StartedAt: &startStaleEnrich,
			HasError: false,
		},
		{
			ID: 6, JobType: store.JobIncrementalSync, TriggerType: store.TriggerTimer,
			Status: store.OpStatusRunning, RequestedAt: startStaleInc, StartedAt: &startStaleInc,
			HasError: false,
		},
	}
}

func testAdminOpsRouter(users store.UserRepo, ops adminOpsRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	api := &API{
		Cfg:      config.Config{AuthSecret: "test-auth-secret"},
		Users:    users,
		AdminOps: ops,
	}
	r := gin.New()
	v1 := r.Group("/api/v1")
	{
		v1.GET("/admin/sync/operations", api.RequireUser, api.RequireAdmin, api.AdminListOperationalRuns)
		v1.GET("/admin/sync/operations/:id", api.RequireUser, api.RequireAdmin, api.AdminGetOperationalRun)
	}
	return r
}

func assertNoForbiddenOpsFields(t *testing.T, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	forbidden := []string{
		`"error_text"`, `"summary_json"`, `"playback_url"`, `"play_page_url"`,
		`"hls_object_key"`, `"cover_r2_key"`, `"migration_error"`,
		"password", "auth_secret", "admin_api_token", "cdn_sign",
		"mysql_dsn", "r2_secret", "x-admin-token", "environment",
		"systemd", "os/exec", "command_line",
	}
	for _, needle := range forbidden {
		if strings.Contains(lower, needle) {
			t.Fatalf("response must not contain %q; body=%s", needle, body)
		}
	}
}

func TestAdminListOperationalRunsAuth(t *testing.T) {
	users := store.NewMemoryUsers()
	now := time.Date(2026, 10, 12, 18, 0, 0, 0, time.UTC)
	r := testAdminOpsRouter(users, &memAdminOps{rows: sampleOperationalRuns(now)})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/operations", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d", w.Code)
	}

	tokUser := userToken(t, users, "ops-user@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/operations", nil)
	req.Header.Set("Authorization", "Bearer "+tokUser)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("user status=%d", w.Code)
	}

	tok := adminToken(t, users, "ops-admin@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/operations", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin status=%d body=%s", w.Code, w.Body.String())
	}
	assertNoForbiddenOpsFields(t, w.Body.String())
}

func TestAdminListOperationalRunsShapesAndFilters(t *testing.T) {
	users := store.NewMemoryUsers()
	tok := adminToken(t, users, "ops-filter@example.com")
	now := time.Date(2026, 10, 12, 18, 0, 0, 0, time.UTC)
	r := testAdminOpsRouter(users, &memAdminOps{rows: sampleOperationalRuns(now)})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/operations?limit=100", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
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
	if !resp.OK || resp.Total != 6 || len(resp.Data) != 6 {
		t.Fatalf("unexpected list: total=%d len=%d", resp.Total, len(resp.Data))
	}

	var incremental, enrichment, rejected map[string]any
	for _, row := range resp.Data {
		switch row["id"] {
		case float64(3):
			incremental = row
		case float64(4):
			enrichment = row
		case float64(2):
			rejected = row
		}
	}
	if incremental == nil || enrichment == nil || rejected == nil {
		t.Fatalf("missing shaped rows: %+v", resp.Data)
	}
	if incremental["job_type"] != store.JobIncrementalSync || incremental["scanned"] != float64(12) {
		t.Fatalf("incremental shape: %+v", incremental)
	}
	if incremental["selected_count"] != nil {
		t.Fatalf("incremental selected_count should be null: %+v", incremental["selected_count"])
	}
	if enrichment["job_type"] != store.JobEnrichment || enrichment["selected_count"] != float64(10) {
		t.Fatalf("enrichment shape: %+v", enrichment)
	}
	if enrichment["scanned"] != nil {
		t.Fatalf("enrichment scanned should be null: %+v", enrichment["scanned"])
	}
	if rejected["status"] != store.OpStatusRejected || rejected["error_code"] != store.ErrCodeLockBusy || rejected["has_error"] != true {
		t.Fatalf("rejected shape: %+v", rejected)
	}
	assertNoForbiddenOpsFields(t, w.Body.String())

	cases := []struct {
		qs    string
		total int
		check func(map[string]any) bool
	}{
		{"job_type=incremental_sync", 4, func(m map[string]any) bool { return m["job_type"] == store.JobIncrementalSync }},
		{"job_type=enrichment", 2, func(m map[string]any) bool { return m["job_type"] == store.JobEnrichment }},
		{"trigger_type=cli", 2, func(m map[string]any) bool { return m["trigger_type"] == store.TriggerCLI }},
		{"trigger_type=timer", 4, func(m map[string]any) bool { return m["trigger_type"] == store.TriggerTimer }},
		{"status=rejected", 1, func(m map[string]any) bool { return m["status"] == store.OpStatusRejected }},
		{"status=running", 3, func(m map[string]any) bool { return m["status"] == store.OpStatusRunning }},
		{"job_type=enrichment&trigger_type=cli&status=succeeded", 1, func(m map[string]any) bool {
			return m["job_type"] == store.JobEnrichment && m["trigger_type"] == store.TriggerCLI && m["status"] == store.OpStatusSucceeded
		}},
	}
	for _, tc := range cases {
		req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/operations?"+tc.qs, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if resp.Total != tc.total || len(resp.Data) != tc.total {
			t.Fatalf("%s: total=%d len=%d", tc.qs, resp.Total, len(resp.Data))
		}
		for _, row := range resp.Data {
			if !tc.check(row) {
				t.Fatalf("%s row mismatch: %+v", tc.qs, row)
			}
		}
	}
}

func TestAdminListOperationalRunsPaginationAndValidation(t *testing.T) {
	users := store.NewMemoryUsers()
	tok := adminToken(t, users, "ops-page@example.com")
	now := time.Date(2026, 10, 12, 18, 0, 0, 0, time.UTC)
	r := testAdminOpsRouter(users, &memAdminOps{rows: sampleOperationalRuns(now)})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/operations?limit=2&offset=0", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
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
	if !resp.OK || resp.Total != 6 || resp.Limit != 2 || !resp.HasMore || len(resp.Data) != 2 {
		t.Fatalf("pagination unexpected: %+v", resp)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/operations?limit=999", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Limit != 100 {
		t.Fatalf("max limit not enforced: %d", resp.Limit)
	}

	for _, path := range []string{
		"/api/v1/admin/sync/operations?limit=abc",
		"/api/v1/admin/sync/operations?offset=-1",
		"/api/v1/admin/sync/operations?job_type=incremental",
		"/api/v1/admin/sync/operations?trigger_type=admin",
		"/api/v1/admin/sync/operations?trigger_type=manual",
		"/api/v1/admin/sync/operations?status=pending",
	} {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
}

func TestOperationalRunStaleAndDuration(t *testing.T) {
	now := time.Date(2026, 10, 12, 18, 0, 0, 0, time.UTC)
	freshStart := now.Add(-5 * time.Minute)
	staleIncStart := now.Add(-21 * time.Minute)
	staleEnrichStart := now.Add(-26 * time.Minute)
	completedStart := now.Add(-10 * time.Minute)
	completedFin := now.Add(-7 * time.Minute)
	rejectedStart := now.Add(-1 * time.Hour)

	fresh := &store.OperationalRunRow{
		ID: 1, JobType: store.JobIncrementalSync, Status: store.OpStatusRunning,
		RequestedAt: freshStart, StartedAt: &freshStart,
	}
	if isOperationalRunStale(fresh, now) {
		t.Fatal("fresh running incremental must not be stale")
	}
	if d := operationalDurationSeconds(fresh, now); d == nil || *d != 300 {
		t.Fatalf("running duration: %v", d)
	}

	staleInc := &store.OperationalRunRow{
		ID: 2, JobType: store.JobIncrementalSync, Status: store.OpStatusRunning,
		RequestedAt: staleIncStart, StartedAt: &staleIncStart,
	}
	if !isOperationalRunStale(staleInc, now) {
		t.Fatal("incremental >20m must be stale")
	}

	staleEnrich := &store.OperationalRunRow{
		ID: 3, JobType: store.JobEnrichment, Status: store.OpStatusRunning,
		RequestedAt: staleEnrichStart, StartedAt: &staleEnrichStart,
	}
	if !isOperationalRunStale(staleEnrich, now) {
		t.Fatal("enrichment >25m must be stale")
	}

	completed := &store.OperationalRunRow{
		ID: 4, JobType: store.JobIncrementalSync, Status: store.OpStatusSucceeded,
		RequestedAt: completedStart, StartedAt: &completedStart, FinishedAt: &completedFin,
	}
	if isOperationalRunStale(completed, now) {
		t.Fatal("completed must not be stale")
	}
	if d := operationalDurationSeconds(completed, now); d == nil || *d != 180 {
		t.Fatalf("completed duration: %v", d)
	}

	rejected := &store.OperationalRunRow{
		ID: 5, JobType: store.JobIncrementalSync, Status: store.OpStatusRejected,
		RequestedAt: rejectedStart, StartedAt: &rejectedStart, FinishedAt: &rejectedStart,
		ErrorCode: store.ErrCodeLockBusy, HasError: true,
	}
	if d := operationalDurationSeconds(rejected, now); d == nil || *d != 0 {
		t.Fatalf("rejected duration: %v", d)
	}

	noStart := &store.OperationalRunRow{
		ID: 6, JobType: store.JobIncrementalSync, Status: store.OpStatusRunning,
		RequestedAt: now.Add(-30 * time.Minute),
	}
	if d := operationalDurationSeconds(noStart, now); d != nil {
		t.Fatalf("missing started_at duration must be null: %v", d)
	}
	// Age falls back to requested_at for stale.
	if !isOperationalRunStale(noStart, now) {
		t.Fatal("running without started_at should use requested_at for stale")
	}

	unknownJob := &store.OperationalRunRow{
		ID: 7, JobType: "mystery", Status: store.OpStatusRunning,
		RequestedAt: now.Add(-time.Hour), StartedAt: ptrTime(now.Add(-time.Hour)),
	}
	if isOperationalRunStale(unknownJob, now) {
		t.Fatal("unknown job_type must not invent a stale threshold")
	}

	raw := operationalRunJSON(completed, now)
	body, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	assertNoForbiddenOpsFields(t, string(body))
	if raw["stale"] != false || raw["duration_seconds"] != 180 {
		t.Fatalf("dto derived fields: %+v", raw)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestAdminGetOperationalRunAuthAndDetail(t *testing.T) {
	users := store.NewMemoryUsers()
	now := time.Date(2026, 10, 12, 18, 0, 0, 0, time.UTC)
	r := testAdminOpsRouter(users, &memAdminOps{rows: sampleOperationalRuns(now)})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/operations/3", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth=%d", w.Code)
	}

	tokUser := userToken(t, users, "ops-detail-user@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/operations/3", nil)
	req.Header.Set("Authorization", "Bearer "+tokUser)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("user=%d", w.Code)
	}

	tok := adminToken(t, users, "ops-detail-admin@example.com")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/operations/3", nil)
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
	if !resp.OK || resp.Data["id"] != float64(3) || resp.Data["job_type"] != store.JobIncrementalSync {
		t.Fatalf("unexpected detail: %+v", resp.Data)
	}
	if _, ok := resp.Data["requested_by_user_id"]; !ok {
		t.Fatal("expected requested_by_user_id key")
	}
	if resp.Data["requested_by_user_id"] != nil {
		t.Fatalf("expected null requester, got %+v", resp.Data["requested_by_user_id"])
	}
	assertNoForbiddenOpsFields(t, w.Body.String())

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/operations/999", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing=%d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/operations/abc", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("malformed=%d", w.Code)
	}
}

func TestAdminListOperationalRunsEmpty(t *testing.T) {
	users := store.NewMemoryUsers()
	tok := adminToken(t, users, "ops-empty@example.com")
	r := testAdminOpsRouter(users, &memAdminOps{rows: nil})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/sync/operations", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	var resp struct {
		OK    bool             `json:"ok"`
		Data  []map[string]any `json:"data"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Total != 0 || len(resp.Data) != 0 {
		t.Fatalf("empty unexpected: %+v", resp)
	}
}
