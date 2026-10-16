package play

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/media"
	"github.com/tycdn/vplayer/internal/model"
)

func TestEligibleFallbackURL(t *testing.T) {
	primary := "https://media.example.com/dongman/videos/1/2/index.m3u8"
	src := "https://source.example/v/index.m3u8"

	cases := []struct {
		name    string
		fromKey bool
		src     string
		status  string
		primary string
		want    string
	}{
		{name: "ok", fromKey: true, src: src, status: "ok", primary: primary, want: src},
		{name: "available", fromKey: true, src: src, status: "available", primary: primary, want: src},
		{name: "empty_status", fromKey: true, src: src, status: "", primary: primary, want: src},
		{name: "unavailable", fromKey: true, src: src, status: "unavailable", primary: primary, want: ""},
		{name: "member", fromKey: true, src: src, status: "member", primary: primary, want: ""},
		{name: "vip", fromKey: true, src: src, status: "vip", primary: primary, want: ""},
		{name: "restricted", fromKey: true, src: src, status: "restricted", primary: primary, want: ""},
		{name: "forbidden", fromKey: true, src: src, status: "forbidden", primary: primary, want: ""},
		{name: "unknown_status", fromKey: true, src: src, status: "pending_enrichment", primary: primary, want: ""},
		{name: "empty_source", fromKey: true, src: "", status: "ok", primary: primary, want: ""},
		{name: "non_http_source", fromKey: true, src: "ftp://x/a.m3u8", status: "ok", primary: primary, want: ""},
		{name: "relative_source", fromKey: true, src: "videos/1/2/index.m3u8", status: "ok", primary: primary, want: ""},
		{name: "no_object_key", fromKey: false, src: src, status: "ok", primary: src, want: ""},
		{name: "same_as_primary", fromKey: true, src: primary, status: "ok", primary: primary, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EligibleFallbackURL(tc.fromKey, tc.src, tc.status, tc.primary)
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func playTestCfg() config.Config {
	return config.Config{
		CDNSignSecret:    "s",
		MediaCDNBaseURL:  "https://media.example.com",
		CDNBaseURL:       "https://media.example.com",
		R2Bucket:         "dongman",
		MediaCDNPathMode: media.CDNPathModeBucketPrefix,
	}
}

func TestResolver_Resolve_fallbackPresentStatuses(t *testing.T) {
	src := "https://source.example/v/index.m3u8"
	for _, status := range []string{"ok", "available", ""} {
		status := status
		t.Run("status_"+status, func(t *testing.T) {
			r := &Resolver{
				Cfg: playTestCfg(),
				VideoByID: func(id uint64) (*model.Video, error) {
					return &model.Video{ID: id, Status: "ready", PlaySID: 1, PlayNID: 1}, nil
				},
				DefaultEpisode: func(videoID uint64) (*model.Episode, error) {
					return &model.Episode{
						ID: 2, VideoID: videoID, SID: 1, NID: 1,
						PlaybackURL: src, PlaybackStatus: status,
						HLSObjectKey: "videos/1/2/index.m3u8", PlaybackSource: "plyr",
					}, nil
				},
			}
			info, err := r.Resolve(1, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			wantPrimary := "https://media.example.com/dongman/videos/1/2/index.m3u8"
			if info.URL != wantPrimary {
				t.Fatalf("primary=%q want %q", info.URL, wantPrimary)
			}
			if info.FallbackURL != src {
				t.Fatalf("fallback=%q want %q", info.FallbackURL, src)
			}
		})
	}
}

func TestResolver_Resolve_fallbackAbsentForRestrictedStatuses(t *testing.T) {
	src := "https://source.example/v/index.m3u8"
	for _, status := range []string{"unavailable", "member", "vip", "restricted", "forbidden", "pending_enrichment", "weird"} {
		status := status
		t.Run(status, func(t *testing.T) {
			r := &Resolver{
				Cfg: playTestCfg(),
				VideoByID: func(id uint64) (*model.Video, error) {
					return &model.Video{ID: id, Status: "ready"}, nil
				},
				DefaultEpisode: func(videoID uint64) (*model.Episode, error) {
					return &model.Episode{
						ID: 2, VideoID: videoID, SID: 1, NID: 1,
						PlaybackURL: src, PlaybackStatus: status,
						HLSObjectKey: "videos/1/2/index.m3u8",
					}, nil
				},
			}
			info, err := r.Resolve(1, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			if info.FallbackURL != "" {
				t.Fatalf("status %q must not emit fallback, got %q", status, info.FallbackURL)
			}
		})
	}
}

func TestResolver_Resolve_fallbackAbsentNoKeyOrBadSource(t *testing.T) {
	t.Run("source_primary", func(t *testing.T) {
		src := "https://source.example/v/index.m3u8"
		r := &Resolver{
			Cfg: playTestCfg(),
			VideoByID: func(id uint64) (*model.Video, error) {
				return &model.Video{ID: id, Status: "ready"}, nil
			},
			DefaultEpisode: func(videoID uint64) (*model.Episode, error) {
				return &model.Episode{
					ID: 2, VideoID: videoID, SID: 1, NID: 1,
					PlaybackURL: src, PlaybackStatus: "ok",
				}, nil
			},
		}
		info, err := r.Resolve(1, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if info.URL != src {
			t.Fatalf("url=%q", info.URL)
		}
		if info.FallbackURL != "" {
			t.Fatalf("fallback=%q", info.FallbackURL)
		}
	})
	t.Run("empty_playback_url", func(t *testing.T) {
		r := &Resolver{
			Cfg: playTestCfg(),
			VideoByID: func(id uint64) (*model.Video, error) {
				return &model.Video{ID: id, Status: "ready"}, nil
			},
			DefaultEpisode: func(videoID uint64) (*model.Episode, error) {
				return &model.Episode{
					ID: 2, VideoID: videoID, SID: 1, NID: 1,
					PlaybackStatus: "ok", HLSObjectKey: "videos/1/2/index.m3u8",
				}, nil
			},
		}
		info, err := r.Resolve(1, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if info.FallbackURL != "" {
			t.Fatalf("fallback=%q", info.FallbackURL)
		}
	})
	t.Run("explicit_sid_nid_uses_status", func(t *testing.T) {
		src := "https://source.example/v/index.m3u8"
		r := &Resolver{
			Cfg: playTestCfg(),
			VideoByID: func(id uint64) (*model.Video, error) {
				return &model.Video{ID: id, Status: "ready"}, nil
			},
			FindEpisode: func(videoID uint64, sid, nid int) (*model.Episode, error) {
				if sid != 2 || nid != 3 {
					t.Fatalf("sid/nid=%d/%d", sid, nid)
				}
				return &model.Episode{
					ID: 9, VideoID: videoID, SID: sid, NID: nid,
					PlaybackURL: src, PlaybackStatus: "member",
					HLSObjectKey: "videos/1/9/index.m3u8",
				}, nil
			},
		}
		info, err := r.Resolve(1, 2, 3)
		if err != nil {
			t.Fatal(err)
		}
		if info.FallbackURL != "" {
			t.Fatalf("member must block fallback")
		}
		if !strings.Contains(info.URL, "videos/1/9/index.m3u8") {
			t.Fatalf("url=%q", info.URL)
		}
	})
}

func TestResolver_Resolve_preservesVideoURLWhenEpisodeURLEmpty(t *testing.T) {
	videoURL := "https://video-level.example/play.m3u8"
	r := &Resolver{
		Cfg: playTestCfg(),
		VideoByID: func(id uint64) (*model.Video, error) {
			return &model.Video{
				ID: id, Status: "ready",
				PlaybackURL: videoURL, PlaybackSource: "video",
			}, nil
		},
		DefaultEpisode: func(videoID uint64) (*model.Episode, error) {
			return &model.Episode{
				ID: 2, VideoID: videoID, SID: 1, NID: 1,
				PlaybackURL: "", PlaybackStatus: "ok",
				HLSObjectKey: "", PlaybackSource: "ep",
			}, nil
		},
	}
	info, err := r.Resolve(1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if info.URL != videoURL {
		t.Fatalf("primary=%q want video-level %q", info.URL, videoURL)
	}
	if info.FallbackURL != "" {
		t.Fatalf("source-only must not emit fallback, got %q", info.FallbackURL)
	}
}

func TestResolver_Resolve_restrictedEpisodeBlocksFallbackDespiteVideoMasterKey(t *testing.T) {
	src := "https://source.example/restricted.m3u8"
	for _, status := range []string{"member", "vip", "restricted"} {
		status := status
		t.Run(status, func(t *testing.T) {
			r := &Resolver{
				Cfg: playTestCfg(),
				VideoByID: func(id uint64) (*model.Video, error) {
					return &model.Video{
						ID: id, Status: "ready",
						HLSMasterKey: "videos/9/9/index.m3u8",
					}, nil
				},
				DefaultEpisode: func(videoID uint64) (*model.Episode, error) {
					return &model.Episode{
						ID: 9, VideoID: videoID, SID: 1, NID: 1,
						PlaybackURL: src, PlaybackStatus: status,
						HLSObjectKey: "", // empty → video master key becomes primary
					}, nil
				},
			}
			info, err := r.Resolve(1, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			wantPrimary := "https://media.example.com/dongman/videos/9/9/index.m3u8"
			if info.URL != wantPrimary {
				t.Fatalf("primary=%q want %q", info.URL, wantPrimary)
			}
			if info.FallbackURL != "" {
				t.Fatalf("restricted status %q must not expose source as fallback", status)
			}
		})
	}
}

func TestEpisodeQuerySQLIncludesPlaybackStatus(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	dir := filepath.Dir(thisFile)
	resolverSrc, err := os.ReadFile(filepath.Join(dir, "resolver.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resolverSrc), "COALESCE(playback_status,'')") {
		t.Fatal("findEpisode must SELECT playback_status")
	}
	storeSrc, err := os.ReadFile(filepath.Join(dir, "..", "store", "sync.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(storeSrc), "func (m *MySQL) GetDefaultEpisode") {
		t.Fatal("GetDefaultEpisode missing")
	}
	idx := strings.Index(string(storeSrc), "func (m *MySQL) GetDefaultEpisode")
	chunk := string(storeSrc)[idx:]
	if end := strings.Index(chunk[1:], "\nfunc "); end > 0 {
		chunk = chunk[:end+1]
	}
	if !strings.Contains(chunk, "COALESCE(playback_status,'')") {
		t.Fatal("GetDefaultEpisode must SELECT playback_status")
	}
}
