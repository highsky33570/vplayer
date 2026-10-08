package maccms

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	reImgTag = regexp.MustCompile(`(?is)<img\b[^>]*>`)
	reAttr   = func(name string) *regexp.Regexp {
		return regexp.MustCompile(`(?i)` + name + `=["']([^"']+)["']`)
	}
	reDataOriginal = reAttr("data-original")
	reDataSrc      = reAttr("data-src")
	reDataPic      = reAttr("data-pic")
	reSrc          = reAttr("src")
	reBgImage      = regexp.MustCompile(`(?i)background(?:-image)?\s*:\s*url\(["']?([^)"']+)["']?\)`)
)

// ExtractPosters returns candidate poster URLs from HTML, filtering chrome assets.
func ExtractPosters(html, baseURL string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		abs := AbsoluteURL(baseURL, raw)
		if abs == "" || seen[abs] || isChromeAsset(abs) {
			return
		}
		seen[abs] = true
		out = append(out, abs)
	}

	for _, tag := range reImgTag.FindAllString(html, -1) {
		for _, re := range []*regexp.Regexp{reDataOriginal, reDataSrc, reDataPic, reSrc} {
			if m := re.FindStringSubmatch(tag); len(m) == 2 {
				add(m[1])
			}
		}
	}
	for _, m := range reBgImage.FindAllStringSubmatch(html, -1) {
		add(m[1])
	}
	return out
}

// BestPoster picks the most likely content poster.
func BestPoster(html, baseURL string) string {
	cands := ExtractPosters(html, baseURL)
	if len(cands) == 0 {
		return ""
	}
	for _, c := range cands {
		if IsValidContentPoster(c) && strings.Contains(strings.ToLower(c), "/upload/vod/") {
			return c
		}
	}
	for _, c := range cands {
		if IsValidContentPoster(c) {
			low := strings.ToLower(c)
			if strings.Contains(low, "/vod/") || strings.Contains(low, "poster") || strings.Contains(low, "cover") {
				return c
			}
		}
	}
	for _, c := range cands {
		if IsValidContentPoster(c) {
			return c
		}
	}
	return ""
}

// ExtractPosterNear finds a poster near a detail URL occurrence.
func ExtractPosterNear(html, detailURL string) string {
	idx := strings.Index(html, detailURL)
	if idx < 0 {
		// try path-only
		if u, err := url.Parse(detailURL); err == nil {
			idx = strings.Index(html, u.Path)
		}
	}
	if idx < 0 {
		return BestPoster(html, detailURL)
	}
	start := idx - 800
	if start < 0 {
		start = 0
	}
	end := idx + 800
	if end > len(html) {
		end = len(html)
	}
	return BestPoster(html[start:end], detailURL)
}

// IsPlaceholderPoster reports site default / UI art that must not become cover URLs.
func IsPlaceholderPoster(u string) bool {
	return isChromeAsset(u)
}

// IsValidContentPoster is true for a non-empty, non-placeholder poster URL.
func IsValidContentPoster(u string) bool {
	u = strings.TrimSpace(u)
	return u != "" && !IsPlaceholderPoster(u)
}

// ChooseContentPoster prefers a real detail poster; otherwise keeps a real catalog poster.
func ChooseContentPoster(detailPoster, catalogPoster string) string {
	if IsValidContentPoster(detailPoster) && !IsPlaceholderPoster(detailPoster) {
		// Prefer detail when it looks like a real vod asset.
		if strings.Contains(strings.ToLower(detailPoster), "/upload/vod/") {
			return detailPoster
		}
		if !IsValidContentPoster(catalogPoster) {
			return detailPoster
		}
		// Detail is non-placeholder but catalog has /upload/vod/ — prefer catalog.
		if strings.Contains(strings.ToLower(catalogPoster), "/upload/vod/") &&
			!strings.Contains(strings.ToLower(detailPoster), "/upload/vod/") {
			return catalogPoster
		}
		return detailPoster
	}
	if IsValidContentPoster(catalogPoster) {
		return catalogPoster
	}
	return ""
}

func isChromeAsset(u string) bool {
	low := strings.ToLower(strings.TrimSpace(u))
	if low == "" {
		return true
	}
	for _, bad := range []string{
		"logo", "favicon", "icon", "avatar", "qrcode", "qr.png",
		"banner_ad", "/ads/", "sprite", "loading.gif", "load.gif",
		"placeholder", "/static/images/img/hd.png", "hd.png",
		"/static/images/img/",
	} {
		if strings.Contains(low, bad) {
			return true
		}
	}
	return false
}
