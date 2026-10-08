package mediamigrate

import (
	"strings"
	"testing"
)

func TestResolvePlaylistURL_relativeAndAbsolute(t *testing.T) {
	base := "https://cdn.source.example/vod/12/index.m3u8?token=SECRET"
	rel, err := ResolvePlaylistURL(base, "segment0.ts")
	if err != nil {
		t.Fatal(err)
	}
	if rel != "https://cdn.source.example/vod/12/segment0.ts" {
		t.Fatalf("relative got %q", rel)
	}
	abs, err := ResolvePlaylistURL(base, "https://other.example/a/b.ts?x=1")
	if err != nil {
		t.Fatal(err)
	}
	if abs != "https://other.example/a/b.ts?x=1" {
		t.Fatalf("absolute got %q", abs)
	}
}

func TestDetectAndParseMediaPlaylist(t *testing.T) {
	body := `#EXTM3U
#EXT-X-TARGETDURATION:4
#EXTINF:4.0,
seg0.ts
#EXTINF:4.0,
https://abs.example/p/seg1.ts
#EXT-X-ENDLIST
`
	p, err := ParseMediaPlaylist("https://cdn.source.example/vod/index.m3u8", body)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != PlaylistMedia {
		t.Fatalf("kind %s", p.Kind)
	}
	if len(p.Segments) != 2 {
		t.Fatalf("segments %d", len(p.Segments))
	}
	if p.Segments[0].SourceURL != "https://cdn.source.example/vod/seg0.ts" {
		t.Fatalf("seg0 %q", p.Segments[0].SourceURL)
	}
	if p.Segments[1].SourceURL != "https://abs.example/p/seg1.ts" {
		t.Fatalf("seg1 %q", p.Segments[1].SourceURL)
	}
	rewritten, err := RewriteMediaPlaylist(body, p.Segments)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rewritten, "https://") {
		t.Fatalf("rewritten still has absolute URLs:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, "segment000.ts") || !strings.Contains(rewritten, "segment001.ts") {
		t.Fatalf("missing local names:\n%s", rewritten)
	}
}

func TestParseMasterPlaylist(t *testing.T) {
	body := `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=800000
720p.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=400000
https://cdn.source.example/vod/360p/index.m3u8
`
	p, err := ParseMediaPlaylist("https://cdn.source.example/vod/master.m3u8", body)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != PlaylistMaster {
		t.Fatalf("kind %s", p.Kind)
	}
	if len(p.MasterRefs) != 2 {
		t.Fatalf("refs %d", len(p.MasterRefs))
	}
}

func TestRedactURL_stripsQuery(t *testing.T) {
	got := RedactURL("https://h.example/a/b.m3u8?token=SECRET&sig=1")
	if strings.Contains(got, "SECRET") || strings.Contains(got, "?") {
		t.Fatalf("leaked query: %q", got)
	}
}
