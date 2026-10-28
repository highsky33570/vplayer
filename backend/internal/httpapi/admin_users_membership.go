package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type adminSetUserMemberReq struct {
	IsMember bool `json:"is_member"`
	// MembershipExpiresAt is an ISO 8601 / RFC3339 instant (or YYYY-MM-DD, end of day UTC).
	// Only used when is_member is true; omitted/null/"" means lifetime unless plan_slug is set.
	MembershipExpiresAt *string `json:"membership_expires_at"`
	// PlanSlug grants now + plan duration_days when is_member is true and no explicit expiry is given.
	PlanSlug string `json:"plan_slug"`
}

// parseAdminExpiresAt parses an admin-supplied membership expiry. nil/empty -> (nil, "").
func parseAdminExpiresAt(raw *string) (*time.Time, string) {
	if raw == nil {
		return nil, ""
	}
	s := strings.TrimSpace(*raw)
	if s == "" {
		return nil, ""
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		t = t.UTC()
		return &t, ""
	}
	if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
		return &t, ""
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		t = t.Add(24*time.Hour - time.Second)
		return &t, ""
	}
	return nil, "invalid membership_expires_at"
}

// AdminSetUserMembership PATCH /api/v1/admin/users/:id/membership
// Body: {is_member, membership_expires_at?, plan_slug?}. is_member=false clears expiry;
// is_member=true without an expiry (or plan) is a lifetime grant.
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
	expires, msg := parseAdminExpiresAt(req.MembershipExpiresAt)
	if msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": msg})
		return
	}
	if req.IsMember && expires == nil && strings.TrimSpace(req.PlanSlug) != "" {
		plan, err := a.Store.GetMembershipPlanBySlugAny(req.PlanSlug)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载套餐"})
			return
		}
		if plan == nil || plan.DurationDays <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid plan_slug"})
			return
		}
		t := time.Now().UTC().Add(time.Duration(plan.DurationDays) * 24 * time.Hour)
		expires = &t
	}
	if req.IsMember && expires != nil && !expires.After(time.Now()) {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "membership_expires_at must be in the future"})
		return
	}
	if !req.IsMember {
		expires = nil
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
	if err := a.Store.SetUserMembership(id, req.IsMember, expires); err != nil {
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
