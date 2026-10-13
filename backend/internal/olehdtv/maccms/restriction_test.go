package maccms_test

import (
	"testing"

	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
)

func TestIsSourceRestrictedPage_confirmedMembership(t *testing.T) {
	html := `<html><body><div class="mac_msg_l">
系统提示
您没有权限访问此数据，请升级会员
</div></body></html>`
	if !maccms.IsSourceRestrictedPage(html) {
		t.Fatal("expected source_restricted")
	}
	if maccms.ClassifyPlayPage(html) != maccms.PlayPageSourceRestricted {
		t.Fatalf("class=%q", maccms.ClassifyPlayPage(html))
	}
}

func TestIsSourceRestrictedPage_unrelatedChineseNotRestricted(t *testing.T) {
	cases := []string{
		`<html><body>精彩内容 即将上线 系统维护中</body></html>`,
		`<html><body>请升级浏览器以获得更好体验</body></html>`,
		`<html><body>系统提示：页面加载失败，请稍后重试</body></html>`,
		`var player_aaaa={"url":"https://cdn.example/a.m3u8","encrypt":0,"id":1}</script>`,
	}
	for _, html := range cases {
		if maccms.IsSourceRestrictedPage(html) {
			t.Fatalf("false positive: %q", html)
		}
	}
}

func TestClassifyPlayPage_playerAAAAAndUnknown(t *testing.T) {
	okHTML := `var player_aaaa={"flag":"play","encrypt":0,"url":"https://cdn.example/x.m3u8","from":"plyr","id":"1","sid":1,"nid":1}</script>`
	if maccms.ClassifyPlayPage(okHTML) != maccms.PlayPageOK {
		t.Fatalf("want ok, got %q", maccms.ClassifyPlayPage(okHTML))
	}
	unknown := `<html><body><div>play page chrome only</div></body></html>`
	if maccms.ClassifyPlayPage(unknown) != maccms.PlayPageUnknown {
		t.Fatalf("want unknown, got %q", maccms.ClassifyPlayPage(unknown))
	}
	// Missing player_aaaa alone is not source_restricted.
	if maccms.IsSourceRestrictedPage(unknown) {
		t.Fatal("unknown must not be restricted")
	}
}

func TestChooseContentPoster_siteCatalogYieldsToVodDetail(t *testing.T) {
	site := "https://static.olelive.com/upload/site/20201117-1/d2035d18801242fc62991b32d6366bf8.png"
	vod := "https://static.olelive.com/upload/vod/2026/real.jpg"
	if maccms.IsValidContentPoster(site) {
		t.Fatal("/upload/site/ must be invalid")
	}
	if !maccms.IsValidContentPoster(vod) {
		t.Fatal("/upload/vod/ must remain valid")
	}
	got := maccms.ChooseContentPoster(vod, site)
	if got != vod {
		t.Fatalf("got %q want vod detail", got)
	}
	if got := maccms.ChooseContentPoster("", site); got != "" {
		t.Fatalf("invalid catalog alone should yield empty, got %q", got)
	}
}
