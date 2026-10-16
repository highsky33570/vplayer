package media

import (
	"strings"
	"testing"
)

func TestParseCDNPathMode(t *testing.T) {
	got, err := ParseCDNPathMode("")
	if err != nil || got != CDNPathModeBucketPrefix {
		t.Fatalf("unset: got %q err=%v", got, err)
	}
	got, err = ParseCDNPathMode("  ")
	if err != nil || got != CDNPathModeBucketPrefix {
		t.Fatalf("blank: got %q err=%v", got, err)
	}
	got, err = ParseCDNPathMode("bucket_prefix")
	if err != nil || got != CDNPathModeBucketPrefix {
		t.Fatalf("explicit bucket_prefix: got %q err=%v", got, err)
	}
	got, err = ParseCDNPathMode("OBJECT_KEY")
	if err != nil || got != CDNPathModeObjectKey {
		t.Fatalf("object_key: got %q err=%v", got, err)
	}
	_, err = ParseCDNPathMode("tycdn")
	if err == nil || !strings.Contains(err.Error(), "unsupported MEDIA_CDN_PATH_MODE") {
		t.Fatalf("invalid mode should error clearly, got %v", err)
	}
}

func TestBuildMediaURL_defaultPathModePreservesBucketPrefix(t *testing.T) {
	opts := CDNURLOptions{
		BaseURL: "https://media.example.com",
		Bucket:  "dongman",
		// PathMode unset → bucket_prefix
	}
	got := BuildMediaURL(opts, "posters/100.jpg")
	want := "https://media.example.com/dongman/posters/100.jpg"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildMediaURL_preservesBucketPrefix(t *testing.T) {
	opts := CDNURLOptions{
		BaseURL:  "https://media.example.com",
		Bucket:   "dongman",
		PathMode: CDNPathModeBucketPrefix,
	}
	got := BuildMediaURL(opts, "posters/100.jpg")
	want := "https://media.example.com/dongman/posters/100.jpg"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildMediaURL_objectKeyMode(t *testing.T) {
	opts := CDNURLOptions{
		BaseURL:  "https://media.example.com",
		Bucket:   "dongman",
		PathMode: CDNPathModeObjectKey,
	}
	got := BuildMediaURL(opts, "videos/12/53/index.m3u8")
	want := "https://media.example.com/videos/12/53/index.m3u8"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildMediaURL_trailingSlashBaseAndLeadingSlashKey(t *testing.T) {
	opts := CDNURLOptions{
		BaseURL:  "https://media.example.com/",
		Bucket:   "dongman",
		PathMode: CDNPathModeBucketPrefix,
	}
	got := BuildMediaURL(opts, "/videos/12/53/index.m3u8")
	want := "https://media.example.com/dongman/videos/12/53/index.m3u8"
	if got != want {
		t.Fatalf("bucket_prefix: got %q want %q", got, want)
	}

	opts.PathMode = CDNPathModeObjectKey
	got = BuildMediaURL(opts, "/videos/12/53/index.m3u8")
	want = "https://media.example.com/videos/12/53/index.m3u8"
	if got != want {
		t.Fatalf("object_key: got %q want %q", got, want)
	}
}

func TestBuildMediaURL_doesNotDoubleBucket(t *testing.T) {
	opts := CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}
	got := BuildMediaURL(opts, "dongman/hls-test/test.m3u8")
	want := "https://media.example.com/dongman/hls-test/test.m3u8"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildMediaURL_keepsAbsoluteExternal(t *testing.T) {
	opts := CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}
	src := "https://cdn.olehdtv.example/a.jpg"
	if got := BuildMediaURL(opts, src); got != src {
		t.Fatalf("got %q want %q", got, src)
	}
}

func TestResolveMediaURL_prefersObjectKey(t *testing.T) {
	opts := CDNURLOptions{BaseURL: "https://media.example.com", Bucket: "dongman"}
	got := ResolveMediaURL(opts, "videos/1/2/index.m3u8", "https://old.example/x.m3u8")
	want := "https://media.example.com/dongman/videos/1/2/index.m3u8"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveMediaURL_objectKeyMode(t *testing.T) {
	opts := CDNURLOptions{
		BaseURL:  "https://media.example.com",
		Bucket:   "dongman",
		PathMode: CDNPathModeObjectKey,
	}
	got := ResolveMediaURL(opts, "videos/1/2/index.m3u8", "https://old.example/x.m3u8")
	want := "https://media.example.com/videos/1/2/index.m3u8"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestEpisodeHLSObjectKey(t *testing.T) {
	if got := EpisodeHLSObjectKey(123, 456); got != "videos/123/456/index.m3u8" {
		t.Fatalf("got %q", got)
	}
}

func TestBuildMediaURL_episode53DongmanPath(t *testing.T) {
	opts := CDNURLOptions{BaseURL: "https://PLACEHOLDER_MEDIA_CDN_HOST", Bucket: "dongman"}
	got := BuildMediaURL(opts, EpisodeHLSObjectKey(12, 53))
	want := "https://PLACEHOLDER_MEDIA_CDN_HOST/dongman/videos/12/53/index.m3u8"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDeliveryObjectPath_matchesBuildMediaURLPath(t *testing.T) {
	opts := CDNURLOptions{
		BaseURL:  "https://media.example.com",
		Bucket:   "dongman",
		PathMode: CDNPathModeBucketPrefix,
	}
	key := "videos/12/53/index.m3u8"
	if got := DeliveryObjectPath(opts, key); got != "dongman/videos/12/53/index.m3u8" {
		t.Fatalf("bucket_prefix path: %q", got)
	}
	opts.PathMode = CDNPathModeObjectKey
	if got := DeliveryObjectPath(opts, key); got != "videos/12/53/index.m3u8" {
		t.Fatalf("object_key path: %q", got)
	}
}

func TestOptionsFromEnv_appliesPathMode(t *testing.T) {
	opts := OptionsFromEnv("https://media.example.com/", "", "dongman", CDNPathModeObjectKey)
	if opts.BaseURL != "https://media.example.com" {
		t.Fatalf("base %q", opts.BaseURL)
	}
	if opts.PathMode != CDNPathModeObjectKey {
		t.Fatalf("path mode %q", opts.PathMode)
	}
	got := BuildMediaURL(opts, "videos/1/2/index.m3u8")
	want := "https://media.example.com/videos/1/2/index.m3u8"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
