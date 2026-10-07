package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/play"
	"github.com/tycdn/vplayer/internal/scheduler"
	"github.com/tycdn/vplayer/internal/store"
)

type API struct {
	Cfg        config.Config
	Store      *store.MySQL
	Users      store.UserRepo
	Resolver   *play.Resolver
	SyncRunner *scheduler.OleHdTvRunner
	MediaBase  string
}

func (a *API) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true, "service": "vplayer-api"})
}

func (a *API) ListCategories(c *gin.Context) {
	if a.Store == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": demoCategories()})
		return
	}
	rows, err := a.Store.ListCategories()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	if len(rows) == 0 {
		rows = demoCategories()
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": rows})
}

func (a *API) ListVideos(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "24"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	catID, _ := strconv.ParseUint(c.Query("category_id"), 10, 64)
	q := strings.TrimSpace(c.Query("q"))
	if limit <= 0 {
		limit = 24
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	if a.Store == nil {
		rows := demoVideos()
		if q != "" {
			filtered := make([]model.Video, 0, len(rows))
			ql := strings.ToLower(q)
			for _, v := range rows {
				if strings.Contains(strings.ToLower(v.Title), ql) || strings.Contains(strings.ToLower(v.Description), ql) {
					filtered = append(filtered, v)
				}
			}
			rows = filtered
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": rows, "total": len(rows), "limit": limit, "offset": offset, "has_more": false})
		return
	}
	total, err := a.Store.CountReadyVideosFiltered(catID, q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	rows, err := a.Store.ListReadyVideosFiltered(catID, q, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	if len(rows) == 0 && catID == 0 && q == "" && total == 0 {
		rows = demoVideos()
		total = len(rows)
	}
	for i := range rows {
		rows[i] = a.withCoverURL(rows[i])
	}
	c.JSON(http.StatusOK, gin.H{
		"ok": true, "data": rows,
		"total": total, "limit": limit, "offset": offset,
		"has_more": offset+len(rows) < total,
	})
}

func (a *API) GetVideo(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	var v *model.Video
	if a.Store != nil {
		v, err = a.Store.GetVideo(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
			return
		}
	}
	if v == nil {
		for _, d := range demoVideos() {
			if d.ID == id {
				tmp := d
				v = &tmp
				break
			}
		}
	}
	if v == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	out := a.withCoverURL(*v)
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": out})
}

// ResolvePlay handles GET /api/v1/videos/:id/play (and POST for compatibility).
func (a *API) ResolvePlay(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	sid, _ := strconv.Atoi(c.DefaultQuery("sid", "0"))
	nid, _ := strconv.Atoi(c.DefaultQuery("nid", "0"))

	resolver := a.Resolver
	if resolver == nil {
		resolver = &play.Resolver{Cfg: a.Cfg, Store: a.Store}
	}
	info, err := resolver.Resolve(id, sid, nid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": info})
}

// CreatePlaySession keeps POST compatibility; response includes both shapes.
func (a *API) CreatePlaySession(c *gin.Context) {
	a.ResolvePlay(c)
}

func (a *API) AdminOleHdTvSync(c *gin.Context) {
	if !a.adminAuthorized(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "unauthorized"})
		return
	}
	if a.SyncRunner == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "sync not configured"})
		return
	}
	sum, err := a.SyncRunner.Trigger(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error(), "data": sum})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": sum})
}

func (a *API) adminAuthorized(c *gin.Context) bool {
	token := a.Cfg.AdminAPIToken
	if token == "" {
		return false
	}
	h := c.GetHeader("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:]) == token
	}
	return c.GetHeader("X-Admin-Token") == token
}

func (a *API) withCoverURL(v model.Video) model.Video {
	key := v.CoverR2Key
	if key == "" {
		key = v.CoverKey
	}
	switch {
	case key != "" && (strings.HasPrefix(key, "http://") || strings.HasPrefix(key, "https://")):
		v.CoverURL = key
	case key != "" && a.MediaBase != "" && strings.HasPrefix(key, "posters/"):
		v.CoverURL = strings.TrimRight(a.MediaBase, "/") + "/" + strings.TrimLeft(key, "/")
	case key != "" && a.Cfg.R2PublicBase != "":
		v.CoverURL = strings.TrimRight(a.Cfg.R2PublicBase, "/") + "/" + strings.TrimLeft(key, "/")
	case key != "":
		v.CoverURL = play.SignURL(a.Cfg.CDNBaseURL, a.Cfg.CDNSignSecret, key, a.Cfg.PlayTicketTTL)
	case v.PosterSourceURL != "":
		// Until poster is imported to R2/local, show authorized source poster URL.
		v.CoverURL = v.PosterSourceURL
	default:
		v.CoverURL = play.CoverPlaceholder(v.Title)
	}
	return v
}

func demoCategories() []model.Category {
	return []model.Category{
		{ID: 1, Name: "推荐", Slug: "home", Sort: 0, IsActive: true},
		{ID: 2, Name: "电影", Slug: "movie", Sort: 10, IsActive: true},
		{ID: 3, Name: "剧集", Slug: "tv", Sort: 20, IsActive: true},
		{ID: 4, Name: "动漫", Slug: "anime", Sort: 30, IsActive: true},
	}
}

func demoVideos() []model.Video {
	return []model.Video{
		{ID: 1, CategoryID: 1, Title: "示例视频 · Big Buck Bunny", Description: "本地联调 HLS 示例", DurationSec: 596, ViewCount: 12800, Status: "demo", IsActive: true, CoverURL: play.CoverPlaceholder("bbb")},
		{ID: 2, CategoryID: 2, Title: "示例视频 · Sintel", Description: "本地联调封面与卡片", DurationSec: 888, ViewCount: 8600, Status: "demo", IsActive: true, CoverURL: play.CoverPlaceholder("sintel")},
		{ID: 3, CategoryID: 3, Title: "示例视频 · Tears of Steel", Description: "播放器对接用", DurationSec: 734, ViewCount: 5400, Status: "demo", IsActive: true, CoverURL: play.CoverPlaceholder("tos")},
	}
}
