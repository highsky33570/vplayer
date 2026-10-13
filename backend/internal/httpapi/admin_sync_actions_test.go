package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/auth"
	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/store"
)

type memActionGuards struct {
	mu            sync.Mutex
	busy          map[string]bool
	recentAdmin   map[string]bool
	busyErr       error
	cooldownErr   error
}

func (m *memActionGuards) HasNonStaleRunningOperationalRun(jobType string, now time.Time) (bool, error) {
	_ = now
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busyErr != nil {
		return false, m.busyErr
	}
	return m.busy[jobType], nil
}

func (m *memActionGuards) HasRecentAdminOperationalRequest(jobType string, now time.Time, cooldown time.Duration) (bool, error) {
	_ = now
	_ = cooldown
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cooldownErr != nil {
		return false, m.cooldownErr
	}
	return m.recentAdmin[jobType], nil
}

type recordingLauncher struct {
	mu      sync.Mutex
	calls   []launchCall
	failErr error
}

type launchCall struct {
	Path    string
	Args    []string
	WorkDir string
}

func (r *recordingLauncher) Start(path string, args []string, workDir string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := append([]string(nil), args...)
	r.calls = append(r.calls, launchCall{Path: path, Args: cp, WorkDir: workDir})
	return r.failErr
}

func (r *recordingLauncher) last() launchCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.calls) == 0 {
		return launchCall{}
	}
	return r.calls[len(r.calls)-1]
}

func testAdminActionsRouter(users store.UserRepo, guards adminActionGuards, launch WorkerLauncher) *gin.Engine {
	gin.SetMode(gin.TestMode)
	api := &API{
		Cfg:               config.Config{AuthSecret: "test-auth-secret"},
		Users:             users,
		AdminActionGuards: guards,
		WorkerLauncher:    launch,
	}
	r := gin.New()
	v1 := r.Group("/api/v1")
	{
		v1.POST("/admin/sync/actions/incremental", api.RequireUser, api.RequireAdmin, api.AdminRunIncremental)
		v1.POST("/admin/sync/actions/enrichment", api.RequireUser, api.RequireAdmin, api.AdminRunEnrichment)
	}
	return r
}

func assertSafeActionBody(t *testing.T, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	for _, needle := range []string{
		"/opt/vplayer", "syncincremental", "enrichpending",
		"--limit", "--trigger", "requested-by-user-id",
		"password", "mysql_dsn", "admin_api_token", "stack",
		"no such file", "permission denied", "exec:",
	} {
		if strings.Contains(lower, needle) {
			t.Fatalf("response must not contain %q; body=%s", needle, body)
		}
	}
}

func TestAdminSyncActionsAuth(t *testing.T) {
	users := store.NewMemoryUsers()
	launch := &recordingLauncher{}
	r := testAdminActionsRouter(users, &memActionGuards{}, launch)
	tokUser := userToken(t, users, "actions-user@example.com")

	for _, path := range []string{
		"/api/v1/admin/sync/actions/incremental",
		"/api/v1/admin/sync/actions/enrichment",
	} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s unauth=%d", path, w.Code)
		}

		req = httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("Authorization", "Bearer "+tokUser)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s user=%d", path, w.Code)
		}
	}
	if len(launch.calls) != 0 {
		t.Fatal("launcher must not run for unauth/non-admin")
	}
}

func TestAdminRunIncrementalAcceptedFixedArgs(t *testing.T) {
	users := store.NewMemoryUsers()
	admin, err := users.CreateUser("actions-inc@example.com", "password1", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := users.SetIsAdmin(admin.ID, true); err != nil {
		t.Fatal(err)
	}
	tok := adminTokenForUser(t, admin.ID, admin.Email)
	launch := &recordingLauncher{}
	r := testAdminActionsRouter(users, &memActionGuards{
		busy:        map[string]bool{},
		recentAdmin: map[string]bool{},
	}, launch)

	body := `{"limit":999,"trigger":"cli","requested_by_user_id":1,"executable":"/evil","args":["--dry-run"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/sync/actions/incremental", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["ok"] != true || resp["message"] != "Run request accepted." || resp["job_type"] != store.JobIncrementalSync {
		t.Fatalf("%+v", resp)
	}
	if _, ok := resp["operational_run_id"]; ok {
		t.Fatal("must not return operational_run_id")
	}
	assertSafeActionBody(t, w.Body.String())

	call := launch.last()
	if call.Path != SyncIncrementalBin || call.WorkDir != WorkerWorkingDir {
		t.Fatalf("path/dir: %+v", call)
	}
	wantArgs := incrementalAdminArgs(admin.ID)
	if len(call.Args) != len(wantArgs) {
		t.Fatalf("args=%v want=%v", call.Args, wantArgs)
	}
	for i := range wantArgs {
		if call.Args[i] != wantArgs[i] {
			t.Fatalf("args=%v want=%v", call.Args, wantArgs)
		}
	}
	if strings.Contains(strings.Join(call.Args, " "), "999") || strings.Contains(strings.Join(call.Args, " "), "evil") {
		t.Fatal("body must not influence argv")
	}
}

func TestAdminRunEnrichmentAcceptedFixedArgs(t *testing.T) {
	users := store.NewMemoryUsers()
	admin, err := users.CreateUser("actions-en@example.com", "password1", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := users.SetIsAdmin(admin.ID, true); err != nil {
		t.Fatal(err)
	}
	tok := adminTokenForUser(t, admin.ID, admin.Email)
	launch := &recordingLauncher{}
	r := testAdminActionsRouter(users, &memActionGuards{}, launch)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/sync/actions/enrichment", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	call := launch.last()
	want := enrichmentAdminArgs(admin.ID)
	if call.Path != EnrichPendingBin || call.WorkDir != WorkerWorkingDir {
		t.Fatalf("%+v", call)
	}
	for i := range want {
		if call.Args[i] != want[i] {
			t.Fatalf("args=%v want=%v", call.Args, want)
		}
	}
}

func adminTokenForUser(t *testing.T, id uint64, email string) string {
	t.Helper()
	tok, err := auth.IssueToken("test-auth-secret", id, email, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestAdminSyncActionsGuardsAndLaunchFailure(t *testing.T) {
	users := store.NewMemoryUsers()
	tok := adminToken(t, users, "actions-guard@example.com")

	t.Run("busy_409", func(t *testing.T) {
		launch := &recordingLauncher{}
		r := testAdminActionsRouter(users, &memActionGuards{
			busy: map[string]bool{store.JobIncrementalSync: true},
		}, launch)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/sync/actions/incremental", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("status=%d", w.Code)
		}
		if !strings.Contains(w.Body.String(), `"operation_busy"`) {
			t.Fatalf("body=%s", w.Body.String())
		}
		assertSafeActionBody(t, w.Body.String())
		if len(launch.calls) != 0 {
			t.Fatal("must not launch when busy")
		}
	})

	t.Run("cooldown_429", func(t *testing.T) {
		launch := &recordingLauncher{}
		r := testAdminActionsRouter(users, &memActionGuards{
			recentAdmin: map[string]bool{store.JobEnrichment: true},
		}, launch)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/sync/actions/enrichment", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("status=%d", w.Code)
		}
		if !strings.Contains(w.Body.String(), `"operation_cooldown"`) {
			t.Fatalf("body=%s", w.Body.String())
		}
		assertSafeActionBody(t, w.Body.String())
	})

	t.Run("stale_running_does_not_block", func(t *testing.T) {
		// mem guard returns false for "stale" (non-busy) — mirrors store semantics.
		launch := &recordingLauncher{}
		r := testAdminActionsRouter(users, &memActionGuards{
			busy: map[string]bool{store.JobIncrementalSync: false},
		}, launch)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/sync/actions/incremental", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusAccepted {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if len(launch.calls) != 1 {
			t.Fatal("expected launch")
		}
	})

	t.Run("launch_failure_503", func(t *testing.T) {
		launch := &recordingLauncher{failErr: errors.New("SENSITIVE_EXEC_ERROR no such file /opt/vplayer/backend/bin/syncincremental")}
		r := testAdminActionsRouter(users, &memActionGuards{}, launch)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/sync/actions/incremental", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status=%d", w.Code)
		}
		if !strings.Contains(w.Body.String(), `"worker_unavailable"`) {
			t.Fatalf("body=%s", w.Body.String())
		}
		assertSafeActionBody(t, w.Body.String())
		if strings.Contains(w.Body.String(), "SENSITIVE_EXEC_ERROR") {
			t.Fatal("raw launch error leaked")
		}
	})

	t.Run("nil_guards_503_no_launch", func(t *testing.T) {
		launch := &recordingLauncher{}
		r := testAdminActionsRouter(users, nil, launch)
		for _, path := range []string{
			"/api/v1/admin/sync/actions/incremental",
			"/api/v1/admin/sync/actions/enrichment",
		} {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			req.Header.Set("Authorization", "Bearer "+tok)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("%s status=%d body=%s", path, w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), `"worker_unavailable"`) {
				t.Fatalf("%s body=%s", path, w.Body.String())
			}
			assertSafeActionBody(t, w.Body.String())
		}
		if len(launch.calls) != 0 {
			t.Fatal("launcher must not run when guards unavailable")
		}
	})

	t.Run("active_query_error_503_no_launch", func(t *testing.T) {
		launch := &recordingLauncher{}
		r := testAdminActionsRouter(users, &memActionGuards{
			busyErr: errors.New("SENSITIVE_DB_BUSY mysql://user:pass@host/db"),
		}, launch)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/sync/actions/incremental", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"worker_unavailable"`) {
			t.Fatalf("body=%s", w.Body.String())
		}
		assertSafeActionBody(t, w.Body.String())
		if strings.Contains(w.Body.String(), "SENSITIVE_DB_BUSY") || strings.Contains(w.Body.String(), "mysql://") {
			t.Fatal("raw DB error leaked")
		}
		if len(launch.calls) != 0 {
			t.Fatal("launcher must not run on active-check error")
		}
	})

	t.Run("cooldown_query_error_503_no_launch", func(t *testing.T) {
		launch := &recordingLauncher{}
		r := testAdminActionsRouter(users, &memActionGuards{
			cooldownErr: errors.New("SENSITIVE_DB_COOLDOWN password=secret"),
		}, launch)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/sync/actions/enrichment", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"worker_unavailable"`) {
			t.Fatalf("body=%s", w.Body.String())
		}
		assertSafeActionBody(t, w.Body.String())
		if strings.Contains(w.Body.String(), "SENSITIVE_DB_COOLDOWN") || strings.Contains(w.Body.String(), "password=") {
			t.Fatal("raw DB error leaked")
		}
		if len(launch.calls) != 0 {
			t.Fatal("launcher must not run on cooldown-check error")
		}
	})
}

func TestFixedAdminArgBuilders(t *testing.T) {
	inc := incrementalAdminArgs(99)
	if strings.Join(inc, " ") != "--limit 5 --trigger=admin --requested-by-user-id=99" {
		t.Fatalf("%v", inc)
	}
	en := enrichmentAdminArgs(7)
	if strings.Join(en, " ") != "--limit 15 --concurrency 1 --trigger=admin --requested-by-user-id=7" {
		t.Fatalf("%v", en)
	}
}
