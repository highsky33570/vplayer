package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/store"
)

type adminMediaRepo interface {
	AdminMediaOverviewStats() (*store.AdminMediaOverview, error)
	ListAdminMediaEpisodesFiltered(migrationStatus, storageProvider, query string, limit, offset int) ([]store.AdminMediaEpisode, error)
	CountAdminMediaEpisodesFiltered(migrationStatus, storageProvider, query string) (int, error)
	GetAdminMediaEpisodeByID(id uint64) (*store.AdminMediaEpisode, error)
}

func (a *API) adminMediaRepo() adminMediaRepo {
	if a.AdminMedia != nil {
		return a.AdminMedia
	}
	if a.Store != nil {
		return a.Store
	}
	return nil
}

// AdminMediaEpisodeDTO is the allowlisted browser media episode response.
type AdminMediaEpisodeDTO struct {
	EpisodeID       uint64     `json:"episode_id"`
	VideoID         uint64     `json:"video_id"`
	VideoTitle      string     `json:"video_title"`
	SID             int        `json:"sid"`
	NID             int        `json:"nid"`
	EpisodeTitle    string     `json:"episode_title"`
	StorageProvider string     `json:"storage_provider"`
	MigrationStatus string     `json:"migration_status"`
	MigratedAt      *time.Time `json:"migrated_at,omitempty"`
	HasError        bool       `json:"has_error"`
	PlaybackStatus  string     `json:"playback_status"`
	HasHLSObject    *bool      `json:"has_hls_object,omitempty"`
}

func adminMediaEpisodeDTO(ep store.AdminMediaEpisode, includeHLS bool) AdminMediaEpisodeDTO {
	out := AdminMediaEpisodeDTO{
		EpisodeID:       ep.EpisodeID,
		VideoID:         ep.VideoID,
		VideoTitle:      ep.VideoTitle,
		SID:             ep.SID,
		NID:             ep.NID,
		EpisodeTitle:    ep.EpisodeTitle,
		StorageProvider: ep.StorageProvider,
		MigrationStatus: ep.MigrationStatus,
		MigratedAt:      ep.MigratedAt,
		HasError:        ep.HasError,
		PlaybackStatus:  ep.PlaybackStatus,
	}
	if includeHLS {
		v := ep.HasHLSObject
		out.HasHLSObject = &v
	}
	return out
}

type adminMediaListQuery struct {
	Limit           int
	Offset          int
	MigrationStatus string
	StorageProvider string
	Query           string
}

func parseAdminMediaListQuery(c *gin.Context) (adminMediaListQuery, string, bool) {
	out := adminMediaListQuery{Limit: 24, Offset: 0}
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

	if raw := strings.TrimSpace(c.Query("migration_status")); raw != "" {
		switch strings.ToLower(raw) {
		case "all":
			out.MigrationStatus = ""
		case "done", "failed", "pending":
			out.MigrationStatus = strings.ToLower(raw)
		default:
			return out, "invalid migration_status", false
		}
	}

	out.StorageProvider = strings.TrimSpace(c.Query("storage_provider"))
	out.Query = strings.TrimSpace(c.Query("q"))
	return out, "", true
}

// AdminMediaOverview GET /api/v1/admin/media
func (a *API) AdminMediaOverview(c *gin.Context) {
	repo := a.adminMediaRepo()
	if repo == nil {
		c.JSON(http.StatusOK, gin.H{
			"ok": true,
			"data": gin.H{
				"episodes_total": 0, "migrated_count": 0, "failed_count": 0,
				"not_migrated_count": 0, "eligible_count": 0, "storage_r2_count": 0,
				"latest_migrated_at": nil,
			},
		})
		return
	}
	stats, err := repo.AdminMediaOverviewStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载媒体概览"})
		return
	}
	data := gin.H{
		"episodes_total":     stats.EpisodesTotal,
		"migrated_count":     stats.MigratedCount,
		"failed_count":       stats.FailedCount,
		"not_migrated_count": stats.NotMigratedCount,
		"eligible_count":     stats.EligibleCount,
		"storage_r2_count":   stats.StorageR2Count,
		"latest_migrated_at": nil,
	}
	if stats.LatestMigratedAt != nil {
		data["latest_migrated_at"] = stats.LatestMigratedAt
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": data})
}

// AdminListMediaEpisodes GET /api/v1/admin/media/episodes
func (a *API) AdminListMediaEpisodes(c *gin.Context) {
	q, msg, ok := parseAdminMediaListQuery(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": msg})
		return
	}
	repo := a.adminMediaRepo()
	if repo == nil {
		c.JSON(http.StatusOK, gin.H{
			"ok": true, "data": []AdminMediaEpisodeDTO{},
			"total": 0, "limit": q.Limit, "offset": q.Offset, "has_more": false,
		})
		return
	}
	total, err := repo.CountAdminMediaEpisodesFiltered(q.MigrationStatus, q.StorageProvider, q.Query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载媒体列表"})
		return
	}
	rows, err := repo.ListAdminMediaEpisodesFiltered(q.MigrationStatus, q.StorageProvider, q.Query, q.Limit, q.Offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载媒体列表"})
		return
	}
	out := make([]AdminMediaEpisodeDTO, 0, len(rows))
	for _, ep := range rows {
		out = append(out, adminMediaEpisodeDTO(ep, false))
	}
	c.JSON(http.StatusOK, gin.H{
		"ok": true, "data": out,
		"total": total, "limit": q.Limit, "offset": q.Offset,
		"has_more": q.Offset+len(out) < total,
	})
}

// AdminGetMediaEpisode GET /api/v1/admin/media/episodes/:id
func (a *API) AdminGetMediaEpisode(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	repo := a.adminMediaRepo()
	if repo == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	row, err := repo.GetAdminMediaEpisodeByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载媒体详情"})
		return
	}
	if row == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": adminMediaEpisodeDTO(*row, true)})
}
