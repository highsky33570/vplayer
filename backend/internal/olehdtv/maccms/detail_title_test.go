package maccms_test

import (
	"strings"
	"testing"

	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
)

const olehSiteBrand = "欧乐影院－面向海外华人的在线视频媒体平台,海量高清视频在线观看"

func TestIsGenericSiteTitle(t *testing.T) {
	if !maccms.IsGenericSiteTitle(olehSiteBrand) {
		t.Fatal("expected OLEHDTV branding rejected")
	}
	if !maccms.IsGenericSiteTitle("某某影院－面向海外华人的在线视频媒体平台") {
		t.Fatal("expected generic portal branding rejected")
	}
	if maccms.IsGenericSiteTitle("漫长的季节") {
		t.Fatal("real title rejected")
	}
	if maccms.IsGenericSiteTitle("庆余年第二季") {
		t.Fatal("real series title rejected")
	}
}

func TestParseDetail_rejectsHeaderBrandTitleAttr(t *testing.T) {
	// Live-style page: header logo title= branding, no useful h1 in chrome,
	// real title in myui detail heading + catalog card must win over brand.
	html := `
<html><head>
<title>` + olehSiteBrand + `</title>
<meta name="description" content="` + olehSiteBrand + `"/>
</head><body>
<a href="/" title="` + olehSiteBrand + `"><img src="/logo.png"/></a>
<h1><a href="/" title="` + olehSiteBrand + `">欧乐影院</a></h1>
<div class="myui-content__detail">
  <h1 class="title">真实电影甲<span class="score">8.5</span></h1>
  <p>导演：甲导演</p>
  <p>主演：演员A,演员B</p>
  <p>地区：美国</p>
  <p>年份：2024</p>
  <img data-original="/upload/vod/82861.jpg"/>
  <div class="sketch content">这是剧情简介不是站点广告</div>
</div>
<div class="playlist">
  <a href="/index.php/vod/play/id/82861/sid/1/nid/1.html">正片</a>
</div>
</body></html>`

	meta := maccms.ParseDetailPage(html, "https://maccms.local/index.php/vod/detail/id/82861.html")
	if meta.Title != "真实电影甲" {
		t.Fatalf("title=%q want 真实电影甲", meta.Title)
	}
	if meta.Year != "2024" || meta.Area != "美国" || meta.Director != "甲导演" {
		t.Fatalf("meta fields wrong: %#v", meta)
	}
	if !strings.Contains(meta.PosterURL, "82861.jpg") {
		t.Fatalf("poster=%q", meta.PosterURL)
	}
	if len(meta.Episodes) != 1 {
		t.Fatalf("episodes=%d", len(meta.Episodes))
	}
	if maccms.IsGenericSiteTitle(meta.Description) || strings.Contains(meta.Description, "在线视频媒体平台") {
		t.Fatalf("description kept site brand: %q", meta.Description)
	}
}

func TestParseDetail_ogTitleAndJSONLD(t *testing.T) {
	html := `
<html><head>
<title>` + olehSiteBrand + `</title>
<meta property="og:title" content="《og电影乙》高清在线观看 - 欧乐影院"/>
<meta property="og:description" content="og剧情简介乙"/>
<meta property="og:image" content="/upload/vod/84001.jpg"/>
<script type="application/ld+json">{"@type":"Movie","name":"JSON电影乙","description":"ld简介"}</script>
</head><body>
<a href="/" title="` + olehSiteBrand + `">home</a>
<div class="playlist">
  <a href="/index.php/vod/play/id/84001/sid/1/nid/1.html">第1集</a>
  <a href="/index.php/vod/play/id/84001/sid/1/nid/2.html">第2集</a>
</div>
</body></html>`
	meta := maccms.ParseDetailPage(html, "https://maccms.local/index.php/vod/detail/id/84001.html")
	// JSON-LD name is preferred over og:title when no content heading.
	if meta.Title != "JSON电影乙" {
		t.Fatalf("title=%q", meta.Title)
	}
	if len(meta.Episodes) != 2 {
		t.Fatalf("episodes=%d", len(meta.Episodes))
	}
	if !strings.Contains(meta.PosterURL, "84001.jpg") {
		t.Fatalf("poster=%q", meta.PosterURL)
	}
}

func TestParseDetail_ogTitleWhenNoHeadingOrJSONLD(t *testing.T) {
	html := `
<html><head>
<title>` + olehSiteBrand + `</title>
<meta property="og:title" content="og连续剧丙"/>
</head><body>
<a href="/" title="` + olehSiteBrand + `">home</a>
<div class="playlist">
  <a href="/index.php/vod/play/id/83449/sid/1/nid/1.html">第1集</a>
</div>
</body></html>`
	meta := maccms.ParseDetailPage(html, "https://maccms.local/index.php/vod/detail/id/83449.html")
	if meta.Title != "og连续剧丙" {
		t.Fatalf("title=%q", meta.Title)
	}
}

func TestMergeDetailWithCatalog_preservesCatalogOverBrand(t *testing.T) {
	html := `
<html><head><title>` + olehSiteBrand + `</title></head>
<body>
<a href="/" title="` + olehSiteBrand + `">logo</a>
<h1>` + olehSiteBrand + `</h1>
<div class="playlist">
  <a href="/index.php/vod/play/id/29698/sid/1/nid/1.html">正片</a>
</div>
</body></html>`
	detail := maccms.ParseDetailPage(html, "https://maccms.local/index.php/vod/detail/id/29698.html")
	if detail.Title != "" && !maccms.IsGenericSiteTitle(detail.Title) {
		// parser may return empty after rejecting brand
	}
	if maccms.IsValidContentTitle(detail.Title) {
		t.Fatalf("detail should not expose brand as valid content title: %q", detail.Title)
	}
	card := maccms.CatalogItem{
		VodID: "29698", Title: "目录卡片片名丁", TypeID: "1",
		DetailURL: "https://maccms.local/index.php/vod/detail/id/29698.html",
		PosterURL: "/upload/vod/29698.jpg",
	}
	merged := maccms.MergeDetailWithCatalog(card, detail)
	if merged.Title != "目录卡片片名丁" {
		t.Fatalf("catalog title overwritten: %q", merged.Title)
	}
	if merged.PosterURL == "" {
		t.Fatal("poster fallback lost")
	}
	if len(merged.Episodes) != 1 {
		t.Fatalf("episodes=%d", len(merged.Episodes))
	}
}

func TestMergeDetailWithCatalog_detailHeadingWins(t *testing.T) {
	html := `
<html><body>
<div class="myui-content__detail"><h1 class="title">详情页正确标题</h1></div>
<div class="playlist"><a href="/index.php/vod/play/id/37782/sid/1/nid/1.html">正片</a></div>
</body></html>`
	detail := maccms.ParseDetailPage(html, "https://maccms.local/index.php/vod/detail/id/37782.html")
	card := maccms.CatalogItem{VodID: "37782", Title: "较旧的目录标题", TypeID: "2"}
	merged := maccms.MergeDetailWithCatalog(card, detail)
	if merged.Title != "详情页正确标题" {
		t.Fatalf("got %q", merged.Title)
	}
}

func TestChooseContentTitle(t *testing.T) {
	if got := maccms.ChooseContentTitle(olehSiteBrand, "好片名"); got != "好片名" {
		t.Fatalf("%q", got)
	}
	if got := maccms.ChooseContentTitle("新片名", "旧片名"); got != "新片名" {
		t.Fatalf("%q", got)
	}
	if got := maccms.ChooseContentTitle("", "仅目录"); got != "仅目录" {
		t.Fatalf("%q", got)
	}
}
