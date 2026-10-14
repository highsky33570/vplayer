package olehdtv

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
)

type detailMockFetcher struct {
	mu    sync.Mutex
	urls  []string
	byURL map[string]string
}

func (m *detailMockFetcher) Fetch(ctx context.Context, pageURL string) (string, error) {
	_ = ctx
	m.mu.Lock()
	m.urls = append(m.urls, pageURL)
	m.mu.Unlock()
	u, err := url.Parse(pageURL)
	if err != nil {
		return "", err
	}
	key := u.Path
	m.mu.Lock()
	body, ok := m.byURL[key]
	if !ok {
		body, ok = m.byURL[pageURL]
	}
	m.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("mock missing %s", pageURL)
	}
	return body, nil
}

func (m *detailMockFetcher) requested() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.urls...)
}

func TestFetchDetailVideo_titleOwnedPosterNoPlayEnrich(t *testing.T) {
	const wantPoster = "https://static.olelive.com/upload/vod/20260928-1/64dc7d18d85e96eccf9a3a03572eca6c.jpg"
	const sourceID = "84839"
	html := `
<html><head>
<meta property="og:image" content="https://static.olelive.com/upload/site/20201117-1/d2035d18801242fc62991b32d6366bf8.png"/>
</head><body>
<div class="bgi" style="background-image:url(` + wantPoster + `)"></div>
<div class="myui-content__detail">
  <h1 class="title">穿古代捞金带回绝色美女</h1>
  <a class="content_thumb vodlist_thumb lazyload" dids="84839"
     href="/index.php/vod/play/id/84839/sid/1/nid/1.html"
     data-original="` + wantPoster + `"></a>
</div>
<div class="playlist">
  <a href="/index.php/vod/play/id/84839/sid/1/nid/1.html">正片</a>
</div>
<div class="site-ad">
  <img src="https://static.olelive.com/upload/site/20201117-1/105e22b56101eb0268ea9904972357ba.png"/>
</div>
<ul class="myui-vodlist__related">
  <li><a class="vodlist_thumb lazyload" dids="90001"
        href="/index.php/vod/play/id/90001/sid/1/nid/1.html"
        data-original="https://static.olelive.com/upload/vod/2026/rec-a.jpg"></a></li>
  <li><a class="vodlist_thumb lazyload" dids="90002"
        href="/index.php/vod/detail/id/90002.html"
        data-original="https://static.olelive.com/upload/vod/2026/rec-b.jpg"></a></li>
</ul>
</body></html>`

	mock := &detailMockFetcher{byURL: map[string]string{
		"/index.php/vod/detail/id/84839.html": html,
		// Intentionally no play-page bodies — FetchDetailVideo must not request them.
	}}
	src := NewMacCMSHTTPRecentWithFetcher("https://www.olehdtv.com", mock, 1)
	v, meta, err := src.FetchDetailVideo(context.Background(), sourceID)
	if err != nil {
		t.Fatal(err)
	}
	if v.SourceID != sourceID {
		t.Fatalf("source_id=%q", v.SourceID)
	}
	if v.PosterURL != wantPoster {
		t.Fatalf("poster=%q want title-owned %q", v.PosterURL, wantPoster)
	}
	if strings.Contains(v.PosterURL, "/upload/site/") || strings.Contains(v.PosterURL, "rec-") {
		t.Fatalf("chrome/recommendation poster leaked: %q", v.PosterURL)
	}
	if meta.DetailRequests != 1 || meta.PlayRequests != 0 || meta.ListingRequests != 0 {
		t.Fatalf("budget=%+v", meta)
	}
	reqs := mock.requested()
	if len(reqs) != 1 {
		t.Fatalf("expected exactly 1 HTTP fetch, got %v", reqs)
	}
	if !strings.Contains(reqs[0], "/vod/detail/id/84839.html") {
		t.Fatalf("unexpected url %s", reqs[0])
	}
	for _, ep := range v.Episodes {
		if ep.PlaybackURL != "" {
			t.Fatalf("detail-only must leave playback empty: %+v", ep)
		}
		if ep.PlaybackStatus != "pending_enrichment" {
			t.Fatalf("expected pending_enrichment, got %+v", ep)
		}
	}
}

func TestFetchDetailVideo_invalidID(t *testing.T) {
	src := NewMacCMSHTTPRecentWithFetcher("https://www.olehdtv.com", &detailMockFetcher{byURL: map[string]string{}}, 1)
	_, _, err := src.FetchDetailVideo(context.Background(), "0")
	if err == nil {
		t.Fatal("expected invalid id error")
	}
}

var _ maccms.PageFetcher = (*detailMockFetcher)(nil)
