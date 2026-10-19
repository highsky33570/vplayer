package commenthtml

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestListURLIsAnonymousCommentRead(t *testing.T) {
	got, err := ListURL("https://www.olehdtv.com", "82861", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "/index.php/comment/ajax.html?") {
		t.Fatalf("url=%s", got)
	}
	for _, banned := range []string{"saveData", "gbook", "verify", "digg", "report"} {
		if strings.Contains(strings.ToLower(got), banned) {
			t.Fatalf("url contains %s: %s", banned, got)
		}
	}
	for _, part := range []string{"rid=82861", "mid=1", "page=1"} {
		if !strings.Contains(got, part) {
			t.Fatalf("missing %s in %s", part, got)
		}
	}
	if _, err := ListURL("https://www.olehdtv.com", "82861", 0); err == nil {
		t.Fatal("page 0 accepted")
	}
	if _, err := ListURL("https://www.olehdtv.com", "not-an-id", 1); err == nil {
		t.Fatal("non numeric rid accepted")
	}
}

func TestFetchCommentPageDecodesJSONStringAndRejectsPOST(t *testing.T) {
	var method string
	client := NewClient("https://www.olehdtv.com", 2*time.Second, 0, 1)
	client.HTTP.Client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		method = r.Method
		if r.URL.Path != "/index.php/comment/ajax.html" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.URL.Query().Get("rid") != "82861" || r.URL.Query().Get("mid") != "1" || r.URL.Query().Get("page") != "2" {
			t.Errorf("query %s", r.URL.RawQuery)
		}
		if r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
			t.Errorf("xhr %q", r.Header.Get("X-Requested-With"))
		}
		if !strings.Contains(r.Header.Get("Accept"), "application/json") {
			t.Errorf("accept %q", r.Header.Get("Accept"))
		}
		if r.Header.Get("User-Agent") != "VPlayerCatalogSync/1.0" {
			t.Errorf("ua %q", r.Header.Get("User-Agent"))
		}
		if r.Body != nil {
			body, _ := io.ReadAll(r.Body)
			if len(body) != 0 {
				t.Errorf("body %q", body)
			}
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`"<div class=\"comm_content\">ok</div>"`)),
			Request:    r,
		}, nil
	})
	got, err := client.FetchCommentPage(context.Background(), "82861", 2)
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodGet {
		t.Fatalf("method %s", method)
	}
	if got != `<div class="comm_content">ok</div>` {
		t.Fatalf("decoded %q", got)
	}
}

func TestFetchHeadersKeepsPlainGET(t *testing.T) {
	f := maccms.NewHTTPFetcher(time.Second, 0, 1, "VPlayerCatalogSync/1.0")
	f.Client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet {
			t.Errorf("method %s", r.Method)
		}
		if r.Header.Get("X-Requested-With") != "" {
			t.Errorf("catalog fetch gained xhr header")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})
	got, err := f.Fetch(context.Background(), "https://www.olehdtv.com/")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ok" {
		t.Fatalf("body %q", got)
	}
}
