package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/store"
)

// adminCategoryRepo is the store surface for admin category reads.
type adminCategoryRepo interface {
	ListAdminCategories() ([]store.AdminCategory, error)
	GetAdminCategoryByID(id uint64) (*store.AdminCategory, error)
}

func (a *API) adminCategoryRepo() adminCategoryRepo {
	if a.AdminCategories != nil {
		return a.AdminCategories
	}
	if a.Store != nil {
		return a.Store
	}
	return nil
}

// AdminCategoryDTO is the allowlisted admin category response (no sync_error).
type AdminCategoryDTO struct {
	ID           uint64     `json:"id"`
	Name         string     `json:"name"`
	Slug         string     `json:"slug"`
	Sort         int        `json:"sort"`
	IsActive     bool       `json:"is_active"`
	SourceSystem string     `json:"source_system"`
	SourceID     string     `json:"source_id"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
	SyncStatus   string     `json:"sync_status"`
	HasError     bool       `json:"has_error"`
	CreatedAt    time.Time  `json:"created_at"`
	VideoCount   int        `json:"video_count"`
}

func adminCategoryDTO(c store.AdminCategory) AdminCategoryDTO {
	return AdminCategoryDTO{
		ID:           c.ID,
		Name:         c.Name,
		Slug:         c.Slug,
		Sort:         c.Sort,
		IsActive:     c.IsActive,
		SourceSystem: c.SourceSystem,
		SourceID:     c.SourceID,
		LastSyncedAt: c.LastSyncedAt,
		SyncStatus:   c.SyncStatus,
		HasError:     c.HasError,
		CreatedAt:    c.CreatedAt,
		VideoCount:   c.VideoCount,
	}
}

// AdminListCategories GET /api/v1/admin/categories
func (a *API) AdminListCategories(c *gin.Context) {
	repo := a.adminCategoryRepo()
	if repo == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": []AdminCategoryDTO{}})
		return
	}
	rows, err := repo.ListAdminCategories()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载分类列表"})
		return
	}
	out := make([]AdminCategoryDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, adminCategoryDTO(row))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": out})
}

// AdminGetCategory GET /api/v1/admin/categories/:id
func (a *API) AdminGetCategory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	repo := a.adminCategoryRepo()
	if repo == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	row, err := repo.GetAdminCategoryByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载分类"})
		return
	}
	if row == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": adminCategoryDTO(*row)})
}
