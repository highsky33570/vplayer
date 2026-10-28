package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/store"
)

type adminUpdateUserReq struct {
	Nickname            *string `json:"nickname"`
	IsAdmin             *bool   `json:"is_admin"`
	IsMember            *bool   `json:"is_member"`
	MembershipExpiresAt *string `json:"membership_expires_at"`
}

// AdminUpdateUser PATCH /api/v1/admin/users/:id
// nickname is required (trimmed, non-empty, <=64 chars). is_admin and is_member are optional;
// membership_expires_at only applies when is_member is true (omitted = lifetime).
func (a *API) AdminUpdateUser(c *gin.Context) {
	if a.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "store unavailable"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	var req adminUpdateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid json"})
		return
	}
	if req.Nickname == nil || strings.TrimSpace(*req.Nickname) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "nickname is required"})
		return
	}
	nickname := strings.TrimSpace(*req.Nickname)
	if utf8.RuneCountInString(nickname) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "nickname is too long"})
		return
	}
	upd := store.AdminUserUpdate{Nickname: nickname, IsAdmin: req.IsAdmin}
	if req.IsMember != nil {
		upd.SetMembership = true
		upd.IsMember = *req.IsMember
		if *req.IsMember {
			expires, msg := parseAdminExpiresAt(req.MembershipExpiresAt)
			if msg != "" {
				c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": msg})
				return
			}
			upd.MembershipExpiresAt = expires
		}
	}
	// Admins must not demote themselves (avoids accidental lock-out).
	if req.IsAdmin != nil && !*req.IsAdmin {
		if uid, ok := authUserIDFromContext(c); ok && uid == id {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "cannot remove your own admin role"})
			return
		}
	}
	if err := a.Store.UpdateAdminUser(id, upd); err != nil {
		switch {
		case errors.Is(err, store.ErrUserNotFound):
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		case errors.Is(err, store.ErrLastAdmin):
			c.JSON(http.StatusConflict, gin.H{"ok": false, "message": "cannot remove the last admin"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法更新用户"})
		}
		return
	}
	updated, err := a.Store.GetAdminUserByID(id)
	if err != nil || updated == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"id": id}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": adminUserDTO(*updated)})
}

// AdminDeleteUser DELETE /api/v1/admin/users/:id
// Refuses deleting yourself and deleting the last admin.
func (a *API) AdminDeleteUser(c *gin.Context) {
	if a.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "store unavailable"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	if uid, ok := authUserIDFromContext(c); ok && uid == id {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "cannot delete your own account"})
		return
	}
	if err := a.Store.DeleteAdminUser(id); err != nil {
		switch {
		case errors.Is(err, store.ErrUserNotFound):
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		case errors.Is(err, store.ErrLastAdmin):
			c.JSON(http.StatusConflict, gin.H{"ok": false, "message": "cannot delete the last admin"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法删除用户"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"id": id, "deleted": true}})
}
