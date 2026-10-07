package maccms_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
)

func fixtureRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "..", "..", "testdata", "maccms_html")
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("fixture root: %v", err)
	}
	abs, _ := filepath.Abs(root)
	return abs
}

func read(t *testing.T, parts ...string) string {
	t.Helper()
	p := filepath.Join(append([]string{fixtureRoot(t)}, parts...)...)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseNavigationCategories(t *testing.T) {
	html := read(t, "home.html")
	cats := maccms.ParseNavigationCategories(html, "https://maccms.local")
	if len(cats) != 3 {
		t.Fatalf("cats=%d %#v", len(cats), cats)
	}
	if cats[0].SourceID != "1" || cats[0].Name != "电影" {
		t.Fatalf("%#v", cats[0])
	}
}

func TestParseCatalogPaginationAndDedup(t *testing.T) {
	html := read(t, "type", "1", "page-1.html")
	items, pag := maccms.ParseCatalogPage(html, "https://maccms.local/index.php/vod/type/id/1.html", "1")
	if len(items) != 5 {
		t.Fatalf("items=%d (want 5 unique)", len(items))
	}
	if !pag.HasNext || pag.NextURL == "" {
		t.Fatalf("pagination %#v", pag)
	}
	page2 := read(t, "type", "1", "page-2.html")
	items2, pag2 := maccms.ParseCatalogPage(page2, pag.NextURL, "1")
	if len(items2) != 3 {
		t.Fatalf("page2 items=%d", len(items2))
	}
	if pag2.HasNext {
		t.Fatalf("page2 should not have next: %#v", pag2)
	}
}

func TestPosterExtractionVariants(t *testing.T) {
	html := read(t, "type", "1", "page-1.html")
	posters := maccms.ExtractPosters(html, "https://maccms.local")
	if len(posters) < 4 {
		t.Fatalf("posters=%v", posters)
	}
	missing := `<html><body><h1>x</h1></body></html>`
	if maccms.BestPoster(missing, "https://maccms.local") != "" {
		t.Fatal("expected empty poster")
	}
}

func TestDetailAndEpisodes(t *testing.T) {
	html := read(t, "detail", "20001.html")
	meta := maccms.ParseDetailPage(html, "https://maccms.local/index.php/vod/detail/id/20001.html")
	if meta.Title != "测试剧集甲" || meta.Year != "2025" {
		t.Fatalf("%#v", meta)
	}
	if len(meta.Episodes) != 4 {
		t.Fatalf("episodes=%d (dedup expected 4)", len(meta.Episodes))
	}
	var maxSID int
	for _, e := range meta.Episodes {
		if e.SID > maxSID {
			maxSID = e.SID
		}
	}
	if maxSID < 2 {
		t.Fatalf("expected sid=2 episode, got %#v", meta.Episodes)
	}
}

func TestPlayerAAAAJSONAndJSObject(t *testing.T) {
	p1, ok := maccms.ParsePlayerAAAA(read(t, "play", "10001-s1-n1.html"))
	if !ok || p1.URL == "" || int(p1.ID) != 10001 {
		t.Fatalf("%v %#v", ok, p1)
	}
	p2, ok := maccms.ParsePlayerAAAA(read(t, "play", "20001-s1-n1.html"))
	if !ok || p2.URL == "" {
		t.Fatalf("js object parse failed")
	}
	ep := maccms.EpisodeRef{VodID: "20001", SID: 1, NID: 2}
	enc, ok := maccms.ParsePlayerAAAA(read(t, "play", "20001-s1-n2.html"))
	if !ok {
		t.Fatal("encrypted player missing")
	}
	maccms.ApplyPlayerToEpisode(&ep, enc)
	if ep.Available || ep.StreamURL != "" {
		t.Fatalf("encrypted should be unavailable: %#v", ep)
	}
	if _, ok := maccms.ParsePlayerAAAA(read(t, "play", "20001-s1-n3.html")); ok {
		t.Fatal("missing player should fail")
	}
}

func TestMalformedHTML(t *testing.T) {
	meta := maccms.ParseDetailPage("<html><body>broken", "https://maccms.local/index.php/vod/detail/id/9.html")
	if meta.VodID != "9" {
		t.Fatalf("%#v", meta)
	}
}

func TestPaginationLoopDetection(t *testing.T) {
	visited := map[string]bool{}
	maccms.MarkVisited(visited, "https://maccms.local/a")
	if !maccms.DetectPaginationLoop(visited, "https://maccms.local/a/") {
		t.Fatal("expected loop")
	}
}

func TestEngineCrawlHTMLDump(t *testing.T) {
	root := fixtureRoot(t)
	eng := maccms.NewEngine(&maccms.FileFetcher{Root: root, BaseURL: "https://maccms.local"}, "https://maccms.local")
	eng.Concurrency = 2
	cats, videos, stats, err := eng.Crawl(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cats) != 3 {
		t.Fatalf("cats=%d", len(cats))
	}
	if stats.CategoryPages < 4 {
		t.Fatalf("category pages=%d stats=%#v", stats.CategoryPages, stats)
	}
	if len(videos) < 10 {
		t.Fatalf("videos=%d", len(videos))
	}
	var series *maccms.DetailMeta
	for i := range videos {
		if videos[i].VodID == "20001" {
			series = &videos[i]
			break
		}
	}
	if series == nil || len(series.Episodes) < 3 {
		t.Fatalf("series %#v", series)
	}
}

func TestHTTPFetcherRetriesTransient(t *testing.T) {
	f := maccms.NewHTTPFetcher(time.Second, 0, 2, "test")
	_, err := f.Fetch(context.Background(), "http://127.0.0.1:1/")
	if err == nil {
		t.Fatal("expected error")
	}
}
