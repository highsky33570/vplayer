package mediasign

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/tycdn/vplayer/internal/media"
)

var uriAttr = regexp.MustCompile(`URI="([^"]*)"`)

// RewritePlaylist rewrites media references onto signed gateway URLs.
// Absolute URLs that are not this site's own CDN objects fail closed.
func RewritePlaylist(body, currentKey string, opts media.CDNURLOptions, sign func(objectKey string) (string, error)) (string, error) {
	current, err := CanonicalObjectKey(currentKey)
	if err != nil {
		return "", err
	}
	nl := "\n"
	if strings.Contains(body, "\r\n") {
		nl = "\r\n"
	}
	lines := strings.Split(body, nl)
	for i, line := range lines {
		raw := strings.TrimRight(line, "\r")
		if raw == "" || (strings.HasPrefix(raw, "#") && !strings.Contains(raw, `URI="`)) {
			continue
		}
		rewritten, err := rewriteLine(raw, current, opts, sign)
		if err != nil {
			return "", err
		}
		lines[i] = rewritten
	}
	return strings.Join(lines, nl), nil
}

func rewriteLine(line, currentKey string, opts media.CDNURLOptions, sign func(objectKey string) (string, error)) (string, error) {
	if strings.HasPrefix(line, "#") {
		var rewriteErr error
		out := uriAttr.ReplaceAllStringFunc(line, func(match string) string {
			if rewriteErr != nil {
				return match
			}
			sub := uriAttr.FindStringSubmatch(match)
			if len(sub) != 2 {
				rewriteErr = errDenied
				return match
			}
			signed, err := signRef(currentKey, sub[1], opts, sign)
			if err != nil {
				rewriteErr = err
				return match
			}
			return `URI="` + signed + `"`
		})
		if rewriteErr != nil {
			return "", rewriteErr
		}
		return out, nil
	}
	return signRef(currentKey, strings.TrimSpace(line), opts, sign)
}

func signRef(currentKey, ref string, opts media.CDNURLOptions, sign func(objectKey string) (string, error)) (string, error) {
	key, err := ResolveRef(currentKey, ref, opts)
	if err != nil {
		return "", err
	}
	return sign(key)
}

// ResolveRef maps a playlist reference onto a videos/ object key.
// External absolute URLs are rejected so the gateway cannot become an open proxy.
func ResolveRef(currentKey, ref string, opts media.CDNURLOptions) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.ContainsAny(ref, " \t\r\n") || strings.Contains(ref, "?") || strings.Contains(ref, "#") {
		return "", errDenied
	}
	if strings.Contains(ref, "://") || strings.HasPrefix(ref, "//") {
		return objectKeyUnderPlaylist(currentKey, func() (string, error) {
			return objectKeyFromAbsolute(ref, opts)
		})
	}
	if strings.HasPrefix(ref, "/") {
		return "", errDenied
	}
	dir := currentKey
	if i := strings.LastIndex(currentKey, "/"); i >= 0 {
		dir = currentKey[:i]
	}
	parts := strings.Split(dir, "/")
	for _, p := range strings.Split(ref, "/") {
		switch p {
		case "", ".":
			continue
		case "..":
			if len(parts) == 0 {
				return "", errDenied
			}
			parts = parts[:len(parts)-1]
		default:
			parts = append(parts, p)
		}
	}
	return objectKeyUnderPlaylist(currentKey, func() (string, error) {
		return CanonicalObjectKey(strings.Join(parts, "/"))
	})
}

func playlistDir(currentKey string) string {
	if i := strings.LastIndex(currentKey, "/"); i >= 0 {
		return currentKey[:i]
	}
	return currentKey
}

func objectKeyUnderPlaylist(currentKey string, resolve func() (string, error)) (string, error) {
	key, err := resolve()
	if err != nil {
		return "", err
	}
	dir := playlistDir(currentKey)
	if key != currentKey && !strings.HasPrefix(key, dir+"/") {
		return "", errDenied
	}
	return key, nil
}

// ObjectKeyFromCDNURL maps a durable CDN URL back to a videos/ object key.
// It does not accept arbitrary hosts.
func ObjectKeyFromCDNURL(raw string, opts media.CDNURLOptions) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", errDenied
	}
	u.RawQuery = ""
	u.Fragment = ""
	return objectKeyFromAbsolute(u.String(), opts)
}

func objectKeyFromAbsolute(raw string, opts media.CDNURLOptions) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errDenied
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", errDenied
	}
	base, err := url.Parse(opts.BaseURL)
	if err != nil || base.Host == "" || !strings.EqualFold(u.Host, base.Host) {
		return "", fmt.Errorf("%w", errDenied)
	}
	path := strings.Trim(u.EscapedPath(), "/")
	if decoded, err := url.PathUnescape(path); err == nil {
		path = decoded
	}
	if basePath := strings.Trim(base.EscapedPath(), "/"); basePath != "" {
		if decoded, err := url.PathUnescape(basePath); err == nil {
			basePath = decoded
		}
		prefix := basePath + "/"
		if !strings.HasPrefix(path, prefix) {
			return "", errDenied
		}
		path = strings.TrimPrefix(path, prefix)
	}
	mode := opts.PathMode
	if mode == "" {
		mode = media.CDNPathModeBucketPrefix
	}
	switch mode {
	case media.CDNPathModeObjectKey:
	default:
		bucket := strings.Trim(opts.Bucket, "/")
		prefix := bucket + "/"
		if bucket == "" || !strings.HasPrefix(path, prefix) {
			return "", errDenied
		}
		path = strings.TrimPrefix(path, prefix)
	}
	return CanonicalObjectKey(path)
}
