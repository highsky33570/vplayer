package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/store"
)

type adminUpdateCategoryReq struct {
	Name     *string `json:"name"`
	Sort     *int    `json:"sort"`
	IsActive *bool   `json:"is_active"`
	ParentID *uint64 `json:"parent_id"`
}

type adminCreateCategoryReq struct {
	Name     string  `json:"name"`
	ParentID uint64  `json:"parent_id"`
	Sort     *int    `json:"sort"`
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
	if req.ParentID != nil {
		newParent := *req.ParentID
		if newParent == 0 || newParent == id {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid parent_id"})
			return
		}
		curParent, ok, err := a.Store.CategoryParentID(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法更新分类"})
			return
		}
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
			return
		}
		if curParent == nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "主分类不能移动"})
			return
		}
		targetParent, tok, err := a.Store.CategoryParentID(newParent)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法更新分类"})
			return
		}
		if !tok || targetParent != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "只能移动到主分类下"})
			return
		}
		upd.ParentID = &newParent
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

// AdminCreateCategory POST /api/v1/admin/categories
// Creates a subcategory under a top-level main category.
func (a *API) AdminCreateCategory(c *gin.Context) {
	if a.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "store unavailable"})
		return
	}
	var req adminCreateCategoryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid json"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "name is required"})
		return
	}
	if utf8.RuneCountInString(name) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "name is too long"})
		return
	}
	if req.ParentID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "parent_id is required"})
		return
	}
	parentParent, ok, err := a.Store.CategoryParentID(req.ParentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法创建分类"})
		return
	}
	if !ok || parentParent != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "只能在主分类下添加子分类"})
		return
	}
	sort := 0
	if req.Sort != nil {
		sort = *req.Sort
	}
	uniq := fmt.Sprintf("%d-%d", req.ParentID, time.Now().UnixNano())
	if len(uniq) > 48 {
		uniq = uniq[:48]
	}
	slug := "admin-" + uniq
	id, err := a.Store.CreateAdminCategory(store.AdminCategoryCreate{
		Name:     name,
		ParentID: req.ParentID,
		Sort:     sort,
		Slug:     slug,
		SourceID: uniq,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法创建分类"})
		return
	}
	row, err := a.Store.GetAdminCategoryByID(id)
	if err != nil || row == nil {
		c.JSON(http.StatusCreated, gin.H{"ok": true, "data": gin.H{"id": id, "name": name}})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true, "data": adminCategoryDTO(*row)})
}

// AdminDeleteCategory DELETE /api/v1/admin/categories/:id
// Soft-deactivates a subcategory (mains cannot be deleted).
func (a *API) AdminDeleteCategory(c *gin.Context) {
	if a.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "store unavailable"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	parent, ok, err := a.Store.CategoryParentID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法删除分类"})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	if parent == nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "主分类不能删除"})
		return
	}
	ok, err = a.Store.DeactivateAdminSubcategory(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法删除分类"})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"id": id, "is_active": false}})
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
