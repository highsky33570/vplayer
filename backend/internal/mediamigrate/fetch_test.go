package mediamigrate

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tycdn/vplayer/internal/media"
	"github.com/tycdn/vplayer/internal/store"
)

func TestIsRetryableSourceError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{&httpStatusError{Code: 404}, false},
		{&httpStatusError{Code: 401}, false},
		{&httpStatusError{Code: 403}, false},
		{&httpStatusError{Code: 408}, true},
		{&httpStatusError{Code: 429}, true},
		{&httpStatusError{Code: 500}, true},
		{&httpStatusError{Code: 503}, true},
		{errors.New("connection reset by peer"), true},
		{errors.New("i/o timeout"), true},
		{errors.New("temporary failure in name resolution"), true},
		{errors.New("something else"), false},
	}
	for _, tc := range cases {
		if got := IsRetryableSourceError(tc.err); got != tc.want {
			t.Fatalf("err=%v got=%v want=%v", tc.err, got, tc.want)
		}
	}
}

func TestFetchBytesLabeled_transientThenSuccess(t *testing.T) {
	var attempts int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("OKDATA")), Header: make(http.Header)}, nil
	})}
	m := &Migrator{HTTP: client, SourceRetryBase: time.Millisecond}
	b, err := m.fetchBytesLabeled(context.Background(), "https://src.example/seg.ts", "segment", 9, "segment009.ts")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if string(b) != "OKDATA" || attempts != 3 {
		t.Fatalf("body=%q attempts=%d", b, attempts)
	}
}

func TestFetchBytesLabeled_retryExhaustion(t *testing.T) {
	var attempts int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return &http.Response{StatusCode: 503, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
	})}
	m := &Migrator{HTTP: client, SourceRetryBase: time.Millisecond, SourceMaxAttempts: 5}
	_, err := m.fetchBytesLabeled(context.Background(), "https://src.example/seg.ts", "segment", 0, "segment000.ts")
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 5 {
		t.Fatalf("attempts=%d", attempts)
	}
	var st *httpStatusError
	if !errors.As(err, &st) || st.Code != 503 {
		t.Fatalf("err=%v", err)
	}
}

func TestFetchBytesLabeled_404NotRetried(t *testing.T) {
	var attempts int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return &http.Response{StatusCode: 404, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
	})}
	m := &Migrator{HTTP: client, SourceRetryBase: time.Millisecond}
	_, err := m.fetchBytesLabeled(context.Background(), "https://src.example/missing.ts", "segment", 1, "segment001.ts")
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Fatalf("attempts=%d want 1", attempts)
	}
}

func TestFetchBytesLabeled_429Retried(t *testing.T) {
	var attempts int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			return &http.Response{StatusCode: 429, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("x")), Header: make(http.Header)}, nil
	})}
	m := &Migrator{HTTP: client, SourceRetryBase: time.Millisecond}
	if _, err := m.fetchBytesLabeled(context.Background(), "https://src.example/a.ts", "segment", 0, "a.ts"); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts=%d", attempts)
	}
}

func TestFetchBytesLabeled_5xxRetried(t *testing.T) {
	var attempts int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			return &http.Response{StatusCode: 502, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("x")), Header: make(http.Header)}, nil
	})}
	m := &Migrator{HTTP: client, SourceRetryBase: time.Millisecond}
	if _, err := m.fetchBytesLabeled(context.Background(), "https://src.example/a.ts", "segment", 0, "a.ts"); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts=%d", attempts)
	}
}

func TestFetchBytesLabeled_networkErrorRetried(t *testing.T) {
	var attempts int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			return nil, errors.New("connection reset by peer")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("x")), Header: make(http.Header)}, nil
	})}
	m := &Migrator{HTTP: client, SourceRetryBase: time.Millisecond}
	if _, err := m.fetchBytesLabeled(context.Background(), "https://src.example/a.ts", "segment", 0, "a.ts"); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts=%d", attempts)
	}
}

func TestMigrateEpisode_successAfterTransientSegmentFailure(t *testing.T) {
	var segBAttempts int32
	obj := NewMemoryObjectStore()
	eps := &fakeEpisodes{playback: "https://src.example/v/index.m3u8"}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		u := r.URL.String()
		switch {
		case strings.HasSuffix(u, "index.m3u8"):
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(mediaPlaylistBody())), Header: make(http.Header)}, nil
		case strings.HasSuffix(u, "segA.ts"):
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("TSDATA")), Header: make(http.Header)}, nil
		case strings.HasSuffix(u, "segB.ts"):
			n := atomic.AddInt32(&segBAttempts, 1)
			if n == 1 {
				return &http.Response{StatusCode: 503, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("TSDATA")), Header: make(http.Header)}, nil
		default:
			return &http.Response{StatusCode: 404, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
		}
	})}
	m := &Migrator{
		Store: obj, Episodes: eps, HTTP: client,
		CDN: media.CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"},
		SourceRetryBase: time.Millisecond,
	}
	res := m.MigrateEpisode(context.Background(), store.MigratableEpisode{
		ID: 53, VideoID: 12, PlaybackURL: "https://src.example/v/index.m3u8", PlaybackStatus: "ok",
	})
	if res.Error != "" {
		t.Fatalf("error %s", res.Error)
	}
	if eps.migrated != 1 {
		t.Fatalf("expected migrated %#v", eps)
	}
	if segBAttempts < 2 {
		t.Fatalf("segB attempts=%d", segBAttempts)
	}
	// Prior segment must remain present (not force full re-download of package).
	if _, ok := obj.Objects["videos/12/53/segment000.ts"]; !ok {
		t.Fatal("segment000 missing after resume-style success")
	}
}
