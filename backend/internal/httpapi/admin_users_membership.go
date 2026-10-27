package httpapi

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type adminSetUserMemberReq struct {
	IsMember bool `json:"is_member"`
}

// AdminSetUserMembership PATCH /api/v1/admin/users/:id/membership
func (a *API) AdminSetUserMembership(c *gin.Context) {
	if a.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "store unavailable"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	var req adminSetUserMemberReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid json"})
		return
	}
	existing, err := a.Store.GetAdminUserByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载用户"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	if err := a.Store.SetUserIsMember(id, req.IsMember); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法更新会员状态"})
		return
	}
	updated, err := a.Store.GetAdminUserByID(id)
	if err != nil || updated == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"id": id, "is_member": req.IsMember}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": adminUserDTO(*updated)})
}
