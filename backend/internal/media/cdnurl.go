package media

import (
	"fmt"
	"strconv"
	"strings"
)

// CDNPathMode controls whether browser-facing media URLs include the bucket
// name as a path prefix.
type CDNPathMode string

const (
	// CDNPathModeBucketPrefix builds {base}/{bucket}/{key} (historical default).
	CDNPathModeBucketPrefix CDNPathMode = "bucket_prefix"
	// CDNPathModeObjectKey builds {base}/{key} for hosts that already map to the bucket root.
	CDNPathModeObjectKey CDNPathMode = "object_key"
)

// ParseCDNPathMode accepts MEDIA_CDN_PATH_MODE values.
// Empty/unset → bucket_prefix (production-compatible default).
// Unsupported values return an error (no silent fallback).
func ParseCDNPathMode(raw string) (CDNPathMode, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return CDNPathModeBucketPrefix, nil
	}
	mode := CDNPathMode(s)
	switch mode {
	case CDNPathModeBucketPrefix, CDNPathModeObjectKey:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported MEDIA_CDN_PATH_MODE %q (supported: %s, %s)",
			raw, CDNPathModeBucketPrefix, CDNPathModeObjectKey)
	}
}

func (m CDNPathMode) normalized() CDNPathMode {
	if m == "" {
		return CDNPathModeBucketPrefix
	}
	return m
}

// CDNURLOptions controls how object keys become browser-facing media URLs.
type CDNURLOptions struct {
	// BaseURL is MEDIA_CDN_BASE_URL (or CDN_BASE_URL fallback), without trailing slash.
	BaseURL string
	// Bucket is the R2 bucket name (used when PathMode is bucket_prefix).
	Bucket string
	// PathMode selects URL path shape. Empty means CDNPathModeBucketPrefix.
	PathMode CDNPathMode
}

// ObjectPathWithBucket returns bucket/key for CDN path construction (no host).
func ObjectPathWithBucket(bucket, objectPath string) string {
	bucket = strings.Trim(strings.TrimSpace(bucket), "/")
	key := strings.TrimLeft(filepathToSlash(strings.TrimSpace(objectPath)), "/")
	if key == "" {
		return bucket
	}
	if bucket != "" && (key == bucket || strings.HasPrefix(key, bucket+"/")) {
		return key
	}
	if bucket == "" {
		return key
	}
	return bucket + "/" + key
}

// DeliveryObjectPath returns the URL path (no leading slash, no host) for a relative object key.
func DeliveryObjectPath(opts CDNURLOptions, objectPath string) string {
	key := strings.TrimLeft(filepathToSlash(strings.TrimSpace(objectPath)), "/")
	switch opts.PathMode.normalized() {
	case CDNPathModeObjectKey:
		return key
	default:
		return ObjectPathWithBucket(opts.Bucket, objectPath)
	}
}

// BuildMediaURL turns a relative object key into a browser-facing media URL.
// Absolute http(s) URLs are returned unchanged for progressive migration.
// Signing is intentionally not applied here.
// Callers that need VPlayer's legacy exp/sig can wrap the result with play.SignURL
// using DeliveryObjectPath for the signed path.
func BuildMediaURL(opts CDNURLOptions, objectPath string) string {
	objectPath = strings.TrimSpace(objectPath)
	if objectPath == "" {
		return ""
	}
	if isAbsoluteURL(objectPath) {
		return objectPath
	}

	base := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	path := DeliveryObjectPath(opts, objectPath)
	if path == "" {
		return base
	}
	if base == "" {
		return "/" + path
	}
	return base + "/" + path
}

// ResolveMediaURL prefers an object key (R2/CDN) over a legacy absolute source URL.
func ResolveMediaURL(opts CDNURLOptions, objectKey, fallbackURL string) string {
	if key := strings.TrimSpace(objectKey); key != "" {
		return BuildMediaURL(opts, key)
	}
	return strings.TrimSpace(fallbackURL)
}

// OptionsFromEnv builds CDNURLOptions from common config fields.
// pathMode should already be validated (e.g. via ParseCDNPathMode / config.Load).
func OptionsFromEnv(mediaCDNBase, cdnBase, bucket string, pathMode CDNPathMode) CDNURLOptions {
	base := strings.TrimRight(strings.TrimSpace(mediaCDNBase), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(cdnBase), "/")
	}
	return CDNURLOptions{
		BaseURL:  base,
		Bucket:   strings.TrimSpace(bucket),
		PathMode: pathMode.normalized(),
	}
}

func isAbsoluteURL(s string) bool {
	l := strings.ToLower(s)
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://")
}

func filepathToSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

// EpisodeHLSObjectKey is the deterministic VOD layout under the bucket:
//
//	videos/<videoID>/<episodeID>/index.m3u8
func EpisodeHLSObjectKey(videoID, episodeID uint64) string {
	return "videos/" + strconv.FormatUint(videoID, 10) + "/" + strconv.FormatUint(episodeID, 10) + "/index.m3u8"
}
