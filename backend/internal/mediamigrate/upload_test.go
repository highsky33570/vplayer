package mediamigrate

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tycdn/vplayer/internal/media"
	"github.com/tycdn/vplayer/internal/store"
)

func migrateHTTPClient() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		u := r.URL.String()
		var body string
		switch {
		case strings.HasSuffix(u, "index.m3u8"):
			body = mediaPlaylistBody()
		case strings.HasSuffix(u, "segA.ts") || strings.HasSuffix(u, "segB.ts"):
			body = "TSDATA"
		default:
			return &http.Response{StatusCode: 404, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}
}

func TestEnsureObject_skipsMatchingHeadSize(t *testing.T) {
	obj := NewMemoryObjectStore()
	obj.Objects["videos/12/53/segment000.ts"] = []byte("TSDATA")
	obj.Objects["videos/12/53/segment001.ts"] = []byte("TSDATA")
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	m := &Migrator{Store: obj, Episodes: eps, HTTP: migrateHTTPClient(), CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}}
	res := m.MigrateEpisode(context.Background(), store.MigratableEpisode{
		ID: 53, VideoID: 12, PlaybackURL: "https://src.example/v/index.m3u8", PlaybackStatus: "ok",
	})
	if res.Error != "" {
		t.Fatalf("error %s", res.Error)
	}
	if obj.PutCalls != 1 {
		t.Fatalf("expected 1 put (playlist only), got %d", obj.PutCalls)
	}
	if eps.migrated != 1 {
		t.Fatalf("db not migrated %#v", eps)
	}
}

func TestEnsureObject_reuploadsSizeMismatch(t *testing.T) {
	obj := NewMemoryObjectStore()
	obj.Objects["videos/12/53/segment000.ts"] = []byte("WRONG")
	obj.Objects["videos/12/53/segment001.ts"] = []byte("TSDATA")
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	m := &Migrator{Store: obj, Episodes: eps, HTTP: migrateHTTPClient(), CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}}
	res := m.MigrateEpisode(context.Background(), store.MigratableEpisode{
		ID: 53, VideoID: 12, PlaybackURL: "https://src.example/v/index.m3u8", PlaybackStatus: "ok",
	})
	if res.Error != "" {
		t.Fatalf("error %s", res.Error)
	}
	if string(obj.Objects["videos/12/53/segment000.ts"]) != "TSDATA" {
		t.Fatalf("segment000 not corrected")
	}
}

func TestMigrateEpisode_noDBSwitchWhenPlaylistMissingAfterSegments(t *testing.T) {
	obj := NewMemoryObjectStore()
	obj.FailPut = "index.m3u8"
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	m := &Migrator{Store: obj, Episodes: eps, HTTP: migrateHTTPClient(), CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}}
	res := m.MigrateEpisode(context.Background(), store.MigratableEpisode{
		ID: 53, VideoID: 12, PlaybackURL: "https://src.example/v/index.m3u8", PlaybackStatus: "ok",
	})
	if res.Error != "playlist_upload_failed" {
		t.Fatalf("error %q", res.Error)
	}
	if eps.migrated != 0 {
		t.Fatal("db migrated without playlist")
	}
	if _, ok := obj.Objects["videos/12/53/index.m3u8"]; ok {
		t.Fatal("playlist should not exist")
	}
}

func TestMigrateEpisode_transientPutThenSuccess(t *testing.T) {
	obj := NewMemoryObjectStore()
	obj.PutTransientFails = map[string]int{
		"videos/12/53/segment001.ts": 2,
	}
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	m := &Migrator{Store: obj, Episodes: eps, HTTP: migrateHTTPClient(), CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}}
	res := m.MigrateEpisode(context.Background(), store.MigratableEpisode{
		ID: 53, VideoID: 12, PlaybackURL: "https://src.example/v/index.m3u8", PlaybackStatus: "ok",
	})
	if res.Error != "" {
		t.Fatalf("error %s", res.Error)
	}
	if eps.migrated != 1 {
		t.Fatalf("expected migrated %#v", eps)
	}
	if obj.PutAttemptCount("videos/12/53/segment001.ts") < 3 {
		t.Fatalf("expected retries, got %d", obj.PutAttemptCount("videos/12/53/segment001.ts"))
	}
}

func TestMigrateEpisode_retryExhaustionLeavesDBUnmigrated(t *testing.T) {
	obj := NewMemoryObjectStore()
	obj.PutTransientFails = map[string]int{
		"videos/12/53/segment001.ts": 10, // > max attempts
	}
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	m := &Migrator{Store: obj, Episodes: eps, HTTP: migrateHTTPClient(), CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}}
	res := m.MigrateEpisode(context.Background(), store.MigratableEpisode{
		ID: 53, VideoID: 12, PlaybackURL: "https://src.example/v/index.m3u8", PlaybackStatus: "ok",
	})
	if res.Error != "segment_upload_failed" {
		t.Fatalf("error %q", res.Error)
	}
	if eps.migrated != 0 {
		t.Fatal("db migrated after retry exhaustion")
	}
	if obj.PutAttemptCount("videos/12/53/segment001.ts") != 5 {
		t.Fatalf("attempts=%d want 5", obj.PutAttemptCount("videos/12/53/segment001.ts"))
	}
}

func TestMigrateEpisode_successfulResumeAllSegmentsPresent(t *testing.T) {
	obj := NewMemoryObjectStore()
	obj.Objects["videos/12/53/segment000.ts"] = []byte("TSDATA")
	obj.Objects["videos/12/53/segment001.ts"] = []byte("TSDATA")
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	m := &Migrator{Store: obj, Episodes: eps, HTTP: migrateHTTPClient(), CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}}
	res := m.MigrateEpisode(context.Background(), store.MigratableEpisode{
		ID: 53, VideoID: 12, PlaybackURL: "https://src.example/v/index.m3u8", PlaybackStatus: "ok",
	})
	if res.Error != "" {
		t.Fatalf("error %s", res.Error)
	}
	if obj.PutCalls != 1 {
		t.Fatalf("expected playlist put only, got %d puts", obj.PutCalls)
	}
	if eps.migrated != 1 {
		t.Fatalf("expected migrated %#v", eps)
	}
}
