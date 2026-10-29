package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/store"
)

const adminLocalSourceSystem = "admin"

type adminCreateVideoReq struct {
	Title       string `json:"title"`
	PlaybackURL string `json:"playback_url"`
	CoverURL    string `json:"cover_url"`
	Description string `json:"description"`
	// AsVIP places the title in VIP蓝光影院 (default true for this create path).
	AsVIP *bool `json:"as_vip"`
	// CategoryID optional override when AsVIP is false.
	CategoryID uint64 `json:"category_id"`
}

func adminCreateAsVIP(req adminCreateVideoReq) bool {
	if req.AsVIP == nil {
		return false
	}
	return *req.AsVIP
}

func newAdminSourceID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "admin-" + hex.EncodeToString(b[:]), nil
}

func validateAdminPlaybackURL(raw string) (string, string) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return "", "playback_url is required"
	}
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "playback_url must be an absolute http(s) URL"
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return u, ""
	default:
		return "", "playback_url must be http or https"
	}
}

func validateOptionalHTTPURL(raw, field string) (string, string) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return "", ""
	}
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", field + " must be an absolute http(s) URL"
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return u, ""
	default:
		return "", field + " must be http or https"
	}
}

func playbackSourceForURL(u string) string {
	lower := strings.ToLower(u)
	if strings.Contains(lower, ".m3u8") {
		return "m3u8"
	}
	return "direct"
}

// AdminCreateVideo POST /api/v1/admin/videos
// Creates a local (non-olehdtv) catalog title + episode so it can appear in VIP
// without OLEHDTV membership ingest.
func (a *API) AdminCreateVideo(c *gin.Context) {
	if a.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "store unavailable"})
		return
	}
	var req adminCreateVideoReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid json"})
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "title is required"})
		return
	}
	playURL, msg := validateAdminPlaybackURL(req.PlaybackURL)
	if msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": msg})
		return
	}
	coverURL, msg := validateOptionalHTTPURL(req.CoverURL, "cover_url")
	if msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": msg})
		return
	}

	if req.CategoryID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "category_id is required"})
		return
	}
	cat, err := a.Store.GetCategoryByID(req.CategoryID)
	if err != nil || cat == nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid category_id"})
		return
	}
	browse := store.CategoryBrowseFromRow(req.CategoryID, cat)
	if browse.FailClosed || browse.CanonicalCategoryID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid category_id"})
		return
	}
	categoryID := browse.CanonicalCategoryID
	sourceCategoryID := strings.TrimSpace(cat.SourceID)
	if cat.ParentID != nil && *cat.ParentID > 0 {
		if parent, perr := a.Store.GetCategoryByID(*cat.ParentID); perr == nil && parent != nil {
			sourceCategoryID = strings.TrimSpace(parent.SourceID)
		}
	}

	sourceID, err := newAdminSourceID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法生成来源 ID"})
		return
	}
	playSrc := playbackSourceForURL(playURL)
	videoID, err := a.Store.InsertVideoCatalog(store.VideoUpsert{
		SourceSystem:     adminLocalSourceSystem,
		SourceID:         sourceID,
		CategoryID:       categoryID,
		SourceCategoryID: sourceCategoryID,
		Title:            title,
		Description:      strings.TrimSpace(req.Description),
		PosterSourceURL:  coverURL,
		PlaybackSource:   playSrc,
		PlaySID:          1,
		PlayNID:          1,
		PlaybackURL:      playURL,
		IsActive:         true,
		Status:           "ready",
		SyncStatus:       "ok",
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法创建视频"})
		return
	}
	if browse.LeafSourceID != "" {
		if err := a.Store.SetVideoLeafSourceID(videoID, browse.LeafSourceID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "视频已创建但子分类写入失败"})
			return
		}
	}
	if err := a.Store.InsertEpisodeCatalog(store.EpisodeUpsert{
		VideoID:        videoID,
		SourceSystem:   adminLocalSourceSystem,
		SourceVideoID:  sourceID,
		SID:            1,
		NID:            1,
		Title:          "正片",
		PlaybackSource: playSrc,
		PlaybackURL:    playURL,
		PlaybackStatus: "ok",
		IsActive:       true,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "视频已创建但剧集写入失败"})
		return
	}
	// VIP channel or explicit as_vip => membership-gated.
	vipChannel := strings.TrimSpace(cat.Slug) == "vip-bluray"
	if !vipChannel && cat.ParentID != nil && *cat.ParentID > 0 {
		if parent, perr := a.Store.GetCategoryByID(*cat.ParentID); perr == nil && parent != nil {
			vipChannel = strings.TrimSpace(parent.Slug) == "vip-bluray"
		}
	}
	if adminCreateAsVIP(req) || vipChannel {
		_ = a.Store.SetVideoMembershipRequired(videoID, true, categoryID)
	}

	v, err := a.Store.GetVideo(videoID)
	if err != nil || v == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"id": videoID}})
		return
	}
	shaped := a.withCoverURL(*v)
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": adminVideoDetailFrom(shaped)})
}
