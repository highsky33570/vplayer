package config

import (
	"os"
	"testing"

	"github.com/tycdn/vplayer/internal/media"
)

func TestLoad_mediaCDNPathModeDefault(t *testing.T) {
	t.Setenv("MEDIA_CDN_PATH_MODE", "")
	_ = os.Unsetenv("MEDIA_CDN_PATH_MODE")
	cfg := Load()
	if cfg.MediaCDNPathMode != media.CDNPathModeBucketPrefix {
		t.Fatalf("default mode=%q want %q", cfg.MediaCDNPathMode, media.CDNPathModeBucketPrefix)
	}
}

func TestLoad_mediaCDNPathModeObjectKey(t *testing.T) {
	t.Setenv("MEDIA_CDN_PATH_MODE", "object_key")
	cfg := Load()
	if cfg.MediaCDNPathMode != media.CDNPathModeObjectKey {
		t.Fatalf("mode=%q want %q", cfg.MediaCDNPathMode, media.CDNPathModeObjectKey)
	}
}

func TestLoad_mediaCDNPathModeExplicitBucketPrefix(t *testing.T) {
	t.Setenv("MEDIA_CDN_PATH_MODE", "bucket_prefix")
	cfg := Load()
	if cfg.MediaCDNPathMode != media.CDNPathModeBucketPrefix {
		t.Fatalf("mode=%q want %q", cfg.MediaCDNPathMode, media.CDNPathModeBucketPrefix)
	}
}
