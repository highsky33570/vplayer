package play

import (
	"strings"
)

// playbackStatusAllowsFallback is fail-closed: only known playable statuses qualify.
func playbackStatusAllowsFallback(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "ok", "available":
		return true
	default:
		return false
	}
}

func isAbsoluteHTTPURL(s string) bool {
	l := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://")
}

// EligibleFallbackURL returns a source fallback URL when primary came from an
// object key and the preserved absolute playback_url is safely playable.
// Empty string means do not expose fallback_url.
func EligibleFallbackURL(primaryFromObjectKey bool, sourcePlaybackURL, playbackStatus, primaryURL string) string {
	if !primaryFromObjectKey {
		return ""
	}
	if !playbackStatusAllowsFallback(playbackStatus) {
		return ""
	}
	src := strings.TrimSpace(sourcePlaybackURL)
	if src == "" || !isAbsoluteHTTPURL(src) {
		return ""
	}
	primary := strings.TrimSpace(primaryURL)
	if primary != "" && strings.EqualFold(src, primary) {
		return ""
	}
	return src
}
