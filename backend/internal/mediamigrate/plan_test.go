package mediamigrate

import (
	"strings"
	"testing"

	"github.com/tycdn/vplayer/internal/media"
	"github.com/tycdn/vplayer/internal/store"
)

func TestBuildPlan_authorizedHLS(t *testing.T) {
	cdn := media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}
	plan := BuildPlan(store.MigratableEpisode{
		ID: 456, VideoID: 123, SID: 1, NID: 1,
		PlaybackURL: "https://source.example/a/b/index.m3u8", PlaybackStatus: "ok",
	}, cdn)
	if plan.SkipReason != "" {
		t.Fatalf("unexpected skip: %s", plan.SkipReason)
	}
	if plan.ProposedR2Key != "videos/123/456/index.m3u8" {
		t.Fatalf("key %q", plan.ProposedR2Key)
	}
	if plan.ProposedCDNURL != "https://media.example.com/dongman/videos/123/456/index.m3u8" {
		t.Fatalf("cdn %q", plan.ProposedCDNURL)
	}
	if !strings.Contains(plan.ExpectedDB, "playback_url preserved") {
		t.Fatalf("db plan %q", plan.ExpectedDB)
	}
	if plan.SourceHost != "source.example" {
		t.Fatalf("host %q", plan.SourceHost)
	}
}

func TestBuildPlan_skipsUnavailable(t *testing.T) {
	cdn := media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}
	plan := BuildPlan(store.MigratableEpisode{
		ID: 1, VideoID: 1, PlaybackURL: "https://x/y.m3u8", PlaybackStatus: "member",
	}, cdn)
	if plan.SkipReason != "unauthorized_or_unavailable" {
		t.Fatalf("skip %q", plan.SkipReason)
	}
}
