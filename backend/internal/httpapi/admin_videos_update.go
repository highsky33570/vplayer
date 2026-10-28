package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/store"
)

type adminUpdateVideoReq struct {
	Title       *string         `json:"title"`
	Description *string         `json:"description"`
	Year        json.RawMessage `json:"year"`
	Director    *string         `json:"director"`
	Actors      *string         `json:"actors"`
	CategoryID  *uint64         `json:"category_id"`
	IsActive    *bool           `json:"is_active"`
	Status      *string         `json:"status"`
	// PosterSourceURL / CoverURL both set poster_source_url (PosterSourceURL wins).
	PosterSourceURL    *string `json:"poster_source_url"`
	CoverURL           *string `json:"cover_url"`
	MembershipRequired *bool   `json:"membership_required"`
	IsHot              *bool   `json:"is_hot"`
}

var adminVideoStatuses = map[string]bool{"draft": true, "ready": true, "disabled": true, "demo": true}

// adminYearString accepts a JSON string or number for year. ok=false means invalid type.
func adminYearString(raw json.RawMessage) (val *string, ok bool) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return nil, true
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		str = strings.TrimSpace(str)
		return &str, true
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		out := n.String()
		return &out, true
	}
	return nil, false
}

// AdminUpdateVideo PATCH /api/v1/admin/videos/:id
// Updates editable catalog fields only; sync / source / playback keys are untouched.
func (a *API) AdminUpdateVideo(c *gin.Context) {
	if a.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "store unavailable"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	var req adminUpdateVideoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid json"})
		return
	}

	upd := store.AdminVideoUpdate{
		Description:        req.Description,
		Director:           req.Director,
		Actors:             req.Actors,
		IsActive:           req.IsActive,
		MembershipRequired: req.MembershipRequired,
		IsHot:              req.IsHot,
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "title is required"})
			return
		}
		if utf8.RuneCountInString(title) > 255 {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "title is too long"})
			return
		}
		upd.Title = &title
	}
	year, ok := adminYearString(req.Year)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid year"})
		return
	}
	upd.Year = year
	if req.Status != nil {
		status := strings.ToLower(strings.TrimSpace(*req.Status))
		if !adminVideoStatuses[status] {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid status"})
			return
		}
		upd.Status = &status
	}
	poster := req.PosterSourceURL
	if poster == nil {
		poster = req.CoverURL
	}
	if poster != nil {
		u, msg := validateOptionalHTTPURL(*poster, "poster_source_url")
		if msg != "" {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": msg})
			return
		}
		upd.PosterSourceURL = &u
	}
	if req.CategoryID != nil {
		if *req.CategoryID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid category_id"})
			return
		}
		cat, err := a.Store.GetCategoryByID(*req.CategoryID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载分类"})
			return
		}
		if cat == nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid category_id"})
			return
		}
		upd.CategoryID = req.CategoryID
	} else if req.MembershipRequired != nil && *req.MembershipRequired {
		// Same placement as PATCH .../membership: membership titles live in the VIP box.
		vip, gerr := a.Store.GetCategoryBySlug("vip-bluray")
		if gerr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法解析 VIP 分类"})
			return
		}
		if vip == nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "VIP 分类不存在"})
			return
		}
		upd.CategoryID = &vip.ID
	}

	found, err := a.Store.AdminUpdateVideoFields(id, upd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法更新视频"})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	v, err := a.Store.GetVideo(id)
	if err != nil || v == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"id": id}})
		return
	}
	shaped := a.withCoverURL(*v)
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": adminVideoDetailFrom(shaped)})
}
