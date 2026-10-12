package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/store"
)

// AdminDashboard returns lightweight operational stats for the admin home.
// Protected by RequireUser + RequireAdmin. Never exposes secrets or credentials.
func (a *API) AdminDashboard(c *gin.Context) {
	data := gin.H{
		"status":           "ok",
		"users_total":      0,
		"videos_total":     0,
		"videos_active":    0,
		"categories_total": 0,
		"latest_sync":      nil,
	}
	if a.Store != nil {
		stats, err := a.Store.AdminDashboardStats()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载仪表盘数据"})
			return
		}
		data["users_total"] = stats.UsersTotal
		data["videos_total"] = stats.VideosTotal
		data["videos_active"] = stats.VideosActive
		data["categories_total"] = stats.CategoriesTotal
		if stats.LatestSync != nil {
			data["latest_sync"] = syncRunJSON(stats.LatestSync)
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": data})
}

func syncRunJSON(s *store.AdminSyncRunSummary) gin.H {
	out := gin.H{
		"id":                s.ID,
		"source_system":     s.SourceSystem,
		"trigger_type":      s.TriggerType,
		"started_at":        s.StartedAt,
		"scanned":           s.Scanned,
		"created_count":     s.CreatedCount,
		"updated_count":     s.UpdatedCount,
		"unchanged_count":   s.UnchangedCount,
		"failed_count":      s.FailedCount,
		"deactivated_count": s.DeactivatedCount,
		"has_error":         s.HasError,
	}
	if s.FinishedAt != nil {
		out["finished_at"] = s.FinishedAt
	}
	return out
}
