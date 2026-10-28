package httpapi

import (
	"errors"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/store"
)

var adminPlanSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// AdminMembershipPlanDTO is the allowlisted admin plan response.
type AdminMembershipPlanDTO struct {
	ID           uint64  `json:"id"`
	Slug         string  `json:"slug"`
	Name         string  `json:"name"`
	PriceUSD     float64 `json:"price_usd"`
	DurationDays int     `json:"duration_days"`
	Sort         int     `json:"sort"`
	IsActive     bool    `json:"is_active"`
}

func adminMembershipPlanDTO(p store.AdminMembershipPlan) AdminMembershipPlanDTO {
	return AdminMembershipPlanDTO{
		ID: p.ID, Slug: p.Slug, Name: p.Name, PriceUSD: p.PriceUSD,
		DurationDays: p.DurationDays, Sort: p.Sort, IsActive: p.IsActive,
	}
}

type adminMembershipPlanReq struct {
	Slug         *string  `json:"slug"`
	Name         *string  `json:"name"`
	PriceUSD     *float64 `json:"price_usd"`
	DurationDays *int     `json:"duration_days"`
	Sort         *int     `json:"sort"`
	IsActive     *bool    `json:"is_active"`
}

func validAdminPlanPrice(p float64) bool {
	return !math.IsNaN(p) && !math.IsInf(p, 0) && p >= 0 && p < 100000000
}

func validAdminPlanName(raw string) (string, string) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", "name is required"
	}
	if utf8.RuneCountInString(name) > 64 {
		return "", "name is too long"
	}
	return name, ""
}

// AdminListMembershipPlans GET /api/v1/admin/membership/plans (includes inactive plans)
func (a *API) AdminListMembershipPlans(c *gin.Context) {
	if a.Store == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": []AdminMembershipPlanDTO{}})
		return
	}
	rows, err := a.Store.ListAllMembershipPlans()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载套餐"})
		return
	}
	out := make([]AdminMembershipPlanDTO, 0, len(rows))
	for _, p := range rows {
		out = append(out, adminMembershipPlanDTO(p))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": out})
}

// AdminUpdateMembershipPlan PATCH /api/v1/admin/membership/plans/:id (slug is immutable)
func (a *API) AdminUpdateMembershipPlan(c *gin.Context) {
	if a.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "store unavailable"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	var req adminMembershipPlanReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid json"})
		return
	}
	upd := store.AdminMembershipPlanUpdate{
		PriceUSD: req.PriceUSD, DurationDays: req.DurationDays, Sort: req.Sort, IsActive: req.IsActive,
	}
	if req.Name != nil {
		name, msg := validAdminPlanName(*req.Name)
		if msg != "" {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": msg})
			return
		}
		upd.Name = &name
	}
	if req.PriceUSD != nil && !validAdminPlanPrice(*req.PriceUSD) {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid price_usd"})
		return
	}
	if req.DurationDays != nil && *req.DurationDays <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "duration_days must be positive"})
		return
	}
	found, err := a.Store.UpdateMembershipPlan(id, upd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法更新套餐"})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	row, err := a.Store.GetMembershipPlanByID(id)
	if err != nil || row == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"id": id}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": adminMembershipPlanDTO(*row)})
}

// AdminCreateMembershipPlan POST /api/v1/admin/membership/plans
func (a *API) AdminCreateMembershipPlan(c *gin.Context) {
	if a.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "store unavailable"})
		return
	}
	var req adminMembershipPlanReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid json"})
		return
	}
	if req.Slug == nil || !adminPlanSlugRe.MatchString(strings.TrimSpace(*req.Slug)) {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid slug (a-z, 0-9, _ or -, max 32)"})
		return
	}
	if req.Name == nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "name is required"})
		return
	}
	name, msg := validAdminPlanName(*req.Name)
	if msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": msg})
		return
	}
	if req.PriceUSD == nil || !validAdminPlanPrice(*req.PriceUSD) {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid price_usd"})
		return
	}
	if req.DurationDays == nil || *req.DurationDays <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "duration_days must be positive"})
		return
	}
	plan := store.AdminMembershipPlan{
		Slug: strings.TrimSpace(*req.Slug), Name: name, PriceUSD: *req.PriceUSD,
		DurationDays: *req.DurationDays, IsActive: true,
	}
	if req.Sort != nil {
		plan.Sort = *req.Sort
	}
	if req.IsActive != nil {
		plan.IsActive = *req.IsActive
	}
	id, err := a.Store.CreateMembershipPlan(plan)
	if err != nil {
		if errors.Is(err, store.ErrMembershipPlanExists) {
			c.JSON(http.StatusConflict, gin.H{"ok": false, "message": "slug already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法创建套餐"})
		return
	}
	plan.ID = id
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": adminMembershipPlanDTO(plan)})
}
