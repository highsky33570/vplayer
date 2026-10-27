package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/model"
)

// adminVideoRepo is the store surface used by admin video read APIs.
type adminVideoRepo interface {
	ListAdminVideosFiltered(categoryID uint64, query string, active *bool, limit, offset int) ([]model.Video, error)
	CountAdminVideosFiltered(categoryID uint64, query string, active *bool) (int, error)
	GetVideo(id uint64) (*model.Video, error)
}

func (a *API) adminVideoRepo() adminVideoRepo {
	if a.AdminVideos != nil {
		return a.AdminVideos
	}
	if a.Store != nil {
		return a.Store
	}
	return nil
}

// AdminVideoListItem is the allowlisted list DTO (never serialize model.Video).
type AdminVideoListItem struct {
	ID                 uint64     `json:"id"`
	Title              string     `json:"title"`
	CategoryID         uint64     `json:"category_id"`
	Status             string     `json:"status"`
	IsActive           bool       `json:"is_active"`
	MembershipRequired bool       `json:"membership_required"`
	SourceSystem       string     `json:"source_system"`
	SourceID           string     `json:"source_id"`
	Year               string     `json:"year"`
	SyncStatus         string     `json:"sync_status"`
	CreatedAt          time.Time  `json:"created_at"`
	LastSyncedAt       *time.Time `json:"last_synced_at,omitempty"`
	ViewCount          int64      `json:"view_count"`
	DurationSec        int        `json:"duration_sec"`
	CoverURL           string     `json:"cover_url"`
}

// AdminVideoDetail is the allowlisted detail DTO.
type AdminVideoDetail struct {
	AdminVideoListItem
	Description      string     `json:"description"`
	Director         string     `json:"director"`
	Actors           string     `json:"actors"`
	Rating           float64    `json:"rating"`
	SourceCategoryID string     `json:"source_category_id"`
	SourceUpdatedAt  *time.Time `json:"source_updated_at,omitempty"`
	PlaybackSource   string     `json:"playback_source"`
	PlaySID          int        `json:"play_sid"`
	PlayNID          int        `json:"play_nid"`
}

func adminVideoListItemFrom(v model.Video) AdminVideoListItem {
	return AdminVideoListItem{
		ID:                 v.ID,
		Title:              v.Title,
		CategoryID:         v.CategoryID,
		Status:             v.Status,
		IsActive:           v.IsActive,
		MembershipRequired: v.MembershipRequired,
		SourceSystem:       v.SourceSystem,
		SourceID:           v.SourceID,
		Year:               v.Year,
		SyncStatus:         v.SyncStatus,
		CreatedAt:          v.CreatedAt,
		LastSyncedAt:       v.LastSyncedAt,
		ViewCount:          v.ViewCount,
		DurationSec:        v.DurationSec,
		CoverURL:           v.CoverURL,
	}
}

func adminVideoDetailFrom(v model.Video) AdminVideoDetail {
	return AdminVideoDetail{
		AdminVideoListItem: adminVideoListItemFrom(v),
		Description:        v.Description,
		Director:           v.Director,
		Actors:             v.Actors,
		Rating:             v.Rating,
		SourceCategoryID:   v.SourceCategoryID,
		SourceUpdatedAt:    v.SourceUpdatedAt,
		PlaybackSource:     v.PlaybackSource,
		PlaySID:            v.PlaySID,
		PlayNID:            v.PlayNID,
	}
}

type adminVideoListQuery struct {
	Limit      int
	Offset     int
	Query      string
	CategoryID uint64
	Active     *bool
}

func parseAdminVideoListQuery(c *gin.Context) (adminVideoListQuery, string, bool) {
	out := adminVideoListQuery{Limit: 24, Offset: 0}
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

	if raw := strings.TrimSpace(c.Query("category_id")); raw != "" {
		n, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || n == 0 {
			return out, "invalid category_id", false
		}
		out.CategoryID = n
	}

	if raw := strings.TrimSpace(c.Query("is_active")); raw != "" {
		switch raw {
		case "1", "true", "True":
			v := true
			out.Active = &v
		case "0", "false", "False":
			v := false
			out.Active = &v
		default:
			return out, "invalid is_active", false
		}
	}

	out.Query = strings.TrimSpace(c.Query("q"))
	return out, "", true
}

// AdminListVideos GET /api/v1/admin/videos
func (a *API) AdminListVideos(c *gin.Context) {
	q, msg, ok := parseAdminVideoListQuery(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": msg})
		return
	}
	repo := a.adminVideoRepo()
	if repo == nil {
		c.JSON(http.StatusOK, gin.H{
			"ok": true, "data": []AdminVideoListItem{},
			"total": 0, "limit": q.Limit, "offset": q.Offset, "has_more": false,
		})
		return
	}
	total, err := repo.CountAdminVideosFiltered(q.CategoryID, q.Query, q.Active)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载视频列表"})
		return
	}
	rows, err := repo.ListAdminVideosFiltered(q.CategoryID, q.Query, q.Active, q.Limit, q.Offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载视频列表"})
		return
	}
	out := make([]AdminVideoListItem, 0, len(rows))
	for _, v := range rows {
		shaped := a.withCoverURL(v)
		out = append(out, adminVideoListItemFrom(shaped))
	}
	c.JSON(http.StatusOK, gin.H{
		"ok": true, "data": out,
		"total": total, "limit": q.Limit, "offset": q.Offset,
		"has_more": q.Offset+len(out) < total,
	})
}

// AdminGetVideo GET /api/v1/admin/videos/:id
func (a *API) AdminGetVideo(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	repo := a.adminVideoRepo()
	if repo == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	v, err := repo.GetVideo(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载视频"})
		return
	}
	if v == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	shaped := a.withCoverURL(*v)
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": adminVideoDetailFrom(shaped)})
}
