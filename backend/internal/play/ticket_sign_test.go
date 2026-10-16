package play

import (
	"strings"
	"testing"

	"github.com/tycdn/vplayer/internal/media"
)

func TestSignURL_bucketPrefixPath(t *testing.T) {
	cdn := media.CDNURLOptions{
		BaseURL:  "https://media.example.com",
		Bucket:   "dongman",
		PathMode: media.CDNPathModeBucketPrefix,
	}
	path := media.DeliveryObjectPath(cdn, "videos/12/53/index.m3u8")
	got := SignURL(cdn.BaseURL, "secret", path, 300)
	if !strings.HasPrefix(got, "https://media.example.com/dongman/videos/12/53/index.m3u8?exp=") {
		t.Fatalf("unexpected signed url: %s", got)
	}
	if !strings.Contains(got, "&sig=") {
		t.Fatalf("missing sig: %s", got)
	}
}

func TestSignURL_objectKeyPath(t *testing.T) {
	cdn := media.CDNURLOptions{
		BaseURL:  "https://media.example.com",
		Bucket:   "dongman",
		PathMode: media.CDNPathModeObjectKey,
	}
	path := media.DeliveryObjectPath(cdn, "videos/12/53/index.m3u8")
	got := SignURL(cdn.BaseURL, "secret", path, 300)
	if !strings.HasPrefix(got, "https://media.example.com/videos/12/53/index.m3u8?exp=") {
		t.Fatalf("unexpected signed url: %s", got)
	}
	if strings.Contains(got, "/dongman/") {
		t.Fatalf("object_key mode must not include bucket in signed path: %s", got)
	}
}
