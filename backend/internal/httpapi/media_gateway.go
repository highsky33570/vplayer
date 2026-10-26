package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/media"
	"github.com/tycdn/vplayer/internal/mediasign"
	"github.com/tycdn/vplayer/internal/model"
)

const playlistReadLimit = 2 << 20

var byteRangeHeader = regexp.MustCompile(`^bytes=\d*-\d*$`)

// MediaOpener streams one private object. rangeHeader is empty for a full read.
type MediaOpener interface {
	Open(ctx context.Context, key, rangeHeader string) (*media.ObjectBody, error)
}

type configMediaOpener struct{ cfg config.Config }

func (o configMediaOpener) Open(ctx context.Context, key, rangeHeader string) (*media.ObjectBody, error) {
	return media.OpenPrivateObject(ctx, o.cfg.R2Endpoint, o.cfg.R2Bucket, o.cfg.R2AccessKey, o.cfg.R2SecretKey, o.cfg.R2Region, o.cfg.OleHdTvLocalObjectsDir, key, rangeHeader)
}

func (a *API) opener() MediaOpener {
	if a.MediaOpener != nil {
		return a.MediaOpener
	}
	return configMediaOpener{cfg: a.Cfg}
}

// ServeSignedMedia authorizes one object with HMAC+expiry, then streams it.
// Playlists are rewritten so child playlists, segments, and key URIs are signed too.
func (a *API) ServeSignedMedia(c *gin.Context) {
	if !a.Cfg.MediaSigningEnabled {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
		return
	}
	key, err := mediasign.CanonicalObjectKey(strings.TrimPrefix(c.Param("path"), "/"))
	if err != nil {
		denyMedia(c)
		return
	}
	exp, sig, ok := readMediaAuth(c)
	if !ok || mediasign.Verify(a.Cfg.MediaSigningSecret, key, sig, exp, time.Now().Unix(), a.Cfg.MediaURLTTLSeconds) != nil {
		denyMedia(c)
		return
	}
	rng, ok := safeRange(c.GetHeader("Range"))
	if !ok {
		c.JSON(http.StatusRequestedRangeNotSatisfiable, gin.H{"ok": false, "message": "bad range"})
		return
	}
	openRange := rng
	if strings.HasSuffix(key, ".m3u8") {
		openRange = ""
	}
	body, err := a.opener().Open(c.Request.Context(), key, openRange)
	if err != nil {
		if api, ok := media.AsS3APIError(err); ok && api.StatusCode == http.StatusNotFound {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
			return
		}
		if api, ok := media.AsS3APIError(err); ok && api.StatusCode == http.StatusRequestedRangeNotSatisfiable {
			c.JSON(http.StatusRequestedRangeNotSatisfiable, gin.H{"ok": false, "message": "bad range"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "message": "media unavailable"})
		return
	}
	defer body.Body.Close()

	remain := exp - time.Now().Unix()
	if remain < 0 {
		remain = 0
	}
	c.Header("Cache-Control", fmt.Sprintf("private, max-age=%d", remain))
	c.Header("Accept-Ranges", "bytes")
	c.Header("X-Content-Type-Options", "nosniff")
	if body.ContentRange != "" {
		c.Header("Content-Range", body.ContentRange)
	}

	if strings.HasSuffix(key, ".m3u8") {
		raw, err := io.ReadAll(io.LimitReader(body.Body, playlistReadLimit+1))
		if err != nil || len(raw) > playlistReadLimit {
			denyMedia(c)
			return
		}
		cdn := media.OptionsFromEnv(a.Cfg.MediaCDNBaseURL, a.Cfg.CDNBaseURL, a.Cfg.R2Bucket, a.Cfg.MediaCDNPathMode)
		rewritten, err := mediasign.RewritePlaylist(string(raw), key, cdn, func(objectKey string) (string, error) {
			return mediasign.SignedMediaURL(a.mediaPublicBase(c), objectKey, a.Cfg.MediaSigningSecret, a.Cfg.MediaURLTTLSeconds, time.Now())
		})
		if err != nil {
			denyMedia(c)
			return
		}
		c.Data(http.StatusOK, "application/vnd.apple.mpegurl", []byte(rewritten))
		return
	}

	c.DataFromReader(body.StatusCode, body.ContentLength, mediaContentType(key, body.ContentType), body.Body, nil)
}

func (a *API) protectPlaybackURL(c *gin.Context, info *model.PlaybackInfo) bool {
	if info == nil || !a.Cfg.MediaSigningEnabled {
		return true
	}
	cdn := media.OptionsFromEnv(a.Cfg.MediaCDNBaseURL, a.Cfg.CDNBaseURL, a.Cfg.R2Bucket, a.Cfg.MediaCDNPathMode)
	key, err := mediasign.ObjectKeyFromCDNURL(info.URL, cdn)
	if err != nil {
		if sameHost(info.URL, cdn.BaseURL) {
			c.JSON(http.StatusBadGateway, gin.H{"ok": false, "message": "media unavailable"})
			return false
		}
		return true
	}
	signed, err := mediasign.SignedMediaURL(a.mediaPublicBase(c), key, a.Cfg.MediaSigningSecret, a.Cfg.MediaURLTTLSeconds, time.Now())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "message": "media unavailable"})
		return false
	}
	info.URL = signed
	return true
}

func (a *API) mediaPublicBase(c *gin.Context) string {
	if base := strings.TrimRight(a.Cfg.MediaPublicBase, "/"); base != "" {
		return base
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if p := c.GetHeader("X-Forwarded-Proto"); p == "http" || p == "https" {
		scheme = p
	}
	return scheme + "://" + c.Request.Host
}

func sameHost(raw, base string) bool {
	u, err := url.Parse(raw)
	b, err2 := url.Parse(base)
	if err != nil || err2 != nil || u.Host == "" || b.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, b.Host)
}

func readMediaAuth(c *gin.Context) (exp int64, sig string, ok bool) {
	exp, err := strconv.ParseInt(c.Query("exp"), 10, 64)
	sig = c.Query("sig")
	q := c.Request.URL.Query()
	q.Del("sig")
	c.Request.URL.RawQuery = q.Encode()
	if err != nil || sig == "" {
		return 0, "", false
	}
	return exp, sig, true
}

func safeRange(h string) (string, bool) {
	h = strings.TrimSpace(h)
	if h == "" {
		return "", true
	}
	if len(h) > 48 || strings.ContainsAny(h, "\r\n") || !byteRangeHeader.MatchString(h) || h == "bytes=-" {
		return "", false
	}
	return h, true
}

func mediaContentType(key, upstream string) string {
	switch {
	case strings.HasSuffix(key, ".m3u8"):
		return "application/vnd.apple.mpegurl"
	case strings.HasSuffix(key, ".ts"):
		return "video/mp2t"
	case strings.HasSuffix(key, ".m4s"), strings.HasSuffix(key, ".mp4"):
		return "video/mp4"
	case strings.HasSuffix(key, ".aac"):
		return "audio/aac"
	case strings.HasSuffix(key, ".vtt"):
		return "text/vtt"
	case strings.HasSuffix(key, ".key"):
		return "application/octet-stream"
	case upstream != "":
		return upstream
	default:
		return "application/octet-stream"
	}
}

func denyMedia(c *gin.Context) {
	c.JSON(http.StatusForbidden, gin.H{"ok": false, "message": "forbidden"})
}
