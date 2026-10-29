package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type adminBatchMembershipReq struct {
	IDs                []uint64 `json:"ids"`
	MembershipRequired bool     `json:"membership_required"`
}

// AdminBatchSetVideoMembership PATCH /api/v1/admin/videos/membership
// Sets membership_required for the given video ids (admin batch bar).
// Enabling also moves titles into VIP蓝光影院 when that category exists.
func (a *API) AdminBatchSetVideoMembership(c *gin.Context) {
	if a.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "store unavailable"})
		return
	}
	var req adminBatchMembershipReq
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

	var vipCategoryID uint64
	if req.MembershipRequired {
		vip, gerr := a.Store.GetCategoryBySlug("vip-bluray")
		if gerr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法解析 VIP 分类"})
			return
		}
		if vip == nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "VIP 分类不存在"})
			return
		}
		vipCategoryID = vip.ID
	}

	n, err := a.Store.SetVideosMembershipRequired(ids, req.MembershipRequired, vipCategoryID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法批量更新 VIP 设置"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok": true,
		"data": gin.H{
			"updated":              n,
			"membership_required":  req.MembershipRequired,
			"ids":                  ids,
		},
	})
}
