package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/store"
)

// adminActionGuards is the read-only operational_runs surface for Admin action UX guards.
type adminActionGuards interface {
	HasNonStaleRunningOperationalRun(jobType string, now time.Time) (bool, error)
	HasRecentAdminOperationalRequest(jobType string, now time.Time, cooldown time.Duration) (bool, error)
}

func (a *API) adminActionGuards() adminActionGuards {
	if a.AdminActionGuards != nil {
		return a.AdminActionGuards
	}
	if a.Store != nil {
		return a.Store
	}
	return nil
}

func (a *API) workerLauncher() WorkerLauncher {
	if a.WorkerLauncher != nil {
		return a.WorkerLauncher
	}
	return ExecWorkerLauncher{Log: slog.Default().With("component", "admin-worker-launcher")}
}

// currentAuthUserID returns the RequireUser-populated numeric user id.
func currentAuthUserID(c *gin.Context) (uint64, bool) {
	uidVal, ok := c.Get(ctxAuthUserID)
	if !ok {
		return 0, false
	}
	userID, ok := uidVal.(uint64)
	if !ok || userID == 0 {
		return 0, false
	}
	return userID, true
}

func discardRequestBody(c *gin.Context) {
	if c.Request.Body != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(c.Request.Body, 1<<20))
		_ = c.Request.Body.Close()
	}
}

func (a *API) launchAdminWorker(c *gin.Context, jobType, bin string, argsFn func(uint64) []string) {
	discardRequestBody(c)

	adminID, ok := currentAuthUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "未登录"})
		return
	}

	now := time.Now()
	guards := a.adminActionGuards()
	if guards == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": "worker_unavailable"})
		return
	}
	busy, err := guards.HasNonStaleRunningOperationalRun(jobType, now)
	if err != nil {
		slog.Default().Error("admin action busy check failed", "job", jobType, "err", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": "worker_unavailable"})
		return
	}
	if busy {
		c.JSON(http.StatusConflict, gin.H{"ok": false, "error": "operation_busy"})
		return
	}
	cool, err := guards.HasRecentAdminOperationalRequest(jobType, now, store.AdminActionCooldown)
	if err != nil {
		slog.Default().Error("admin action cooldown check failed", "job", jobType, "err", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": "worker_unavailable"})
		return
	}
	if cool {
		c.JSON(http.StatusTooManyRequests, gin.H{"ok": false, "error": "operation_cooldown"})
		return
	}

	args := argsFn(adminID)
	if err := a.workerLauncher().Start(bin, args, WorkerWorkingDir); err != nil {
		slog.Default().Error("admin worker launch failed",
			"job", jobType, "err", err.Error(), "cmd", formatLaunchLog(bin, args))
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": "worker_unavailable"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"ok":       true,
		"message":  "Run request accepted.",
		"job_type": jobType,
	})
}

// AdminRunIncremental POST /api/v1/admin/sync/actions/incremental
func (a *API) AdminRunIncremental(c *gin.Context) {
	a.launchAdminWorker(c, store.JobIncrementalSync, SyncIncrementalBin, incrementalAdminArgs)
}

// AdminRunEnrichment POST /api/v1/admin/sync/actions/enrichment
func (a *API) AdminRunEnrichment(c *gin.Context) {
	a.launchAdminWorker(c, store.JobEnrichment, EnrichPendingBin, enrichmentAdminArgs)
}
