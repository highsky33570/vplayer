package maccms

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const testAuthCookie = "session=test-olehdtv-auth-cookie-value"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func htmlResponse(code int, body string) *http.Response {
	return &http.Response{
		StatusCode: code,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestIsOleHdTvHost(t *testing.T) {
	for _, host := range []string{"olehdtv.com", "www.olehdtv.com", "WWW.OleHdTv.Com", "cdn.olehdtv.com"} {
		if !IsOleHdTvHost(host) {
			t.Fatalf("expected trusted: %q", host)
		}
	}
	for _, host := range []string{"evil.example", "olehdtv.com.evil", "yuntongclouds.com", ""} {
		if IsOleHdTvHost(host) {
			t.Fatalf("expected untrusted: %q", host)
		}
	}
}

func TestHTTPFetcher_noCookieByDefault(t *testing.T) {
	var sawCookie bool
	f := NewHTTPFetcher(2*time.Second, 0, 1, "test")
	f.Client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Cookie") != "" {
			sawCookie = true
		}
		return htmlResponse(200, "<html>ok</html>"), nil
	})
	body, err := f.Fetch(context.Background(), "https://www.olehdtv.com/index.php/vod/play/id/1/sid/1/nid/1.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "ok") {
		t.Fatalf("body=%q", body)
	}
	if sawCookie {
		t.Fatal("anonymous fetch must not send Cookie")
	}
	if f.AuthCookieConfigured() {
		t.Fatal("expected no auth configured")
	}
}

func TestHTTPFetcher_setsCookieOnTrustedHost(t *testing.T) {
	var gotCookie string
	f := NewHTTPFetcher(2*time.Second, 0, 1, "test")
	f.SetAuthCookie(testAuthCookie)
	f.Client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotCookie = r.Header.Get("Cookie")
		return htmlResponse(200, `var player_aaaa={"url":"https://cdn.example/a.m3u8","encrypt":0}</script>`), nil
	})
	if !f.AuthCookieConfigured() {
		t.Fatal("expected auth configured")
	}
	body, err := f.Fetch(context.Background(), "https://www.olehdtv.com/play")
	if err != nil {
		t.Fatal(err)
	}
	if gotCookie != testAuthCookie {
		t.Fatalf("cookie mismatch: got %q", redactAuth(gotCookie))
	}
	if _, ok := ParsePlayerAAAA(body); !ok {
		t.Fatal("expected player_aaaa parse")
	}
}

func TestHTTPFetcher_ignoresCallerCookieHeader(t *testing.T) {
	var gotCookie string
	f := NewHTTPFetcher(2*time.Second, 0, 1, "test")
	f.SetAuthCookie(testAuthCookie)
	f.Client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotCookie = r.Header.Get("Cookie")
		return htmlResponse(200, "ok"), nil
	})
	_, err := f.FetchHeaders(context.Background(), "https://www.olehdtv.com/", map[string]string{
		"Cookie": "attacker=1",
		"Accept": "text/html",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotCookie != testAuthCookie {
		t.Fatalf("auth cookie must win / caller Cookie ignored; got %q", redactAuth(gotCookie))
	}
}

func TestHTTPFetcher_noCookieOnUntrustedHost(t *testing.T) {
	var sawCookie bool
	f := NewHTTPFetcher(2*time.Second, 0, 1, "test")
	f.SetAuthCookie(testAuthCookie)
	f.Client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Cookie") != "" {
			sawCookie = true
		}
		return htmlResponse(200, "ok"), nil
	})
	_, err := f.Fetch(context.Background(), "https://cdn.example/master.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	if sawCookie {
		t.Fatal("must not attach Cookie to untrusted host")
	}
}

func TestHTTPFetcher_checkRedirectStripsCookieOffTrustedHost(t *testing.T) {
	f := NewHTTPFetcher(2*time.Second, 0, 1, "test")
	f.SetAuthCookie(testAuthCookie)

	viaReq, err := http.NewRequest(http.MethodGet, "https://www.olehdtv.com/index.php/vod/play/id/1/sid/1/nid/1.html", nil)
	if err != nil {
		t.Fatal(err)
	}
	viaReq.Header.Set("Cookie", testAuthCookie)

	offHost, err := http.NewRequest(http.MethodGet, "https://cloud.example/master.m3u8", nil)
	if err != nil {
		t.Fatal(err)
	}
	offHost.Header.Set("Cookie", testAuthCookie)
	if err := f.checkRedirect(offHost, []*http.Request{viaReq}); err != nil {
		t.Fatal(err)
	}
	if offHost.Header.Get("Cookie") != "" {
		t.Fatalf("cookie leaked to untrusted redirect host: %q", redactAuth(offHost.Header.Get("Cookie")))
	}

	stay, err := http.NewRequest(http.MethodGet, "https://www.olehdtv.com/index.php/vod/play/id/1/sid/1/nid/2.html", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.checkRedirect(stay, []*http.Request{viaReq}); err != nil {
		t.Fatal(err)
	}
	if stay.Header.Get("Cookie") != testAuthCookie {
		t.Fatalf("trusted redirect must keep auth cookie; got %q", redactAuth(stay.Header.Get("Cookie")))
	}
}

func TestHTTPFetcher_errorsDoNotContainCookie(t *testing.T) {
	f := NewHTTPFetcher(2*time.Second, 0, 1, "test")
	f.SetAuthCookie(testAuthCookie)
	f.Client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return htmlResponse(403, "nope"), nil
	})
	_, err := f.Fetch(context.Background(), "https://www.olehdtv.com/denied")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if strings.Contains(msg, testAuthCookie) || strings.Contains(msg, "test-olehdtv-auth") {
		t.Fatalf("error leaked cookie material: %v", err)
	}
}

func TestHTTPFetcher_trustPublicBaseURL(t *testing.T) {
	var gotCookie string
	f := NewHTTPFetcher(2*time.Second, 0, 1, "test")
	f.SetAuthCookie(testAuthCookie)
	f.TrustPublicBaseURL("https://maccms.local")
	f.Client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotCookie = r.Header.Get("Cookie")
		return htmlResponse(200, "ok"), nil
	})
	_, err := f.Fetch(context.Background(), "https://maccms.local/play")
	if err != nil {
		t.Fatal(err)
	}
	if gotCookie != testAuthCookie {
		t.Fatalf("public base host should be trusted; got %q", redactAuth(gotCookie))
	}
}

func redactAuth(s string) string {
	if s == "" {
		return ""
	}
	return fmt.Sprintf("<redacted len=%d>", len(s))
}
