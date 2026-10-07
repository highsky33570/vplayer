package maccms

import (
	"regexp"
	"strings"
)

var (
	reNavTypeLink = regexp.MustCompile(`(?is)<a[^>]+href=["']([^"']*vod/type/id/\d+[^"']*)["'][^>]*>(.*?)</a>`)
	reStripTags   = regexp.MustCompile(`(?s)<[^>]+>`)
	reWS          = regexp.MustCompile(`\s+`)
)

type NavCategory struct {
	SourceID string
	Name     string
	URL      string
	Sort     int
}

// ParseNavigationCategories extracts MacCMS type links from homepage/nav HTML.
func ParseNavigationCategories(html, baseURL string) []NavCategory {
	seen := map[string]bool{}
	var out []NavCategory
	sort := 10
	for _, m := range reNavTypeLink.FindAllStringSubmatch(html, -1) {
		href := AbsoluteURL(baseURL, m[1])
		id, ok := ExtractTypeID(href)
		if !ok || seen[id] {
			continue
		}
		name := cleanText(m[2])
		if name == "" || isChromeLabel(name) {
			continue
		}
		seen[id] = true
		out = append(out, NavCategory{
			SourceID: id,
			Name:     name,
			URL:      href,
			Sort:     sort,
		})
		sort += 10
	}
	return out
}

func cleanText(s string) string {
	s = reStripTags.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = reWS.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func isChromeLabel(name string) bool {
	n := strings.ToLower(name)
	switch n {
	case "首页", "home", "登录", "注册", "搜索", "留言", "app", "更多", "more":
		return true
	}
	return false
}
