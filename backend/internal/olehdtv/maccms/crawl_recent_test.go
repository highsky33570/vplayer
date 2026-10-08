package maccms_test

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
)

type mockFetcher struct {
	mu    sync.Mutex
	urls  []string
	byURL map[string]string
}

func (m *mockFetcher) Fetch(ctx context.Context, pageURL string) (string, error) {
	_ = ctx
	m.mu.Lock()
	m.urls = append(m.urls, pageURL)
	m.mu.Unlock()
	u, err := url.Parse(pageURL)
	if err != nil {
		return "", err
	}
	key := u.Path
	if key == "" {
		key = "/"
	}
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

func (m *mockFetcher) requested() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := append([]string(nil), m.urls...)
	return out
}

func (m *mockFetcher) hasPathSubstring(sub string) bool {
	for _, u := range m.requested() {
		if strings.Contains(strings.ToLower(u), strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

func (m *mockFetcher) countPathSubstring(sub string) int {
	n := 0
	for _, u := range m.requested() {
		if strings.Contains(strings.ToLower(u), strings.ToLower(sub)) {
			n++
		}
	}
	return n
}

func listingHTML(ids ...string) string {
	var b strings.Builder
	b.WriteString(`<ul class="myui-vodlist">`)
	for _, id := range ids {
		fmt.Fprintf(&b, `<li><a href="/index.php/vod/detail/id/%s.html" title="V%s">V%s</a></li>`, id, id, id)
	}
	b.WriteString(`</ul><div class="page"><a class="page-next" href="/index.php/vod/type/id/1/page/2.html">下一页</a></div>`)
	return b.String()
}

func detailHTML(vodID string, episodeCount int) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<html><body><h1>Title %s</h1><div class="playlist">`, vodID)
	for n := 1; n <= episodeCount; n++ {
		fmt.Fprintf(&b, `<a href="/index.php/vod/play/id/%s/sid/1/nid/%d.html">第%d集</a>`, vodID, n, n)
	}
	b.WriteString(`</div></body></html>`)
	return b.String()
}

func playHTML(vodID string, nid int) string {
	return fmt.Sprintf(`<html><body><script>var player_aaaa={url:'https://cdn.example/%s/n%d.m3u8',from:'demo',sid:1,nid:%d,id:%s,encrypt:0};</script></body></html>`, vodID, nid, nid, vodID)
}

func recentHomeHTML() string {
	return `<html><body><ul class="nav">
<a href="/index.php/vod/type/id/1.html">电影</a>
<a href="/index.php/vod/type/id/2.html">连续剧</a>
<a href="/index.php/vod/type/id/3.html">综艺</a>
<a href="/index.php/vod/type/id/4.html">动漫</a>
<a href="/index.php/vod/type/id/5.html">午夜影院</a>
<a href="/index.php/vod/type/id/6.html">VIP蓝光影院</a>
<a href="/index.php/vod/type/id/7.html">体育直播</a>
<a href="/index.php/vod/type/id/8.html">短剧</a>
<a href="/">首页</a>
</ul></body></html>`
}

func buildRecentMock(t *testing.T) *mockFetcher {
	t.Helper()
	m := &mockFetcher{byURL: map[string]string{
		"/":           recentHomeHTML(),
		"/index.html": recentHomeHTML(),
		"/index.php/vod/type/id/1.html": listingHTML("101", "102", "dup"),
		"/index.php/vod/type/id/2.html": listingHTML("201", "dup", "202"),
		"/index.php/vod/type/id/3.html": listingHTML("301"),
		"/index.php/vod/type/id/4.html": listingHTML("401"),
		"/index.php/vod/type/id/5.html": listingHTML("501"),
		"/index.php/vod/type/id/6.html": listingHTML("601"),
		// page 2 exists in mock but must never be requested
		"/index.php/vod/type/id/1/page/2.html": listingHTML("999"),
	}}
	for _, id := range []string{"101", "102", "dup", "201", "202", "301", "401", "501", "601"} {
		eps := 4
		if id == "201" {
			eps = 50 // series — play enrichment must cap
		}
		m.byURL["/index.php/vod/detail/id/"+id+".html"] = detailHTML(id, eps)
		for n := 1; n <= eps; n++ {
			m.byURL[fmt.Sprintf("/index.php/vod/play/id/%s/sid/1/nid/%d.html", id, n)] = playHTML(id, n)
		}
	}
	return m
}

func TestValidateRecentLimit(t *testing.T) {
	if err := maccms.ValidateRecentLimit(0); err == nil {
		t.Fatal("expected reject <=0")
	}
	if err := maccms.ValidateRecentLimit(21); err == nil {
		t.Fatal("expected reject >20")
	}
	if err := maccms.ValidateRecentLimit(10); err != nil {
		t.Fatal(err)
	}
}

func TestClampRecentHTTPSettings(t *testing.T) {
	d, c, r := maccms.ClampRecentHTTPSettings(100, 16, 0)
	if d < 1000 || c > 2 || r <= 0 {
		t.Fatalf("clamp failed delay=%d conc=%d retries=%d", d, c, r)
	}
}

func TestFilterRecentCategoriesExcludesSportsShort(t *testing.T) {
	cats := maccms.ParseNavigationCategories(recentHomeHTML(), "https://maccms.local")
	filtered := maccms.FilterRecentCategories(cats)
	names := map[string]bool{}
	for _, c := range filtered {
		names[c.Name] = true
		if c.Name == "体育直播" || c.Name == "短剧" {
			t.Fatalf("excluded category leaked: %s", c.Name)
		}
	}
	for _, want := range []string{"电影", "连续剧", "综艺", "动漫", "午夜影院", "VIP蓝光影院"} {
		if !names[want] {
			t.Fatalf("missing %s in %#v", want, names)
		}
	}
}

func TestCrawlRecent_noPage2_andBounds(t *testing.T) {
	mock := buildRecentMock(t)
	eng := maccms.NewEngine(mock, "https://maccms.local")
	eng.Concurrency = 8 // must clamp internally to <=2 behavior for safety; CrawlRecent clamps workers
	eng.FetchPlay = true // must not cause unbounded fan-out; CrawlRecent uses bounded enrich
	res, err := eng.CrawlRecent(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if mock.hasPathSubstring("/page/2") {
		t.Fatalf("page 2 requested: %v", mock.requested())
	}
	if res.Selected > 10 || len(res.Items) > 10 {
		t.Fatalf("selected=%d items=%d", res.Selected, len(res.Items))
	}
	if res.Budget.DetailRequests > 10 {
		t.Fatalf("detail requests=%d", res.Budget.DetailRequests)
	}
	if res.Budget.PlayRequests > maccms.RecentMaxPlayTotal {
		t.Fatalf("play requests=%d", res.Budget.PlayRequests)
	}
	if res.Budget.PlayRequests > 20 {
		t.Fatalf("play >20: %d", res.Budget.PlayRequests)
	}
	// Dedup: "dup" appears in cat1 and cat2 — only one detail.
	dupDetails := mock.countPathSubstring("/vod/detail/id/dup.html")
	if dupDetails > 1 {
		t.Fatalf("dup detail fetched %d times", dupDetails)
	}
	// Per-video play cap: series 201 has 50 eps — at most 3 play requests for that id.
	play201 := 0
	for _, u := range mock.requested() {
		if strings.Contains(u, "/vod/play/id/201/") {
			play201++
		}
	}
	if play201 > maccms.RecentMaxPlayPerVideo {
		t.Fatalf("per-video play=%d", play201)
	}
	if !res.Budget.EnrichmentCapped && play201 > 0 && play201 < 50 {
		// series selected and capped
		if res.Selected > 0 {
			// enrichment capped expected when 201 selected
			for _, it := range res.Items {
				if it.VodID == "201" && !res.EnrichmentCapped {
					t.Fatal("expected enrichment_capped for 50-ep series")
				}
			}
		}
	}
	if !strings.Contains(res.OrderingNote, "best_effort") {
		t.Fatalf("ordering note: %s", res.OrderingNote)
	}
	// Rejected categories must not be listed.
	for _, u := range mock.requested() {
		if strings.Contains(u, "/type/id/7.html") || strings.Contains(u, "/type/id/8.html") {
			t.Fatalf("excluded category listed: %s", u)
		}
	}
}

func TestCrawlRecent_limitReject(t *testing.T) {
	mock := buildRecentMock(t)
	eng := maccms.NewEngine(mock, "https://maccms.local")
	_, err := eng.CrawlRecent(context.Background(), 21)
	if err == nil {
		t.Fatal("expected limit reject")
	}
	if len(mock.requested()) != 0 {
		t.Fatalf("should not HTTP on reject: %v", mock.requested())
	}
}

func TestCrawlRecent_doesNotCallFullCrawlPagination(t *testing.T) {
	mock := buildRecentMock(t)
	eng := maccms.NewEngine(mock, "https://maccms.local")
	eng.MaxPages = 500
	_, err := eng.CrawlRecent(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	listing := 0
	for _, u := range mock.requested() {
		if strings.Contains(u, "/vod/type/") {
			listing++
			if strings.Contains(u, "/page/") {
				t.Fatalf("paginated listing: %s", u)
			}
		}
	}
	if listing > 6 {
		t.Fatalf("too many listing requests: %d", listing)
	}
}

func TestEngineCrawl_stillPaginatesUnchanged(t *testing.T) {
	// Existing full Crawl() must still walk page 2+ (regression guard).
	root := fixtureRoot(t)
	eng := maccms.NewEngine(&maccms.FileFetcher{Root: root, BaseURL: "https://maccms.local"}, "https://maccms.local")
	_, _, stats, err := eng.Crawl(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.CategoryPages < 4 {
		t.Fatalf("full Crawl pagination broken: %#v", stats)
	}
}
