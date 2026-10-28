package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/media"
	"github.com/tycdn/vplayer/internal/mediasign"
	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/play"
)

const gatewaySecret = "test-media-signing-secret"

type memMedia struct {
	objects map[string][]byte
	ranges  []string
	keys    []string
}

func (m *memMedia) Open(_ context.Context, key, rangeHeader string) (*media.ObjectBody, error) {
	m.keys = append(m.keys, key)
	m.ranges = append(m.ranges, rangeHeader)
	b, ok := m.objects[key]
	if !ok {
		return nil, &media.S3APIError{Op: "get", Key: key, StatusCode: http.StatusNotFound, Message: "not_found"}
	}
	if rangeHeader == "bytes=0-3" {
		return &media.ObjectBody{
			StatusCode:    http.StatusPartialContent,
			ContentLength: 4,
			ContentRange:  "bytes 0-3/8",
			Body:          io.NopCloser(strings.NewReader(string(b[:4]))),
		}, nil
	}
	return &media.ObjectBody{
		StatusCode:    http.StatusOK,
		ContentLength: int64(len(b)),
		Body:          io.NopCloser(strings.NewReader(string(b))),
	}, nil
}

func gatewayRouter(t *testing.T, opener MediaOpener) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := &API{Cfg: config.Config{
		AuthRequired:         true,
		PlaybackAuthRequired: false,
		MediaSigningEnabled:  true,
		MediaSigningSecret:   gatewaySecret,
		MediaURLTTLSeconds:   600,
		MediaPublicBase:      "https://play.example",
		MediaCDNBaseURL:      "https://cdn.example",
		CDNBaseURL:           "https://cdn.example",
		R2Bucket:             "dongman",
		MediaCDNPathMode:     media.CDNPathModeBucketPrefix,
	}, MediaOpener: opener}
	r.GET("/api/v1/media/*path", api.ServeSignedMedia)
	r.GET("/api/v1/videos/:id/play", api.ResolvePlay)
	r.POST("/api/v1/videos/:id/comments", api.RequireUser, func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"ok": true})
	})
	api.Resolver = &play.Resolver{
		Cfg: api.Cfg,
		VideoByID: func(id uint64) (*model.Video, error) {
			return &model.Video{ID: id, Status: "ready"}, nil
		},
		FindEpisode: func(videoID uint64, sid, nid int) (*model.Episode, error) {
			return &model.Episode{
				ID:             8,
				VideoID:        videoID,
				HLSObjectKey:   "videos/9/8/index.m3u8",
				PlaybackURL:    "https://source.example/a.m3u8",
				PlaybackStatus: "ok",
			}, nil
		},
	}
	return r
}

func signedPath(t *testing.T, key string, exp int64) string {
	t.Helper()
	sig, err := mediasign.Sign(gatewaySecret, key, exp)
	if err != nil {
		t.Fatal(err)
	}
	return "/api/v1/media/" + key + "?exp=" + itoa64(exp) + "&sig=" + sig
}

func TestSignedMediaGateway(t *testing.T) {
	playlist := "#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXT-X-KEY:METHOD=AES-128,URI=\"enc.key\"\n#EXTINF:4,\nseg.ts\n720/index.m3u8\n"
	store := &memMedia{objects: map[string][]byte{
		"videos/9/8/index.m3u8": []byte(playlist),
		"videos/9/8/seg.ts":     []byte("01234567"),
	}}
	r := gatewayRouter(t, store)
	exp := time.Now().Add(5 * time.Minute).Unix()
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, signedPath(t, "videos/9/8/index.m3u8", exp), nil))
	if res.Code != http.StatusOK {
		t.Fatalf("playlist %d %s", res.Code, res.Body.String())
	}
	if ct := res.Header().Get("Content-Type"); !strings.Contains(ct, "mpegurl") {
		t.Fatalf("content type %s", ct)
	}
	if !strings.Contains(res.Header().Get("Cache-Control"), "private") {
		t.Fatal(res.Header().Get("Cache-Control"))
	}
	body := res.Body.String()
	for _, frag := range []string{
		"/api/v1/media/videos/9/8/seg.ts?",
		"/api/v1/media/videos/9/8/init.mp4?",
		"/api/v1/media/videos/9/8/enc.key?",
		"/api/v1/media/videos/9/8/720/index.m3u8?",
	} {
		if !strings.Contains(body, frag) {
			t.Fatalf("missing %s\n%s", frag, body)
		}
	}
	childSig := sigOf(t, body, "videos/9/8/seg.ts")
	if err := mediasign.Verify(gatewaySecret, "videos/9/8/seg.ts", childSig.sig, childSig.exp, time.Now().Unix(), 600); err != nil {
		t.Fatal(err)
	}
	if err := mediasign.Verify(gatewaySecret, "videos/1/1/seg.ts", childSig.sig, childSig.exp, time.Now().Unix(), 600); err == nil {
		t.Fatal("segment signature authorized a different path")
	}

	res = httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/media/videos/9/8/seg.ts?exp="+itoa64(exp), nil))
	if res.Code != http.StatusForbidden {
		t.Fatalf("missing sig %d", res.Code)
	}
	res = httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/media/videos/9/8/seg.ts?exp="+itoa64(exp)+"&sig=zz", nil))
	if res.Code != http.StatusForbidden {
		t.Fatalf("malformed %d", res.Code)
	}
	res = httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, signedPath(t, "videos/9/8/seg.ts", time.Now().Add(-time.Minute).Unix()), nil))
	if res.Code != http.StatusForbidden {
		t.Fatalf("expired %d", res.Code)
	}
	res = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, signedPath(t, "videos/9/8/seg.ts", exp), nil)
	q := req.URL.Query()
	q.Set("exp", itoa64(exp+30))
	req.URL.RawQuery = q.Encode()
	r.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("modified exp %d", res.Code)
	}
	res = httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/media/posters/a.jpg?exp="+itoa64(exp)+"&sig=ab", nil))
	if res.Code != http.StatusForbidden {
		t.Fatalf("traversal %d %s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, signedPath(t, "videos/9/8/seg.ts", exp), nil)
	req.Header.Set("Range", "bytes=0-3")
	r.ServeHTTP(res, req)
	if res.Code != http.StatusPartialContent || res.Body.String() != "0123" || res.Header().Get("Content-Range") != "bytes 0-3/8" {
		t.Fatalf("range %d %q %q", res.Code, res.Body.String(), res.Header().Get("Content-Range"))
	}
	if res.Header().Get("Content-Type") != "video/mp2t" {
		t.Fatal(res.Header().Get("Content-Type"))
	}
}

func TestPlayReturnsSignedURLForAnonymousPlayback(t *testing.T) {
	r := gatewayRouter(t, &memMedia{})
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/videos/9/play?episode_id=8", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("%d %s", res.Code, res.Body.String())
	}
	var payload struct {
		Data struct {
			URL         string `json:"url"`
			FallbackURL string `json:"fallback_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload.Data.URL, "https://play.example/api/v1/media/videos/9/8/index.m3u8?exp=") {
		t.Fatal(payload.Data.URL)
	}
	if !strings.Contains(payload.Data.URL, "sig=") {
		t.Fatal(payload.Data.URL)
	}
	if payload.Data.FallbackURL != "https://source.example/a.m3u8" {
		t.Fatal(payload.Data.FallbackURL)
	}
	res = httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v1/videos/9/comments", nil))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("comments %d", res.Code)
	}
}

func TestProtectPlaybackVIPAllowsExternalSourceURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := &API{Cfg: config.Config{
		MediaSigningEnabled: true,
		MediaSigningSecret:  gatewaySecret,
		MediaURLTTLSeconds:  600,
		MediaPublicBase:     "https://play.example",
		MediaCDNBaseURL:     "https://cdn.example",
		CDNBaseURL:          "https://cdn.example",
		R2Bucket:            "dongman",
		MediaCDNPathMode:    media.CDNPathModeBucketPrefix,
	}}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/videos/1/play", nil)
	info := &model.PlaybackInfo{URL: "https://oleh.example/vip/brain.m3u8", Type: "hls"}
	if !api.protectPlaybackURLForUser(c, info, 42, true) {
		t.Fatalf("expected VIP external URL to pass through, got %d %s", w.Code, w.Body.String())
	}
	if info.URL != "https://oleh.example/vip/brain.m3u8" {
		t.Fatalf("url rewritten unexpectedly: %s", info.URL)
	}
}

func TestProtectPlaybackVIPSignsOurCDNURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := &API{Cfg: config.Config{
		MediaSigningEnabled: true,
		MediaSigningSecret:  gatewaySecret,
		MediaURLTTLSeconds:  600,
		MediaPublicBase:     "https://play.example",
		MediaCDNBaseURL:     "https://cdn.example",
		CDNBaseURL:          "https://cdn.example",
		R2Bucket:            "dongman",
		MediaCDNPathMode:    media.CDNPathModeBucketPrefix,
	}}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/videos/1/play", nil)
	info := &model.PlaybackInfo{URL: "https://cdn.example/dongman/videos/9/8/index.m3u8", Type: "hls"}
	if !api.protectPlaybackURLForUser(c, info, 42, true) {
		t.Fatalf("expected VIP CDN URL to sign, got %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(info.URL, "https://play.example/api/v1/media/") || !strings.Contains(info.URL, "uid=42") {
		t.Fatalf("expected uid-bound signed media URL, got %s", info.URL)
	}
}

type parsedSig struct {
	exp int64
	sig string
}

func sigOf(t *testing.T, body, key string) parsedSig {
	t.Helper()
	i := strings.Index(body, "/api/v1/media/"+key+"?")
	if i < 0 {
		t.Fatalf("missing %s", key)
	}
	q := body[i+len("/api/v1/media/"+key+"?"):]
	if end := strings.IndexAny(q, "\"\n "); end >= 0 {
		q = q[:end]
	}
	var out parsedSig
	for _, part := range strings.Split(q, "&") {
		k, v, _ := strings.Cut(part, "=")
		if k == "exp" {
			n, err := parseI64(v)
			if err != nil {
				t.Fatal(err)
			}
			out.exp = n
		}
		if k == "sig" {
			out.sig = v
		}
	}
	return out
}

func parseI64(s string) (int64, error) {
	var n int64
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, io.ErrUnexpectedEOF
		}
		n = n*10 + int64(r-'0')
	}
	return n, nil
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
