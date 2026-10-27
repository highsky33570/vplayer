package httpapi

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/store"
)

type watchCatalog interface {
	GetVideo(id uint64) (*model.Video, error)
	ListRelatedVideos(current model.Video, limit int) ([]model.Video, error)
	ListPublicEpisodes(videoID uint64) ([]store.PublicEpisode, error)
}

func (a *API) watchCatalog() watchCatalog {
	if a.Watch != nil {
		return a.Watch
	}
	if a.Store != nil {
		return a.Store
	}
	return nil
}

type relatedCardJSON struct {
	ID          uint64 `json:"id"`
	Title       string `json:"title"`
	CoverURL    string `json:"cover_url"`
	ViewCount   int64  `json:"view_count"`
	Year        string `json:"year,omitempty"`
	CategoryID  uint64 `json:"category_id"`
	DurationSec int    `json:"duration_sec"`
}

func relatedLimit(raw string) int {
	if raw == "" {
		return store.RelatedVideoLimit
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n > store.RelatedVideoLimit {
		return store.RelatedVideoLimit
	}
	return n
}

func (a *API) ListRelated(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	cat := a.watchCatalog()
	if cat == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": []relatedCardJSON{}})
		return
	}
	video, err := cat.GetVideo(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	if video == nil || video.ID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "视频不存在"})
		return
	}
	limit := relatedLimit(c.Query("limit"))
	var rows []model.Video
	if a.Store != nil {
		rows, err = a.Store.ListRelatedVideosForViewer(*video, limit, true)
	} else {
		rows, err = cat.ListRelatedVideos(*video, limit)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	cards := make([]relatedCardJSON, 0, len(rows))
	seen := map[uint64]bool{}
	for _, row := range rows {
		if row.ID == 0 || row.ID == video.ID || seen[row.ID] {
			continue
		}
		seen[row.ID] = true
		row = a.withCoverURL(row)
		cards = append(cards, relatedCardJSON{
			ID:          row.ID,
			Title:       row.Title,
			CoverURL:    row.CoverURL,
			ViewCount:   row.ViewCount,
			Year:        row.Year,
			CategoryID:  row.CategoryID,
			DurationSec: row.DurationSec,
		})
		if len(cards) >= limit {
			break
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": cards})
}

func (a *API) ListEpisodes(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	cat := a.watchCatalog()
	if cat == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": []store.PublicEpisode{}})
		return
	}
	video, err := cat.GetVideo(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	if video == nil || video.ID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "视频不存在"})
		return
	}
	rows, err := cat.ListPublicEpisodes(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	rows = store.SortPublicEpisodes(rows)
	// SortPublicEpisodes always returns non-nil; keep an explicit empty slice for JSON [].
	if rows == nil {
		rows = []store.PublicEpisode{}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": rows})
}
