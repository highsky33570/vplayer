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
	reBgImage = regexp.MustCompile(`(?i)background(?:-image)?\s*:\s*url\(["']?([^)"']+)["']?\)`)
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
		low := strings.ToLower(c)
		if strings.Contains(low, "/upload/vod/") || strings.Contains(low, "/vod/") ||
			strings.Contains(low, "poster") || strings.Contains(low, "cover") {
			return c
		}
	}
	return cands[0]
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

func isChromeAsset(u string) bool {
	low := strings.ToLower(u)
	for _, bad := range []string{
		"logo", "favicon", "icon", "avatar", "qrcode", "qr.png",
		"banner_ad", "/ads/", "sprite", "loading.gif", "placeholder",
	} {
		if strings.Contains(low, bad) {
			return true
		}
	}
	return false
}
