package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/store"
)

// adminSyncRepo is the store surface for admin sync reads.
type adminSyncRepo interface {
	AdminSyncOverviewStats() (*store.AdminSyncOverview, error)
	ListAdminSyncRunsFiltered(triggerType string, limit, offset int) ([]store.AdminSyncRunSummary, error)
	CountAdminSyncRunsFiltered(triggerType string) (int, error)
	GetAdminSyncRunByID(id uint64) (*store.AdminSyncRunSummary, error)
}

func (a *API) adminSyncRepo() adminSyncRepo {
	if a.AdminSync != nil {
		return a.AdminSync
	}
	if a.Store != nil {
		return a.Store
	}
	return nil
}

type adminSyncListQuery struct {
	Limit       int
	Offset      int
	TriggerType string
}

func parseAdminSyncListQuery(c *gin.Context) (adminSyncListQuery, string, bool) {
	out := adminSyncListQuery{Limit: 24, Offset: 0}
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

	// Parameterized equality filter; no canonical allowlist in product code.
	out.TriggerType = strings.TrimSpace(c.Query("trigger_type"))
	return out, "", true
}

// AdminSyncOverview GET /api/v1/admin/sync
func (a *API) AdminSyncOverview(c *gin.Context) {
	repo := a.adminSyncRepo()
	if repo == nil {
		c.JSON(http.StatusOK, gin.H{
			"ok": true,
			"data": gin.H{
				"latest_sync":                 nil,
				"latest_incremental":          nil,
				"latest_full":                 nil,
				"pending_enrichment_count":    0,
				"videos_sync_error_count":     0,
				"categories_sync_error_count": 0,
				"videos_active":               0,
				"videos_inactive":             0,
			},
		})
		return
	}
	stats, err := repo.AdminSyncOverviewStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载同步概览"})
		return
	}
	data := gin.H{
		"latest_sync":                 nil,
		"latest_incremental":          nil,
		"latest_full":                 nil,
		"pending_enrichment_count":    stats.PendingEnrichmentCount,
		"videos_sync_error_count":     stats.VideosSyncErrorCount,
		"categories_sync_error_count": stats.CategoriesSyncErrorCount,
		"videos_active":               stats.VideosActive,
		"videos_inactive":             stats.VideosInactive,
	}
	if stats.LatestSync != nil {
		data["latest_sync"] = syncRunJSON(stats.LatestSync)
	}
	if stats.LatestIncremental != nil {
		data["latest_incremental"] = syncRunJSON(stats.LatestIncremental)
	}
	if stats.LatestFull != nil {
		data["latest_full"] = syncRunJSON(stats.LatestFull)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": data})
}

// AdminListSyncRuns GET /api/v1/admin/sync/runs
func (a *API) AdminListSyncRuns(c *gin.Context) {
	q, msg, ok := parseAdminSyncListQuery(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": msg})
		return
	}
	repo := a.adminSyncRepo()
	if repo == nil {
		c.JSON(http.StatusOK, gin.H{
			"ok": true, "data": []gin.H{},
			"total": 0, "limit": q.Limit, "offset": q.Offset, "has_more": false,
		})
		return
	}
	total, err := repo.CountAdminSyncRunsFiltered(q.TriggerType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载同步记录"})
		return
	}
	rows, err := repo.ListAdminSyncRunsFiltered(q.TriggerType, q.Limit, q.Offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载同步记录"})
		return
	}
	out := make([]gin.H, 0, len(rows))
	for i := range rows {
		out = append(out, syncRunJSON(&rows[i]))
	}
	c.JSON(http.StatusOK, gin.H{
		"ok": true, "data": out,
		"total": total, "limit": q.Limit, "offset": q.Offset,
		"has_more": q.Offset+len(out) < total,
	})
}

// AdminGetSyncRun GET /api/v1/admin/sync/runs/:id
func (a *API) AdminGetSyncRun(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	repo := a.adminSyncRepo()
	if repo == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	row, err := repo.GetAdminSyncRunByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载同步记录"})
		return
	}
	if row == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": syncRunJSON(row)})
}
