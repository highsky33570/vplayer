package maccms_test

import (
	"strings"
	"testing"

	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
)

func TestIsPlaceholderPoster(t *testing.T) {
	placeholders := []string{
		"https://www.olehdtv.com/static/images/img/hd.png",
		"/static/images/img/hd.png",
		"/static/images/img/info_icon.png",
		"https://cdn.example/load.gif",
		"https://cdn.example/loading.gif",
	}
	for _, u := range placeholders {
		if !maccms.IsPlaceholderPoster(u) {
			t.Fatalf("expected placeholder: %s", u)
		}
	}
	real := "https://static.olelive.com/upload/vod/20260712-1/abc.jpg"
	if maccms.IsPlaceholderPoster(real) || !maccms.IsValidContentPoster(real) {
		t.Fatalf("real poster rejected: %s", real)
	}
}

func TestBestPoster_prefersUploadVodOverHDPlaceholder(t *testing.T) {
	html := `
<img src="/static/images/img/hd.png"/>
<img class="lazyload" src="/static/images/img/hd.png" data-original="https://static.olelive.com/upload/vod/2026/real.jpg"/>
`
	got := maccms.BestPoster(html, "https://www.olehdtv.com")
	if !strings.Contains(got, "/upload/vod/") || strings.Contains(got, "hd.png") {
		t.Fatalf("got %q", got)
	}
}

func TestMergeDetailWithCatalog_keepsCatalogPosterOverPlaceholder(t *testing.T) {
	html := `
<html><head><meta property="og:image" content="/static/images/img/hd.png"/></head>
<body>
<div class="myui-content__detail">
  <h1 class="title">片名</h1>
  <img src="/static/images/img/hd.png"/>
</div>
<div class="playlist"><a href="/index.php/vod/play/id/37782/sid/1/nid/1.html">正片</a></div>
</body></html>`
	detail := maccms.ParseDetailPage(html, "https://www.olehdtv.com/index.php/vod/detail/id/37782.html")
	if maccms.IsValidContentPoster(detail.PosterURL) && strings.Contains(detail.PosterURL, "hd.png") {
		t.Fatalf("detail should not keep hd.png as valid: %q", detail.PosterURL)
	}
	card := maccms.CatalogItem{
		VodID: "37782", Title: "片名",
		PosterURL: "https://static.olelive.com/upload/vod/2026/catalog.jpg",
	}
	merged := maccms.MergeDetailWithCatalog(card, detail)
	if merged.PosterURL != card.PosterURL {
		t.Fatalf("merged poster=%q want catalog", merged.PosterURL)
	}
}

func TestChooseContentPoster(t *testing.T) {
	cat := "https://static.olelive.com/upload/vod/x.jpg"
	if got := maccms.ChooseContentPoster("https://www.olehdtv.com/static/images/img/hd.png", cat); got != cat {
		t.Fatalf("%q", got)
	}
	detail := "https://static.olelive.com/upload/vod/detail.jpg"
	if got := maccms.ChooseContentPoster(detail, cat); got != detail {
		t.Fatalf("%q", got)
	}
}
