package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/store"
)

// adminOpsRepo is the store surface for admin operational_runs reads.
type adminOpsRepo interface {
	ListAdminOperationalRunsFiltered(jobType, triggerType, status string, limit, offset int) ([]store.OperationalRunRow, error)
	CountAdminOperationalRunsFiltered(jobType, triggerType, status string) (int, error)
	GetAdminOperationalRunByID(id uint64) (*store.OperationalRunRow, error)
}

func (a *API) adminOpsRepo() adminOpsRepo {
	if a.AdminOps != nil {
		return a.AdminOps
	}
	if a.Store != nil {
		return a.Store
	}
	return nil
}

type adminOpsListQuery struct {
	Limit       int
	Offset      int
	JobType     string
	TriggerType string
	Status      string
}

func parseAdminOpsListQuery(c *gin.Context) (adminOpsListQuery, string, bool) {
	out := adminOpsListQuery{Limit: 24, Offset: 0}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return out, "invalid limit", false
		}
		out.Limit = n
	}
	if out.Limit == 0 {
		out.Limit = 24
	}
	if out.Limit > 100 {
		out.Limit = 100
	}

	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return out, "invalid offset", false
		}
		out.Offset = n
	}

	job, msg, ok := parseOpsJobTypeFilter(c.Query("job_type"))
	if !ok {
		return out, msg, false
	}
	out.JobType = job

	trig, msg, ok := parseOpsTriggerTypeFilter(c.Query("trigger_type"))
	if !ok {
		return out, msg, false
	}
	out.TriggerType = trig

	st, msg, ok := parseOpsStatusFilter(c.Query("status"))
	if !ok {
		return out, msg, false
	}
	out.Status = st

	return out, "", true
}

func parseOpsJobTypeFilter(raw string) (string, string, bool) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", "", true
	}
	switch strings.ToLower(v) {
	case store.JobIncrementalSync, store.JobEnrichment, store.JobMediaMigration:
		return strings.ToLower(v), "", true
	default:
		return "", "invalid job_type", false
	}
}

func parseOpsTriggerTypeFilter(raw string) (string, string, bool) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", "", true
	}
	switch strings.ToLower(v) {
	case store.TriggerCLI, store.TriggerTimer:
		return strings.ToLower(v), "", true
	default:
		return "", "invalid trigger_type", false
	}
}

func parseOpsStatusFilter(raw string) (string, string, bool) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", "", true
	}
	switch strings.ToLower(v) {
	case store.OpStatusRunning, store.OpStatusSucceeded, store.OpStatusFailed, store.OpStatusRejected:
		return strings.ToLower(v), "", true
	default:
		return "", "invalid status", false
	}
}

// operationalRunJSON builds the allowlisted browser DTO (derived stale/duration; no raw errors).
func operationalRunJSON(r *store.OperationalRunRow, now time.Time) gin.H {
	out := gin.H{
		"id":                   r.ID,
		"job_type":             r.JobType,
		"trigger_type":         r.TriggerType,
		"status":               r.Status,
		"requested_by_user_id": nil,
		"requested_at":         r.RequestedAt,
		"started_at":           nil,
		"finished_at":          nil,
		"duration_seconds":     nil,
		"stale":                isOperationalRunStale(r, now),
		"scanned":              nil,
		"created_count":        nil,
		"updated_count":        nil,
		"unchanged_count":      nil,
		"failed_count":         nil,
		"selected_count":       nil,
		"succeeded_count":      nil,
		"skipped_count":        nil,
		"unavailable_count":    nil,
		"has_error":            r.HasError,
		"error_code":           r.ErrorCode,
		"sync_run_id":          nil,
	}
	if r.RequestedByUserID != nil {
		out["requested_by_user_id"] = *r.RequestedByUserID
	}
	if r.StartedAt != nil {
		out["started_at"] = *r.StartedAt
	}
	if r.FinishedAt != nil {
		out["finished_at"] = *r.FinishedAt
	}
	if d := operationalDurationSeconds(r, now); d != nil {
		out["duration_seconds"] = *d
	}
	if r.Scanned != nil {
		out["scanned"] = *r.Scanned
	}
	if r.CreatedCount != nil {
		out["created_count"] = *r.CreatedCount
	}
	if r.UpdatedCount != nil {
		out["updated_count"] = *r.UpdatedCount
	}
	if r.UnchangedCount != nil {
		out["unchanged_count"] = *r.UnchangedCount
	}
	if r.FailedCount != nil {
		out["failed_count"] = *r.FailedCount
	}
	if r.SelectedCount != nil {
		out["selected_count"] = *r.SelectedCount
	}
	if r.SucceededCount != nil {
		out["succeeded_count"] = *r.SucceededCount
	}
	if r.SkippedCount != nil {
		out["skipped_count"] = *r.SkippedCount
	}
	if r.UnavailableCount != nil {
		out["unavailable_count"] = *r.UnavailableCount
	}
	if r.SyncRunID != nil {
		out["sync_run_id"] = *r.SyncRunID
	}
	return out
}

func isOperationalRunStale(r *store.OperationalRunRow, now time.Time) bool {
	if r == nil || r.Status != store.OpStatusRunning {
		return false
	}
	var threshold time.Duration
	switch r.JobType {
	case store.JobIncrementalSync:
		threshold = store.IncrementalStaleAfter
	case store.JobEnrichment:
		threshold = store.EnrichmentStaleAfter
	case store.JobMediaMigration:
		threshold = store.MediaMigrationStaleAfter
	default:
		return false
	}
	anchor := r.RequestedAt
	if r.StartedAt != nil {
		anchor = *r.StartedAt
	}
	return now.Sub(anchor) > threshold
}

func operationalDurationSeconds(r *store.OperationalRunRow, now time.Time) *int {
	if r == nil || r.StartedAt == nil {
		return nil
	}
	var end time.Time
	if r.Status == store.OpStatusRunning {
		end = now
	} else {
		if r.FinishedAt == nil {
			return nil
		}
		end = *r.FinishedAt
	}
	secs := int(end.Sub(*r.StartedAt).Seconds())
	if secs < 0 {
		secs = 0
	}
	return &secs
}

// AdminListOperationalRuns GET /api/v1/admin/sync/operations
func (a *API) AdminListOperationalRuns(c *gin.Context) {
	q, msg, ok := parseAdminOpsListQuery(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": msg})
		return
	}
	repo := a.adminOpsRepo()
	if repo == nil {
		c.JSON(http.StatusOK, gin.H{
			"ok": true, "data": []gin.H{},
			"total": 0, "limit": q.Limit, "offset": q.Offset, "has_more": false,
		})
		return
	}
	total, err := repo.CountAdminOperationalRunsFiltered(q.JobType, q.TriggerType, q.Status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载运行记录"})
		return
	}
	rows, err := repo.ListAdminOperationalRunsFiltered(q.JobType, q.TriggerType, q.Status, q.Limit, q.Offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载运行记录"})
		return
	}
	now := time.Now()
	out := make([]gin.H, 0, len(rows))
	for i := range rows {
		out = append(out, operationalRunJSON(&rows[i], now))
	}
	c.JSON(http.StatusOK, gin.H{
		"ok": true, "data": out,
		"total": total, "limit": q.Limit, "offset": q.Offset,
		"has_more": q.Offset+len(out) < total,
	})
}

// AdminGetOperationalRun GET /api/v1/admin/sync/operations/:id
func (a *API) AdminGetOperationalRun(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	repo := a.adminOpsRepo()
	if repo == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	row, err := repo.GetAdminOperationalRunByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载运行记录"})
		return
	}
	if row == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": operationalRunJSON(row, time.Now())})
}
