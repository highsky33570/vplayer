package maccms_test

import (
	"strings"
	"testing"

	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
)

func TestParsePlayerAAAA_noSemicolonBeforeScript(t *testing.T) {
	html := `<html><body><script type="text/javascript">var player_aaaa={"flag":"play","encrypt":0,"url":"https:\/\/example.com\/ts\/master.m3u8","from":"plyr","id":"37782","sid":1,"nid":1}</script></body></html>`
	p, ok := maccms.ParsePlayerAAAA(html)
	if !ok {
		t.Fatal("expected parse ok")
	}
	if p.URL != "https://example.com/ts/master.m3u8" {
		t.Fatalf("url=%q", p.URL)
	}
	if int(p.NID) != 1 || int(p.ID) != 37782 {
		t.Fatalf("ids sid=%d nid=%d id=%d", p.SID, p.NID, p.ID)
	}
	ep := maccms.EpisodeRef{PlayURL: "/play"}
	maccms.ApplyPlayerToEpisode(&ep, p)
	if !ep.Available || ep.StreamURL != "https://example.com/ts/master.m3u8" {
		t.Fatalf("episode %#v", ep)
	}
}

func TestParsePlayerAAAA_semicolonStillWorks(t *testing.T) {
	html := `var player_aaaa={"flag":"play","encrypt":0,"url":"https://cdn.example/a.m3u8","nid":2,"id":1};`
	p, ok := maccms.ParsePlayerAAAA(html)
	if !ok || p.URL != "https://cdn.example/a.m3u8" || int(p.NID) != 2 {
		t.Fatalf("%v %#v", ok, p)
	}
}

func TestParsePlayerAAAA_escapedSlashes(t *testing.T) {
	raw, ok := maccms.ExtractPlayerAAAAJSON(`player_aaaa={"url":"https:\/\/cloud.example\/x.m3u8","encrypt":0,"id":9}`)
	if !ok {
		t.Fatal("extract failed")
	}
	if !strings.Contains(raw, `https://cloud.example/x.m3u8`) && !strings.Contains(raw, `https:\/\/cloud.example\/x.m3u8`) {
		// After json.Unmarshal, URL is unescaped; extract may still have escapes.
		t.Logf("raw=%s", raw)
	}
	p, ok := maccms.ParsePlayerAAAA(`var player_aaaa={"url":"https:\/\/cloud.example\/x.m3u8","encrypt":0,"id":9}</script>`)
	if !ok || p.URL != "https://cloud.example/x.m3u8" {
		t.Fatalf("%v url=%q", ok, p)
	}
}

func TestParsePlayerAAAA_malformedRejected(t *testing.T) {
	cases := []string{
		``,
		`<script>var x=1</script>`,
		`var player_aaaa={url: broken`,
		`var player_aaaa={"url":"https://x","encrypt":0,`, // unclosed
		`player_aaaa=null`,
	}
	for _, c := range cases {
		if p, ok := maccms.ParsePlayerAAAA(c); ok {
			t.Fatalf("expected reject for %q got %#v", c, p)
		}
	}
}

func TestParsePlayerAAAA_endOfInput(t *testing.T) {
	html := `var player_aaaa={"encrypt":0,"url":"https://e/end.m3u8","id":3,"nid":1}`
	p, ok := maccms.ParsePlayerAAAA(html)
	if !ok || p.URL != "https://e/end.m3u8" {
		t.Fatalf("%v %#v", ok, p)
	}
}
