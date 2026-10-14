package mediamigrate

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tycdn/vplayer/internal/media"
	"github.com/tycdn/vplayer/internal/store"
)

func multiSegPlaylist(n int) string {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-TARGETDURATION:2\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "#EXTINF:2.0,\nseg%d.ts\n", i)
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return b.String()
}

type concurrencyTracker struct {
	inner ObjectStore

	mu       sync.Mutex
	inFlight int
	maxSeen  int
	putOrder []string
	gate     chan struct{} // optional: hold Puts until closed
}

func (c *concurrencyTracker) Exists(ctx context.Context, key string) (bool, error) {
	return c.inner.Exists(ctx, key)
}

func (c *concurrencyTracker) Head(ctx context.Context, key string) (int64, bool, error) {
	if hs, ok := c.inner.(HeadObjectStore); ok {
		return hs.Head(ctx, key)
	}
	ok, err := c.inner.Exists(ctx, key)
	return 0, ok, err
}

func (c *concurrencyTracker) Put(ctx context.Context, key string, body []byte, contentType string) error {
	c.mu.Lock()
	c.inFlight++
	if c.inFlight > c.maxSeen {
		c.maxSeen = c.inFlight
	}
	c.putOrder = append(c.putOrder, key)
	gate := c.gate
	c.mu.Unlock()

	if gate != nil {
		select {
		case <-ctx.Done():
			c.mu.Lock()
			c.inFlight--
			c.mu.Unlock()
			return ctx.Err()
		case <-gate:
		}
	} else {
		time.Sleep(20 * time.Millisecond)
	}

	err := c.inner.Put(ctx, key, body, contentType)

	c.mu.Lock()
	c.inFlight--
	c.mu.Unlock()
	return err
}

func (c *concurrencyTracker) maxInFlight() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.maxSeen
}

func (c *concurrencyTracker) order() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.putOrder))
	copy(out, c.putOrder)
	return out
}

type countingTransport struct {
	playlist string
	body     string

	mu       sync.Mutex
	inFlight int
	maxSeen  int
	failPath string
}

func (t *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	u := r.URL.String()
	if t.failPath != "" && strings.Contains(u, t.failPath) {
		return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
	}

	t.mu.Lock()
	t.inFlight++
	if t.inFlight > t.maxSeen {
		t.maxSeen = t.inFlight
	}
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		t.inFlight--
		t.mu.Unlock()
	}()

	time.Sleep(25 * time.Millisecond)

	body := t.body
	if strings.HasSuffix(u, "index.m3u8") || strings.Contains(u, ".m3u8") {
		body = t.playlist
	}
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

func (t *countingTransport) maxInFlight() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.maxSeen
}

func TestTransferSegments_successPlaylistLastAndMigratedOnce(t *testing.T) {
	const n = 6
	obj := NewMemoryObjectStore()
	track := &concurrencyTracker{inner: obj}
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	tr := &countingTransport{playlist: multiSegPlaylist(n), body: "TSDATA"}
	m := &Migrator{
		Store: track, Episodes: eps,
		HTTP: &http.Client{Transport: tr},
		CDN:  media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"},
	}
	res := m.MigrateEpisode(context.Background(), store.MigratableEpisode{
		ID: 53, VideoID: 12, PlaybackURL: "https://src.example/v/index.m3u8", PlaybackStatus: "ok",
	})
	if res.Error != "" {
		t.Fatalf("error %s", res.Error)
	}
	if eps.migrated != 1 {
		t.Fatalf("MarkEpisodeMigrated count=%d", eps.migrated)
	}
	if eps.hlsKey != "videos/12/53/index.m3u8" {
		t.Fatalf("key %q", eps.hlsKey)
	}
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("videos/12/53/segment%03d.ts", i)
		if _, ok := obj.Objects[key]; !ok {
			t.Fatalf("missing %s", key)
		}
	}
	order := track.order()
	if len(order) == 0 || !strings.HasSuffix(order[len(order)-1], "index.m3u8") {
		t.Fatalf("playlist must be last upload, order=%v", order)
	}
	for _, k := range order[:len(order)-1] {
		if strings.HasSuffix(k, "index.m3u8") {
			t.Fatalf("playlist uploaded before all segments: %v", order)
		}
	}
}

func TestTransferSegments_deterministicKeys(t *testing.T) {
	obj := NewMemoryObjectStore()
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	tr := &countingTransport{playlist: multiSegPlaylist(3), body: "TS"}
	m := &Migrator{
		Store: obj, Episodes: eps,
		HTTP: &http.Client{Transport: tr},
		CDN:  media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"},
	}
	res := m.MigrateEpisode(context.Background(), store.MigratableEpisode{
		ID: 7, VideoID: 3, PlaybackURL: "https://src.example/v/index.m3u8", PlaybackStatus: "ok",
	})
	if res.Error != "" {
		t.Fatal(res.Error)
	}
	want := []string{
		"videos/3/7/segment000.ts",
		"videos/3/7/segment001.ts",
		"videos/3/7/segment002.ts",
		"videos/3/7/index.m3u8",
	}
	for _, k := range want {
		if _, ok := obj.Objects[k]; !ok {
			t.Fatalf("missing %s keys=%v", k, keysOf(obj))
		}
	}
	pl := string(obj.Objects["videos/3/7/index.m3u8"])
	if !strings.Contains(pl, "segment000.ts") || !strings.Contains(pl, "segment002.ts") {
		t.Fatalf("playlist:\n%s", pl)
	}
}

func keysOf(obj *MemoryObjectStore) []string {
	out := make([]string, 0, len(obj.Objects))
	for k := range obj.Objects {
		out = append(out, k)
	}
	return out
}

func TestTransferSegments_concurrencyBoundedAndParallel(t *testing.T) {
	const n = 8
	obj := NewMemoryObjectStore()
	track := &concurrencyTracker{inner: obj}
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	tr := &countingTransport{playlist: multiSegPlaylist(n), body: "TSDATA"}
	m := &Migrator{
		Store: track, Episodes: eps,
		HTTP: &http.Client{Transport: tr},
		CDN:  media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"},
	}
	res := m.MigrateEpisode(context.Background(), store.MigratableEpisode{
		ID: 1, VideoID: 1, PlaybackURL: "https://src.example/v/index.m3u8", PlaybackStatus: "ok",
	})
	if res.Error != "" {
		t.Fatal(res.Error)
	}
	srcMax := tr.maxInFlight()
	putMax := track.maxInFlight()
	if srcMax > maxSegmentWorkers {
		t.Fatalf("source concurrency %d > %d", srcMax, maxSegmentWorkers)
	}
	if putMax > maxSegmentWorkers {
		t.Fatalf("put concurrency %d > %d", putMax, maxSegmentWorkers)
	}
	if srcMax < 2 {
		t.Fatalf("expected parallel source GETs, max=%d", srcMax)
	}
	if putMax < 2 {
		t.Fatalf("expected parallel R2 puts, max=%d", putMax)
	}
}

func TestTransferSegments_sourceFailureNoSuccess(t *testing.T) {
	obj := NewMemoryObjectStore()
	track := &concurrencyTracker{inner: obj}
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	tr := &countingTransport{
		playlist: multiSegPlaylist(6), body: "TSDATA",
		failPath: "seg3.ts",
	}
	m := &Migrator{
		Store: track, Episodes: eps,
		HTTP:              &http.Client{Transport: tr},
		CDN:               media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"},
		SourceRetryBase:   time.Millisecond,
		SourceMaxAttempts: 2,
	}
	res := m.MigrateEpisode(context.Background(), store.MigratableEpisode{
		ID: 53, VideoID: 12, PlaybackURL: "https://src.example/v/index.m3u8", PlaybackStatus: "ok",
	})
	if res.Error != "segment_download_failed" {
		t.Fatalf("error %q", res.Error)
	}
	if eps.migrated != 0 {
		t.Fatal("must not MarkEpisodeMigrated")
	}
	if eps.failed != 1 {
		t.Fatalf("failed writes=%d", eps.failed)
	}
	if _, ok := obj.Objects["videos/12/53/index.m3u8"]; ok {
		t.Fatal("playlist must not upload after segment failure")
	}
}

func TestTransferSegments_putFailureNoSuccess(t *testing.T) {
	obj := NewMemoryObjectStore()
	obj.FailPut = "segment003"
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	tr := &countingTransport{playlist: multiSegPlaylist(6), body: "TSDATA"}
	m := &Migrator{
		Store: obj, Episodes: eps,
		HTTP: &http.Client{Transport: tr},
		CDN:  media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"},
	}
	res := m.MigrateEpisode(context.Background(), store.MigratableEpisode{
		ID: 53, VideoID: 12, PlaybackURL: "https://src.example/v/index.m3u8", PlaybackStatus: "ok",
	})
	if res.Error != "segment_upload_failed" {
		t.Fatalf("error %q", res.Error)
	}
	if eps.migrated != 0 || eps.failed != 1 {
		t.Fatalf("db %#v", eps)
	}
	if _, ok := obj.Objects["videos/12/53/index.m3u8"]; ok {
		t.Fatal("playlist must not upload after put failure")
	}
}

func TestTransferSegments_contextCancelNoSuccess(t *testing.T) {
	obj := NewMemoryObjectStore()
	gate := make(chan struct{})
	track := &concurrencyTracker{inner: obj, gate: gate}
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	tr := &countingTransport{playlist: multiSegPlaylist(8), body: "TSDATA"}
	m := &Migrator{
		Store: track, Episodes: eps,
		HTTP: &http.Client{Transport: tr},
		CDN:  media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"},
	}
	ctx, cancel := context.WithCancel(context.Background())
	var res MigrateResult
	var done sync.WaitGroup
	done.Add(1)
	go func() {
		defer done.Done()
		res = m.MigrateEpisode(ctx, store.MigratableEpisode{
			ID: 53, VideoID: 12, PlaybackURL: "https://src.example/v/index.m3u8", PlaybackStatus: "ok",
		})
	}()
	time.Sleep(40 * time.Millisecond)
	cancel()
	close(gate) // unblock any Puts waiting on gate
	done.Wait()
	if res.Error == "" || eps.migrated != 0 {
		t.Fatalf("expected cancel failure, err=%q migrated=%d", res.Error, eps.migrated)
	}
}

func TestTransferSegments_headSizeMatchResume(t *testing.T) {
	obj := NewMemoryObjectStore()
	// Pre-seed matching segment bodies so ensureObject skips PUT via HEAD size match.
	body := []byte("TSDATA")
	for i := 0; i < 4; i++ {
		obj.Objects[fmt.Sprintf("videos/12/53/segment%03d.ts", i)] = append([]byte(nil), body...)
	}
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	var putsBefore atomic.Int64
	putsBefore.Store(int64(obj.PutCalls))
	tr := &countingTransport{playlist: multiSegPlaylist(4), body: string(body)}
	m := &Migrator{
		Store: obj, Episodes: eps,
		HTTP: &http.Client{Transport: tr},
		CDN:  media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"},
	}
	res := m.MigrateEpisode(context.Background(), store.MigratableEpisode{
		ID: 53, VideoID: 12, PlaybackURL: "https://src.example/v/index.m3u8", PlaybackStatus: "ok",
	})
	if res.Error != "" {
		t.Fatal(res.Error)
	}
	if eps.migrated != 1 {
		t.Fatal("expected success")
	}
	// Playlist put (+ maybe segment puts if skip failed). Segment puts should be skipped.
	segPuts := 0
	for k, n := range obj.putAttempts {
		if strings.Contains(k, "segment") {
			segPuts += n
		}
	}
	if segPuts != 0 {
		t.Fatalf("expected HEAD skip for segments, putAttempts=%v", obj.putAttempts)
	}
	if _, ok := obj.Objects["videos/12/53/index.m3u8"]; !ok {
		t.Fatal("playlist missing")
	}
}

func TestSegmentWorkerCountCap(t *testing.T) {
	m := &Migrator{}
	if m.segmentWorkerCount(100) != maxSegmentWorkers {
		t.Fatalf("got %d", m.segmentWorkerCount(100))
	}
	if m.segmentWorkerCount(2) != 2 {
		t.Fatalf("got %d", m.segmentWorkerCount(2))
	}
	m.SegmentWorkers = 1
	if m.segmentWorkerCount(10) != 1 {
		t.Fatalf("override got %d", m.segmentWorkerCount(10))
	}
}
