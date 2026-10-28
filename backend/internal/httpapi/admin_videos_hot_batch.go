package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type adminBatchHotReq struct {
	IDs   []uint64 `json:"ids"`
	IsHot bool     `json:"is_hot"`
}

// AdminBatchSetVideoHot PATCH /api/v1/admin/videos/hot
// Sets is_hot for the given video ids (current-page batch from admin UI).
func (a *API) AdminBatchSetVideoHot(c *gin.Context) {
	if a.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "store unavailable"})
		return
	}
	var req adminBatchHotReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid json"})
		return
	}
	ids := make([]uint64, 0, len(req.IDs))
	seen := map[uint64]bool{}
	for _, id := range req.IDs {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "ids required"})
		return
	}
	if len(ids) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "too many ids"})
		return
	}
	n, err := a.Store.SetVideosIsHot(ids, req.IsHot)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法批量更新热门推荐"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok": true,
		"data": gin.H{
			"updated": n,
			"is_hot":  req.IsHot,
			"ids":     ids,
		},
	})
}
