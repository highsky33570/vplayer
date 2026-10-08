package mediamigrate

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tycdn/vplayer/internal/media"
	"github.com/tycdn/vplayer/internal/store"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type fakeEpisodes struct {
	hlsKey   string
	status   string
	playback string
	migrated int
	failed   int
}

func (f *fakeEpisodes) MarkEpisodeMigrated(id uint64, objectKey, provider string) error {
	_ = id
	_ = provider
	f.migrated++
	f.hlsKey = objectKey
	f.status = "done"
	return nil
}

func (f *fakeEpisodes) MarkEpisodeMigrationFailed(id uint64, msg string) error {
	_ = id
	_ = msg
	f.failed++
	f.status = "failed"
	return nil
}

func (f *fakeEpisodes) GetEpisodeMigrationState(id uint64) (string, string, string, error) {
	_ = id
	return f.hlsKey, f.status, f.playback, nil
}

func mediaPlaylistBody() string {
	return `#EXTM3U
#EXT-X-TARGETDURATION:2
#EXTINF:2.0,
segA.ts
#EXTINF:2.0,
https://abs.example/x/segB.ts
#EXT-X-ENDLIST
`
}

func TestMigrateEpisode_successRewritesAndUpdatesDB(t *testing.T) {
	obj := NewMemoryObjectStore()
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
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

	m := &Migrator{
		Store:    obj,
		Episodes: eps,
		HTTP:     client,
		CDN:      media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"},
	}
	ep := store.MigratableEpisode{
		ID: 53, VideoID: 12, PlaybackURL: "https://src.example/v/index.m3u8", PlaybackStatus: "ok",
	}
	res := m.MigrateEpisode(context.Background(), ep)
	if res.Error != "" {
		t.Fatalf("error %s", res.Error)
	}
	if eps.migrated != 1 || eps.hlsKey != "videos/12/53/index.m3u8" || eps.status != "done" {
		t.Fatalf("db state %#v", eps)
	}
	if eps.playback != "https://src.example/v/index.m3u8" {
		t.Fatalf("playback_url changed")
	}
	pl, ok := obj.Objects["videos/12/53/index.m3u8"]
	if !ok {
		t.Fatal("playlist missing")
	}
	if strings.Contains(string(pl), "https://") || strings.Contains(string(pl), "src.example") {
		t.Fatalf("playlist still references provider:\n%s", pl)
	}
	if !strings.Contains(string(pl), "segment000.ts") {
		t.Fatalf("playlist:\n%s", pl)
	}
	if _, ok := obj.Objects["videos/12/53/segment000.ts"]; !ok {
		t.Fatal("segment000 missing")
	}
	if _, ok := obj.Objects["videos/12/53/segment001.ts"]; !ok {
		t.Fatal("segment001 missing")
	}
	wantCDN := "https://media.example.com/dongman/videos/12/53/index.m3u8"
	if res.Plan.ProposedCDNURL != wantCDN {
		t.Fatalf("cdn %q", res.Plan.ProposedCDNURL)
	}
}

func TestMigrateEpisode_segmentDownloadFailureLeavesDBUnmigrated(t *testing.T) {
	obj := NewMemoryObjectStore()
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "index.m3u8") {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(mediaPlaylistBody())), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
	})}
	m := &Migrator{
		Store: obj, Episodes: eps, HTTP: client,
		CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"},
		SourceRetryBase: time.Millisecond,
	}
	res := m.MigrateEpisode(context.Background(), store.MigratableEpisode{
		ID: 53, VideoID: 12, PlaybackURL: "https://src.example/v/index.m3u8", PlaybackStatus: "ok",
	})
	if res.Error != "segment_download_failed" {
		t.Fatalf("error %q", res.Error)
	}
	if eps.migrated != 0 || eps.hlsKey != "" {
		t.Fatalf("should not mark migrated: %#v", eps)
	}
	if eps.playback == "" {
		t.Fatal("playback cleared")
	}
	if eps.failed != 1 || eps.status != "failed" {
		t.Fatalf("expected failed status %#v", eps)
	}
}

func TestMigrateEpisode_uploadFailureLeavesDBUnmigrated(t *testing.T) {
	obj := NewMemoryObjectStore()
	obj.FailPut = "segment001"
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := "TS"
		if strings.HasSuffix(r.URL.Path, "index.m3u8") {
			body = mediaPlaylistBody()
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	m := &Migrator{Store: obj, Episodes: eps, HTTP: client, CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}}
	res := m.MigrateEpisode(context.Background(), store.MigratableEpisode{
		ID: 53, VideoID: 12, PlaybackURL: "https://src.example/v/index.m3u8", PlaybackStatus: "ok",
	})
	if res.Error != "segment_upload_failed" {
		t.Fatalf("error %q", res.Error)
	}
	if eps.migrated != 0 {
		t.Fatal("db migrated on upload failure")
	}
}

func TestMigrateEpisode_idempotentAlreadyMigrated(t *testing.T) {
	obj := NewMemoryObjectStore()
	eps := &fakeEpisodes{
		hlsKey: "videos/12/53/index.m3u8", status: "done",
		playback: "https://src.example/v/index.m3u8",
	}
	m := &Migrator{Store: obj, Episodes: eps, CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}}
	res := m.MigrateEpisode(context.Background(), store.MigratableEpisode{
		ID: 53, VideoID: 12,
		PlaybackURL: "https://src.example/v/index.m3u8", PlaybackStatus: "ok",
		HLSObjectKey: "videos/12/53/index.m3u8", MigrationStatus: "done",
	})
	if !res.AlreadyMigrated || eps.migrated != 0 {
		t.Fatalf("idempotent fail: %#v migrated=%d", res, eps.migrated)
	}
}

func TestMigrateEpisode_rejectsMultiVariantMaster(t *testing.T) {
	obj := NewMemoryObjectStore()
	eps := &fakeEpisodes{playback: "https://src.example/v/master.m3u8"}
	master := `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=800000
720p.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=400000
360p.m3u8
`
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(master)), Header: make(http.Header)}, nil
	})}
	m := &Migrator{Store: obj, Episodes: eps, HTTP: client, CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}}
	res := m.MigrateEpisode(context.Background(), store.MigratableEpisode{
		ID: 1, VideoID: 1, PlaybackURL: "https://src.example/v/master.m3u8", PlaybackStatus: "ok",
	})
	if !strings.Contains(res.Error, "master_playlist_unsupported") || eps.migrated != 0 {
		t.Fatalf("got error=%q migrated=%d", res.Error, eps.migrated)
	}
}

func TestMigrateEpisode_followsSingleVariantMaster(t *testing.T) {
	obj := NewMemoryObjectStore()
	eps := &fakeEpisodes{playback: "https://src.example/v/master.m3u8"}
	master := `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=800000
index-v1-a1.m3u8
`
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		path := r.URL.Path
		var body string
		switch {
		case strings.HasSuffix(path, "master.m3u8"):
			body = master
		case strings.HasSuffix(path, "index-v1-a1.m3u8"):
			body = mediaPlaylistBody()
		default:
			body = "TSDATA"
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	m := &Migrator{Store: obj, Episodes: eps, HTTP: client, CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}}
	res := m.MigrateEpisode(context.Background(), store.MigratableEpisode{
		ID: 53, VideoID: 12, PlaybackURL: "https://src.example/v/master.m3u8", PlaybackStatus: "ok",
	})
	if res.Error != "" {
		t.Fatalf("error %s", res.Error)
	}
	if eps.migrated != 1 {
		t.Fatalf("expected migrate, got %#v", eps)
	}
	if _, ok := obj.Objects["videos/12/53/index.m3u8"]; !ok {
		t.Fatal("playlist missing")
	}
}

func TestBuildPlan_dongmanCDNPath(t *testing.T) {
	cdn := media.CDNURLOptions{BaseURL: "https://PLACEHOLDER_MEDIA_CDN_HOST", Bucket: "dongman"}
	plan := BuildPlan(store.MigratableEpisode{
		ID: 53, VideoID: 12, PlaybackURL: "https://cloud.example/a.m3u8", PlaybackStatus: "ok",
	}, cdn)
	want := "https://PLACEHOLDER_MEDIA_CDN_HOST/dongman/videos/12/53/index.m3u8"
	if plan.ProposedCDNURL != want {
		t.Fatalf("got %q", plan.ProposedCDNURL)
	}
	if plan.R2Bucket != "dongman" {
		t.Fatalf("bucket %q", plan.R2Bucket)
	}
}
