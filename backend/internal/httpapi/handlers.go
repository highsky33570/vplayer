package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/mail"
	"github.com/tycdn/vplayer/internal/media"
	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
	"github.com/tycdn/vplayer/internal/play"
	"github.com/tycdn/vplayer/internal/scheduler"
	"github.com/tycdn/vplayer/internal/store"
)

type API struct {
	Cfg   config.Config
	Store *store.MySQL
	Users store.UserRepo
	// PendingRegs optionally overrides Store for pending registration (tests).
	PendingRegs store.PendingRegistrationStore
	Mailer      mail.Mailer
	registerIP  *registerIPLimiter
	Resolver    *play.Resolver
	SyncRunner  *scheduler.OleHdTvRunner
	MediaBase   string
	// PublicVideos optionally overrides Store for public video list/count (tests).
	PublicVideos publicVideoRepo
	// AdminVideos optionally overrides Store for admin video reads (tests).
	AdminVideos adminVideoRepo
	// AdminCategories optionally overrides Store for admin category reads (tests).
	AdminCategories adminCategoryRepo
	// AdminUsers optionally overrides Store for admin user reads (tests).
	AdminUsers adminUserRepo
	// AdminSync optionally overrides Store for admin sync reads (tests).
	AdminSync adminSyncRepo
	// AdminOps optionally overrides Store for admin operational_runs reads (tests).
	AdminOps adminOpsRepo
	// AdminActionGuards optionally overrides Store for Admin action busy/cooldown checks (tests).
	AdminActionGuards adminActionGuards
	// WorkerLauncher optionally overrides production os/exec launcher (tests).
	WorkerLauncher WorkerLauncher
	// AdminMedia optionally overrides Store for admin media reads (tests).
	AdminMedia adminMediaRepo
	// Comments optionally overrides Store for video comments (tests).
	Comments store.CommentRepo
	// Watch optionally overrides Store for related videos and public episodes (tests).
	Watch watchCatalog
	// Clock optionally overrides time.Now for tests. Values are used in UTC.
	Clock func() time.Time
}

// publicVideoRepo is the store surface used by GET /api/v1/videos.
type publicVideoRepo interface {
	CountReadyVideosFiltered(categoryID uint64, query, excludeCategorySlug string) (int, error)
	ListReadyVideosFiltered(categoryID uint64, query, excludeCategorySlug string, limit, offset int) ([]model.Video, error)
}

func (a *API) publicVideoRepo() publicVideoRepo {
	if a.PublicVideos != nil {
		return a.PublicVideos
	}
	if a.Store != nil {
		return a.Store
	}
	return nil
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
	// Optional homepage-only filter; absent/empty => no exclusion (search/browse/category unchanged).
	excludeSlug := store.NormalizeExcludeCategorySlug(c.Query("exclude_category_slug"))
	if limit <= 0 {
		limit = 24
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	repo := a.publicVideoRepo()
	if repo == nil {
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
	total, err := repo.CountReadyVideosFiltered(catID, q, excludeSlug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	rows, err := repo.ListReadyVideosFiltered(catID, q, excludeSlug, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	if len(rows) == 0 && catID == 0 && q == "" && excludeSlug == "" && total == 0 {
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
		if play.IsPlaybackUnavailable(err) {
			c.JSON(http.StatusOK, gin.H{
				"ok":      false,
				"error":   "playback_unavailable",
				"message": play.UserMessagePlaybackUnavailable,
			})
			return
		}
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
	cdn := media.OptionsFromEnv(a.Cfg.MediaCDNBaseURL, a.Cfg.CDNBaseURL, a.Cfg.R2Bucket, a.Cfg.MediaCDNPathMode)
	switch {
	case key != "" && (strings.HasPrefix(key, "http://") || strings.HasPrefix(key, "https://")):
		v.CoverURL = key
	case key != "" && a.MediaBase != "" && strings.HasPrefix(key, "posters/") &&
		(a.Cfg.MediaCDNBaseURL == "" || a.Cfg.MediaCDNBaseURL == "http://localhost:8080" || strings.Contains(a.Cfg.MediaCDNBaseURL, "localhost")):
		// Local object store for development when no production media CDN is configured.
		v.CoverURL = strings.TrimRight(a.MediaBase, "/") + "/" + strings.TrimLeft(key, "/")
	case key != "":
		url := media.BuildMediaURL(cdn, key)
		if a.Cfg.CDNURLAuthMode == "legacy_hmac" {
			url = play.SignURL(cdn.BaseURL, a.Cfg.CDNSignSecret, media.DeliveryObjectPath(cdn, key), a.Cfg.PlayTicketTTL)
		}
		v.CoverURL = url
	case v.PosterSourceURL != "" && !maccms.IsPlaceholderPoster(v.PosterSourceURL):
		// Until poster is imported to R2/CDN, keep the authorized external source URL.
		v.CoverURL = v.PosterSourceURL
	case v.PosterSourceURL != "":
		// Known source chrome/placeholder: omit cover_url so clients use branded fallback.
		v.CoverURL = ""
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
