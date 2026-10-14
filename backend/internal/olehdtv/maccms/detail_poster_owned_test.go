package maccms_test

import (
	"strings"
	"testing"

	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
)

// Regression for OLEHDTV detail pages shaped like source 84839:
// current-title content_thumb + hero .bgi hold the real /upload/vod/ poster, while
// /upload/site/ chrome and recommendation /upload/vod/ images must be ignored.
func TestParseDetailPoster_prefersCurrentTitleThumbOverRecommendations(t *testing.T) {
	const want = "https://static.olelive.com/upload/vod/20260928-1/64dc7d18d85e96eccf9a3a03572eca6c.jpg"
	html := `
<html><head>
<meta property="og:image" content="https://static.olelive.com/upload/site/20201117-1/d2035d18801242fc62991b32d6366bf8.png"/>
</head><body>
<div class="bgi" style="background-image:url(` + want + `)"></div>
<div class="myui-content__detail">
  <h1 class="title">穿古代捞金带回绝色美女</h1>
  <a class="content_thumb vodlist_thumb lazyload" dids="84839"
     href="/index.php/vod/play/id/84839/sid/1/nid/1.html"
     data-original="` + want + `"></a>
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
  <li style="background-image:url(https://static.olelive.com/upload/vod/2026/rec-c.jpg)"></li>
</ul>
</body></html>`

	meta := maccms.ParseDetailPage(html, "https://www.olehdtv.com/index.php/vod/detail/id/84839.html")
	if meta.PosterURL != want {
		t.Fatalf("poster=%q want %q", meta.PosterURL, want)
	}
	if strings.Contains(meta.PosterURL, "/upload/site/") {
		t.Fatal("site chrome must not win")
	}
	if strings.Contains(meta.PosterURL, "rec-") {
		t.Fatal("recommendation vod must not win")
	}

	// Catalog site placeholder must yield to the detail title poster.
	merged := maccms.MergeDetailWithCatalog(maccms.CatalogItem{
		VodID: "84839", Title: "穿古代捞金带回绝色美女",
		PosterURL: "https://static.olelive.com/upload/site/20201117-1/d2035d18801242fc62991b32d6366bf8.png",
	}, meta)
	if merged.PosterURL != want {
		t.Fatalf("merged poster=%q want detail title art", merged.PosterURL)
	}
}

func TestExtractTitleOwnedPoster_bgiFallback(t *testing.T) {
	const want = "https://static.olelive.com/upload/vod/2026/hero-only.jpg"
	html := `
<div class="bgi foo" style="background-image: url('` + want + `')"></div>
<a class="vodlist_thumb" dids="90001" data-original="https://static.olelive.com/upload/vod/2026/rec.jpg"></a>
`
	got := maccms.ExtractTitleOwnedPoster(html, "https://www.olehdtv.com/index.php/vod/detail/id/777.html", "777")
	if got != want {
		t.Fatalf("got %q want bgi %q", got, want)
	}
}
