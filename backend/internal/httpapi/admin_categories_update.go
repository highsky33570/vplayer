package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/store"
)

type adminUpdateCategoryReq struct {
	Name     *string `json:"name"`
	Sort     *int    `json:"sort"`
	IsActive *bool   `json:"is_active"`
}

// AdminUpdateCategory PATCH /api/v1/admin/categories/:id
func (a *API) AdminUpdateCategory(c *gin.Context) {
	if a.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "store unavailable"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	var req adminUpdateCategoryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid json"})
		return
	}
	upd := store.AdminCategoryUpdate{Sort: req.Sort, IsActive: req.IsActive}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "name is required"})
			return
		}
		if utf8.RuneCountInString(name) > 64 {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "name is too long"})
			return
		}
		upd.Name = &name
	}
	found, err := a.Store.UpdateAdminCategory(id, upd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法更新分类"})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	row, err := a.Store.GetAdminCategoryByID(id)
	if err != nil || row == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"id": id}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": adminCategoryDTO(*row)})
}

// AdminCleanupDuplicateCategories POST /api/v1/admin/categories/cleanup-duplicates
// Merges same-named categories into the one with the most videos (tie: lowest id).
func (a *API) AdminCleanupDuplicateCategories(c *gin.Context) {
	if a.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "store unavailable"})
		return
	}
	merged, deactivated, moved, err := a.Store.CleanupDuplicateCategories()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法清理重复分类"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{
		"merged": merged, "deactivated": deactivated, "videos_moved": moved,
	}})
}
