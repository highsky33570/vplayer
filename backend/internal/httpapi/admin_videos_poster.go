package httpapi

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/media"
)

const adminPosterUploadMaxBytes = 12 << 20

// AdminUploadVideoPoster POST /api/v1/admin/videos/:id/poster
// multipart field "file"; optional aspect_w / aspect_h (original poster ratio).
func (a *API) AdminUploadVideoPoster(c *gin.Context) {
	if a.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "store unavailable"})
		return
	}
	if a.Objects == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "object store unavailable"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	existing, err := a.Store.GetVideo(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载视频"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}

	if err := c.Request.ParseMultipartForm(adminPosterUploadMaxBytes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid multipart form"})
		return
	}
	file, hdr, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "file is required"})
		return
	}
	defer file.Close()
	if hdr.Size > adminPosterUploadMaxBytes {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "image too large"})
		return
	}
	raw, err := io.ReadAll(io.LimitReader(file, adminPosterUploadMaxBytes+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "无法读取图片"})
		return
	}
	if len(raw) == 0 || int64(len(raw)) > adminPosterUploadMaxBytes {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "image too large or empty"})
		return
	}
	ct := strings.ToLower(hdr.Header.Get("Content-Type"))
	if ct != "" && !strings.HasPrefix(ct, "image/") {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "file must be an image"})
		return
	}

	aspectW, _ := strconv.ParseFloat(strings.TrimSpace(c.Request.FormValue("aspect_w")), 64)
	aspectH, _ := strconv.ParseFloat(strings.TrimSpace(c.Request.FormValue("aspect_h")), 64)
	if aspectW <= 0 || aspectH <= 0 {
		aspectW, aspectH = 2, 3
	}

	key, err := media.PutAdminPoster(c.Request.Context(), a.Objects, id, raw, aspectW, aspectH)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": err.Error()})
		return
	}
	if err := a.Store.UpdateVideoPoster(id, key, ""); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法更新封面"})
		return
	}
	row, err := a.Store.GetVideo(id)
	if err != nil || row == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"id": id, "cover_key": key}})
		return
	}
	shaped := a.withCoverURL(*row)
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": adminVideoDetailFrom(shaped)})
}
