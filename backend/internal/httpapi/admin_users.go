package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/store"
)

// adminUserRepo is the store surface for admin user reads.
type adminUserRepo interface {
	ListAdminUsersFiltered(query string, isAdmin *bool, limit, offset int) ([]store.AdminUser, error)
	CountAdminUsersFiltered(query string, isAdmin *bool) (int, error)
	GetAdminUserByID(id uint64) (*store.AdminUser, error)
}

func (a *API) adminUserRepo() adminUserRepo {
	if a.AdminUsers != nil {
		return a.AdminUsers
	}
	if a.Store != nil {
		return a.Store
	}
	return nil
}

// AdminUserDTO is the allowlisted admin user response (never password_hash / tokens).
type AdminUserDTO struct {
	ID                  uint64     `json:"id"`
	Email               string     `json:"email"`
	Nickname            string     `json:"nickname"`
	IsAdmin             bool       `json:"is_admin"`
	IsMember            bool       `json:"is_member"`
	MembershipExpiresAt *time.Time `json:"membership_expires_at"`
	LatestPlanSlug      string     `json:"latest_plan_slug"`
	LatestPlanName      string     `json:"latest_plan_name"`
	CreatedAt           time.Time  `json:"created_at"`
}

func adminUserDTO(u store.AdminUser) AdminUserDTO {
	return AdminUserDTO{
		ID:                  u.ID,
		Email:               u.Email,
		Nickname:            u.Nickname,
		IsAdmin:             u.IsAdmin,
		IsMember:            u.IsMember,
		MembershipExpiresAt: u.MembershipExpiresAt,
		LatestPlanSlug:      u.LatestPlanSlug,
		LatestPlanName:      u.LatestPlanName,
		CreatedAt:           u.CreatedAt,
	}
}

type adminUserListQuery struct {
	Limit   int
	Offset  int
	Query   string
	IsAdmin *bool
}

func parseAdminUserListQuery(c *gin.Context) (adminUserListQuery, string, bool) {
	out := adminUserListQuery{Limit: 24, Offset: 0}
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

	if raw := strings.TrimSpace(c.Query("is_admin")); raw != "" {
		switch raw {
		case "1", "true", "True":
			v := true
			out.IsAdmin = &v
		case "0", "false", "False":
			v := false
			out.IsAdmin = &v
		default:
			return out, "invalid is_admin", false
		}
	}

	out.Query = strings.TrimSpace(c.Query("q"))
	return out, "", true
}

// AdminListUsers GET /api/v1/admin/users
func (a *API) AdminListUsers(c *gin.Context) {
	q, msg, ok := parseAdminUserListQuery(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": msg})
		return
	}
	repo := a.adminUserRepo()
	if repo == nil {
		c.JSON(http.StatusOK, gin.H{
			"ok": true, "data": []AdminUserDTO{},
			"total": 0, "limit": q.Limit, "offset": q.Offset, "has_more": false,
		})
		return
	}
	total, err := repo.CountAdminUsersFiltered(q.Query, q.IsAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载用户列表"})
		return
	}
	rows, err := repo.ListAdminUsersFiltered(q.Query, q.IsAdmin, q.Limit, q.Offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载用户列表"})
		return
	}
	out := make([]AdminUserDTO, 0, len(rows))
	for _, u := range rows {
		out = append(out, adminUserDTO(u))
	}
	c.JSON(http.StatusOK, gin.H{
		"ok": true, "data": out,
		"total": total, "limit": q.Limit, "offset": q.Offset,
		"has_more": q.Offset+len(out) < total,
	})
}

// AdminGetUser GET /api/v1/admin/users/:id
func (a *API) AdminGetUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	repo := a.adminUserRepo()
	if repo == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	row, err := repo.GetAdminUserByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载用户"})
		return
	}
	if row == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": adminUserDTO(*row)})
}
