package olehdtv_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tycdn/vplayer/internal/olehdtv"
)

func TestFixtureNormalizesKnownRecord(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "olehdtv_sample.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	snap, err := olehdtv.ParseFixtureJSON(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(snap.Categories) < 1 {
		t.Fatal("expected categories")
	}
	var movie *olehdtv.SourceCategory
	for i := range snap.Categories {
		if snap.Categories[i].SourceID == "1" {
			movie = &snap.Categories[i]
			break
		}
	}
	if movie == nil || movie.Name != "电影" {
		t.Fatalf("expected category 电影, got %#v", movie)
	}

	if len(snap.Videos) < 8 {
		t.Fatalf("expected multi-video fixture, got %d", len(snap.Videos))
	}
	var v *olehdtv.SourceVideo
	for i := range snap.Videos {
		if snap.Videos[i].SourceID == "82861" {
			v = &snap.Videos[i]
			break
		}
	}
	if v == nil {
		t.Fatal("missing known source_id 82861")
	}
	if v.Title != "功夫女足" {
		t.Fatalf("title=%s", v.Title)
	}
	if v.SourceCategoryID != "1" {
		t.Fatalf("category=%s", v.SourceCategoryID)
	}
	if v.PosterURL == "" || v.Year != "2026" || v.Area != "香港" {
		t.Fatalf("metadata incomplete: %#v", v)
	}
	if len(v.Episodes) < 1 {
		t.Fatalf("episodes=%d", len(v.Episodes))
	}
	ep := v.Episodes[0]
	if ep.SID != 1 || ep.NID != 1 {
		t.Fatalf("sid/nid=%d/%d", ep.SID, ep.NID)
	}
	if ep.PlaybackURL == "" || !contains(ep.PlaybackURL, ".m3u8") {
		t.Fatalf("playback_url not hls: %s", ep.PlaybackURL)
	}

	// Multi-episode sample must remain present for sync/UI coverage.
	var series *olehdtv.SourceVideo
	for i := range snap.Videos {
		if snap.Videos[i].SourceID == "91001" {
			series = &snap.Videos[i]
			break
		}
	}
	if series == nil || len(series.Episodes) < 3 {
		t.Fatalf("expected multi-episode series 91001, got %#v", series)
	}
}

func TestParseMacPlayURL(t *testing.T) {
	eps := olehdtv.ParseMacPlayURL("plyr$$$qq", "立即播放$https://a.com/a.m3u8#二$https://a.com/b.m3u8$$$x$https://b.com/c.m3u8")
	if len(eps) != 3 {
		t.Fatalf("len=%d", len(eps))
	}
	if eps[0].SID != 1 || eps[0].NID != 1 || eps[0].PlaybackSource != "plyr" {
		t.Fatalf("%#v", eps[0])
	}
	if eps[2].SID != 2 || eps[2].NID != 1 {
		t.Fatalf("%#v", eps[2])
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}
