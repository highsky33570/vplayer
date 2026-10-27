package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/store"
)

const (
	libraryListDefault = 24
	libraryListMax     = 100
	// historyProgressMaxSec bounds client-reported progress (about 11 days) so garbage cannot overflow INT.
	historyProgressMaxSec = 1_000_000
)

func (a *API) favorites() store.FavoriteRepo {
	if a.Favorites != nil {
		return a.Favorites
	}
	if a.Store != nil {
		return a.Store
	}
	return nil
}

func (a *API) history() store.HistoryRepo {
	if a.History != nil {
		return a.History
	}
	if a.Store != nil {
		return a.Store
	}
	return nil
}

// pageQuery reads limit/offset with a default and a cap. Invalid values fall back to the default.
func pageQuery(c *gin.Context, def, max int) (int, int) {
	limit := def
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > max {
		limit = max
	}
	offset := 0
	if raw := c.Query("offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			offset = n
		}
	}
	return limit, offset
}

func pageJSON(data any, count, total, limit, offset int) gin.H {
	return gin.H{
		"ok": true, "data": data,
		"total": total, "limit": limit, "offset": offset,
		"has_more": offset+count < total,
	}
}

// libraryUser returns the authenticated user id or writes a 401.
func libraryUser(c *gin.Context) (uint64, bool) {
	uid, ok := authUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "未登录"})
	}
	return uid, ok
}

func videoIDParam(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("videoId"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return 0, false
	}
	return id, true
}

type favoriteItemJSON struct {
	model.Video
	FolderID    *uint64 `json:"folder_id,omitempty"`
	FavoritedAt string  `json:"favorited_at"`
}

type favoriteFolderJSON struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
	Count     int    `json:"count"`
	CreatedAt string `json:"created_at"`
}

type historyItemJSON struct {
	model.Video
	LastWatchedAt string `json:"last_watched_at"`
	ProgressSec   int    `json:"progress_sec"`
}

// MeSummary handles GET /api/v1/me/summary.
func (a *API) MeSummary(c *gin.Context) {
	uid, ok := libraryUser(c)
	if !ok {
		return
	}
	var favs, hist, unread int
	var err error
	if repo := a.favorites(); repo != nil {
		if favs, err = repo.CountFavorites(uid); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
			return
		}
	}
	if repo := a.history(); repo != nil {
		if hist, err = repo.CountHistory(uid); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
			return
		}
	}
	if repo := a.notifications(); repo != nil {
		if unread, err = repo.CountUnread(uid); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{
		"favorites":            favs,
		"history":              hist,
		"unread_notifications": unread,
	}})
}

// ListFavorites handles GET /api/v1/me/favorites?folder_id=&limit=&offset=.
func (a *API) ListFavorites(c *gin.Context) {
	uid, ok := libraryUser(c)
	if !ok {
		return
	}
	repo := a.favorites()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "收藏暂不可用"})
		return
	}
	var folderID *uint64
	if raw := c.Query("folder_id"); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid folder_id"})
			return
		}
		folderID = &id
	}
	limit, offset := pageQuery(c, libraryListDefault, libraryListMax)
	rows, total, err := repo.ListFavorites(uid, folderID, limit, offset, a.viewerSeesMembership(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	data := make([]favoriteItemJSON, 0, len(rows))
	for _, row := range rows {
		data = append(data, favoriteItemJSON{
			Video:       a.withCoverURL(row.Video),
			FolderID:    row.FolderID,
			FavoritedAt: row.FavoritedAt.UTC().Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, pageJSON(data, len(data), total, limit, offset))
}

// ListFavoriteFolders handles GET /api/v1/me/favorite-folders.
func (a *API) ListFavoriteFolders(c *gin.Context) {
	uid, ok := libraryUser(c)
	if !ok {
		return
	}
	repo := a.favorites()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "收藏暂不可用"})
		return
	}
	folders, err := repo.ListFolders(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	data := make([]favoriteFolderJSON, 0, len(folders))
	for _, f := range folders {
		data = append(data, favoriteFolderJSON{
			ID: f.ID, Name: f.Name, IsDefault: f.IsDefault, Count: f.Count,
			CreatedAt: f.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": data})
}

// GetFavoriteStatus handles GET /api/v1/me/favorites/:videoId.
func (a *API) GetFavoriteStatus(c *gin.Context) {
	uid, ok := libraryUser(c)
	if !ok {
		return
	}
	videoID, ok := videoIDParam(c)
	if !ok {
		return
	}
	repo := a.favorites()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "收藏暂不可用"})
		return
	}
	fav, err := repo.IsFavorited(uid, videoID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"favorited": fav}})
}

// AddFavorite handles POST /api/v1/me/favorites/:videoId. Repeating it is a no-op.
func (a *API) AddFavorite(c *gin.Context) {
	uid, ok := libraryUser(c)
	if !ok {
		return
	}
	videoID, ok := videoIDParam(c)
	if !ok {
		return
	}
	repo := a.favorites()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "收藏暂不可用"})
		return
	}
	exists, err := repo.FavoriteVideoExists(videoID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "视频不存在"})
		return
	}
	now := a.nowUTC()
	folderID, err := repo.EnsureDefaultFolder(uid, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	if _, err := repo.AddFavorite(uid, videoID, folderID, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"favorited": true}})
}

// RemoveFavorite handles DELETE /api/v1/me/favorites/:videoId. Missing favorites are not an error.
func (a *API) RemoveFavorite(c *gin.Context) {
	uid, ok := libraryUser(c)
	if !ok {
		return
	}
	videoID, ok := videoIDParam(c)
	if !ok {
		return
	}
	repo := a.favorites()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "收藏暂不可用"})
		return
	}
	if err := repo.RemoveFavorite(uid, videoID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"favorited": false}})
}

// ListHistory handles GET /api/v1/me/history.
func (a *API) ListHistory(c *gin.Context) {
	uid, ok := libraryUser(c)
	if !ok {
		return
	}
	repo := a.history()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "观看历史暂不可用"})
		return
	}
	limit, offset := pageQuery(c, libraryListDefault, libraryListMax)
	rows, total, err := repo.ListHistory(uid, limit, offset, a.viewerSeesMembership(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	data := make([]historyItemJSON, 0, len(rows))
	for _, row := range rows {
		data = append(data, historyItemJSON{
			Video:         a.withCoverURL(row.Video),
			LastWatchedAt: row.LastWatchedAt.UTC().Format(time.RFC3339),
			ProgressSec:   row.ProgressSec,
		})
	}
	c.JSON(http.StatusOK, pageJSON(data, len(data), total, limit, offset))
}

// UpsertHistory handles POST /api/v1/me/history/:videoId with optional {"progress_sec": N}.
// A missing or negative progress_sec refreshes last_watched_at and keeps the stored progress.
func (a *API) UpsertHistory(c *gin.Context) {
	uid, ok := libraryUser(c)
	if !ok {
		return
	}
	videoID, ok := videoIDParam(c)
	if !ok {
		return
	}
	var body struct {
		ProgressSec *int `json:"progress_sec"`
	}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请求格式错误"})
			return
		}
	}
	progress := -1
	if body.ProgressSec != nil && *body.ProgressSec >= 0 {
		progress = *body.ProgressSec
		if progress > historyProgressMaxSec {
			progress = historyProgressMaxSec
		}
	}
	repo := a.history()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "观看历史暂不可用"})
		return
	}
	exists, err := repo.HistoryVideoExists(videoID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "视频不存在"})
		return
	}
	if err := repo.UpsertHistory(uid, videoID, progress, a.nowUTC()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteHistoryItem handles DELETE /api/v1/me/history/:videoId.
func (a *API) DeleteHistoryItem(c *gin.Context) {
	uid, ok := libraryUser(c)
	if !ok {
		return
	}
	videoID, ok := videoIDParam(c)
	if !ok {
		return
	}
	repo := a.history()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "观看历史暂不可用"})
		return
	}
	if err := repo.DeleteHistory(uid, videoID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ClearHistory handles DELETE /api/v1/me/history.
func (a *API) ClearHistory(c *gin.Context) {
	uid, ok := libraryUser(c)
	if !ok {
		return
	}
	repo := a.history()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "观看历史暂不可用"})
		return
	}
	if err := repo.ClearHistory(uid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
