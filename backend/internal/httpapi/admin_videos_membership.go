package httpapi

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type adminSetMembershipReq struct {
	MembershipRequired bool `json:"membership_required"`
}

// AdminSetVideoMembership PATCH /api/v1/admin/videos/:id/membership
// When enabled, the video moves into VIP蓝光影院 (membership box) and is hidden
// from non-member public discovery.
func (a *API) AdminSetVideoMembership(c *gin.Context) {
	if a.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "store unavailable"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	var req adminSetMembershipReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid json"})
		return
	}
	v, err := a.Store.GetVideo(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载视频"})
		return
	}
	if v == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
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
	if err := a.Store.SetVideoMembershipRequired(id, req.MembershipRequired, vipCategoryID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法更新会员设置"})
		return
	}
	updated, err := a.Store.GetVideo(id)
	if err != nil || updated == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"id": id, "membership_required": req.MembershipRequired}})
		return
	}
	shaped := a.withCoverURL(*updated)
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": adminVideoDetailFrom(shaped)})
}
