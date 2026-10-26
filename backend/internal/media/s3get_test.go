package media

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenPrivateObjectForwardsRange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method %s", r.Method)
		}
		if r.URL.Path != "/dongman/videos/9/8/seg.ts" {
			t.Errorf("path %s", r.URL.Path)
		}
		if got := r.Header.Get("Range"); got != "bytes=0-1" {
			t.Errorf("range %q", got)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("missing authorization")
		}
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Set("Content-Range", "bytes 0-1/4")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("ab"))
	}))
	defer func() {
		srv.CloseClientConnections()
		done := make(chan struct{})
		go func() {
			srv.Close()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}()

	body, err := OpenPrivateObject(context.Background(), srv.URL, "dongman", "test-access-key", "test-secret-value", "auto", "", "videos/9/8/seg.ts", "bytes=0-1")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Body.Close()
	got, _ := io.ReadAll(body.Body)
	if body.StatusCode != http.StatusPartialContent || string(got) != "ab" || body.ContentRange != "bytes 0-1/4" {
		t.Fatalf("status %d body %q range %q", body.StatusCode, got, body.ContentRange)
	}
}

func TestLocalObjectRange(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalStore(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), "videos/9/8/seg.ts", []byte("01234567"), "video/mp2t"); err != nil {
		t.Fatal(err)
	}
	body, err := OpenPrivateObject(context.Background(), "", "", "", "", "", dir, "videos/9/8/seg.ts", "bytes=2-4")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Body.Close()
	got, _ := io.ReadAll(body.Body)
	if body.StatusCode != http.StatusPartialContent || string(got) != "234" || body.ContentRange != "bytes 2-4/8" {
		t.Fatalf("status %d body %q range %q", body.StatusCode, got, body.ContentRange)
	}
	if _, err := OpenPrivateObject(context.Background(), "", "", "", "", "", dir, "videos/../../secret", ""); err == nil {
		t.Fatal("traversal opened a file")
	}
}

func TestOpenPrivateObjectDoesNotFollowPlaylistURL(t *testing.T) {
	_, err := OpenPrivateObject(context.Background(), "", "", "", "", "", "", "https://169.254.169.254/latest/meta-data", "")
	if err == nil {
		t.Fatal("fetched an absolute url without storage")
	}
}
