package media

import (
	"strconv"
	"strings"
)

// CDNURLOptions controls how object keys become browser-facing CDN URLs.
// TYCDN/CDNfly private-R2 origins require the bucket name in the request path:
//
//	https://MEDIA_CDN_HOST/dongman/posters/100.jpg
type CDNURLOptions struct {
	// BaseURL is MEDIA_CDN_BASE_URL (or CDN_BASE_URL fallback), without trailing slash.
	BaseURL string
	// Bucket is the R2 bucket name that must appear as the first path segment (e.g. dongman).
	Bucket string
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

// BuildMediaURL turns a relative object key into a CDN URL under /{bucket}/...
// Absolute http(s) URLs are returned unchanged for progressive migration.
// Signing is intentionally not applied here — CDNfly authenticates to private R2.
// Callers that need VPlayer's legacy exp/sig can wrap the result with play.SignURL.
func BuildMediaURL(opts CDNURLOptions, objectPath string) string {
	objectPath = strings.TrimSpace(objectPath)
	if objectPath == "" {
		return ""
	}
	if isAbsoluteURL(objectPath) {
		return objectPath
	}

	base := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	pathWithBucket := ObjectPathWithBucket(opts.Bucket, objectPath)
	if base == "" {
		return "/" + pathWithBucket
	}
	return base + "/" + pathWithBucket
}

// ResolveMediaURL prefers an object key (R2/CDN) over a legacy absolute source URL.
func ResolveMediaURL(opts CDNURLOptions, objectKey, fallbackURL string) string {
	if key := strings.TrimSpace(objectKey); key != "" {
		return BuildMediaURL(opts, key)
	}
	return strings.TrimSpace(fallbackURL)
}

// OptionsFromEnv builds CDNURLOptions from common config fields.
func OptionsFromEnv(mediaCDNBase, cdnBase, bucket string) CDNURLOptions {
	base := strings.TrimRight(strings.TrimSpace(mediaCDNBase), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(cdnBase), "/")
	}
	return CDNURLOptions{BaseURL: base, Bucket: strings.TrimSpace(bucket)}
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
