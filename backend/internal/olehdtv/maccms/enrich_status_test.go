package maccms_test

import (
	"context"
	"testing"

	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
)

// Ensures fetch/parse misses stay retryable (EnrichmentSkipped), not terminal unavailable.
func TestEnrichEpisodesBounded_parseMissIsRetryable(t *testing.T) {
	mock := &mockFetcher{byURL: map[string]string{
		"/": recentHomeHTML(),
		"/index.php/vod/type/id/1.html": listingHTML("9001"),
		"/index.php/vod/type/id/2.html": listingHTML(),
		"/index.php/vod/type/id/3.html": listingHTML(),
		"/index.php/vod/type/id/4.html": listingHTML(),
		"/index.php/vod/type/id/5.html": listingHTML(),
		"/index.php/vod/type/id/6.html": listingHTML(),
		"/index.php/vod/detail/id/9001.html": `
<html><body><div class="myui-content__detail"><h1 class="title">T</h1>
<img data-original="https://static.olelive.com/upload/vod/9001.jpg"/>
</div>
<div class="playlist">
<a href="/index.php/vod/play/id/9001/sid/1/nid/1.html">第1集</a>
</div></body></html>`,
		// Live-shaped: no semicolon — must parse after player.go fix.
		"/index.php/vod/play/id/9001/sid/1/nid/1.html": `var player_aaaa={"flag":"play","encrypt":0,"url":"https:\/\/cdn.example\/9001.m3u8","id":"9001","sid":1,"nid":1}</script>`,
	}}
	eng := maccms.NewEngine(mock, "https://maccms.local")
	res, err := eng.CrawlRecent(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || len(res.Items[0].Episodes) < 1 {
		t.Fatalf("%#v", res.Items)
	}
	ep := res.Items[0].Episodes[0]
	if ep.StreamURL != "https://cdn.example/9001.m3u8" || !ep.Available {
		t.Fatalf("expected HLS from no-semicolon player: %#v", ep)
	}
	if ep.EnrichmentSkipped {
		t.Fatal("successful parse should not be EnrichmentSkipped")
	}
}

func TestEnrichEpisodesBounded_fetchFailPendingNotUnavailable(t *testing.T) {
	mock := &mockFetcher{byURL: map[string]string{
		"/":                                 recentHomeHTML(),
		"/index.php/vod/type/id/1.html":     listingHTML("9002"),
		"/index.php/vod/type/id/2.html":     listingHTML(),
		"/index.php/vod/type/id/3.html":     listingHTML(),
		"/index.php/vod/type/id/4.html":     listingHTML(),
		"/index.php/vod/type/id/5.html":     listingHTML(),
		"/index.php/vod/type/id/6.html":     listingHTML(),
		"/index.php/vod/detail/id/9002.html": detailHTML("9002", 1),
		// play URL intentionally missing from mock → fetch error
	}}
	eng := maccms.NewEngine(mock, "https://maccms.local")
	res, err := eng.CrawlRecent(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) == 0 || len(res.Items[0].Episodes) == 0 {
		t.Fatal("expected episode")
	}
	ep := res.Items[0].Episodes[0]
	if !ep.EnrichmentSkipped {
		t.Fatalf("fetch miss should EnrichmentSkipped: %#v", ep)
	}
	if ep.StreamURL != "" {
		t.Fatalf("unexpected stream: %#v", ep)
	}
}
