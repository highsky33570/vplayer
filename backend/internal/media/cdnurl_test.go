package media

import "testing"

func TestBuildMediaURL_preservesBucketPrefix(t *testing.T) {
	opts := CDNURLOptions{
		BaseURL: "https://media.example.com",
		Bucket:  "dongman",
	}
	got := BuildMediaURL(opts, "posters/100.jpg")
	want := "https://media.example.com/dongman/posters/100.jpg"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
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
