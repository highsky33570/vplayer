package maccms

import (
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var (
	reTypeID   = regexp.MustCompile(`(?i)/vod/type/id/(\d+)`)
	reDetailID = regexp.MustCompile(`(?i)/vod/detail/id/(\d+)`)
	rePlayIDs  = regexp.MustCompile(`(?i)/vod/play/id/(\d+)/sid/(\d+)/nid/(\d+)`)
	reVodIDQ   = regexp.MustCompile(`(?i)[?&]id=(\d+)`)
	rePageNum  = regexp.MustCompile(`(?i)/page/(\d+)`)
	reTypePage = regexp.MustCompile(`(?i)/vod/type/id/(\d+)(?:/page/(\d+))?`)
)

// AbsoluteURL resolves href against base.
func AbsoluteURL(base, href string) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "javascript:") || href == "#" {
		return ""
	}
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if u.IsAbs() {
		return u.String()
	}
	b, err := url.Parse(base)
	if err != nil {
		return href
	}
	return b.ResolveReference(u).String()
}

func ExtractTypeID(rawURL string) (string, bool) {
	if m := reTypeID.FindStringSubmatch(rawURL); len(m) == 2 {
		return m[1], true
	}
	return "", false
}

func ExtractVodID(rawURL string) (string, bool) {
	if m := reDetailID.FindStringSubmatch(rawURL); len(m) == 2 {
		return m[1], true
	}
	if m := rePlayIDs.FindStringSubmatch(rawURL); len(m) >= 2 {
		return m[1], true
	}
	if m := reVodIDQ.FindStringSubmatch(rawURL); len(m) == 2 {
		return m[1], true
	}
	// filename fallback: detail_10001.html / vod-detail-id-10001.html
	base := path.Base(strings.Split(rawURL, "?")[0])
	base = strings.TrimSuffix(base, path.Ext(base))
	for _, p := range []string{"detail_", "vod-detail-id-", "id-"} {
		if strings.HasPrefix(strings.ToLower(base), p) {
			id := strings.TrimPrefix(strings.ToLower(base), p)
			if _, err := strconv.Atoi(id); err == nil {
				return id, true
			}
		}
	}
	return "", false
}

func ExtractPlayRef(rawURL string) (vodID string, sid, nid int, ok bool) {
	if m := rePlayIDs.FindStringSubmatch(rawURL); len(m) == 4 {
		sid, _ = strconv.Atoi(m[2])
		nid, _ = strconv.Atoi(m[3])
		return m[1], sid, nid, true
	}
	return "", 0, 0, false
}

func ExtractPageNumber(rawURL string) int {
	if m := rePageNum.FindStringSubmatch(rawURL); len(m) == 2 {
		n, _ := strconv.Atoi(m[1])
		if n > 0 {
			return n
		}
	}
	if m := reTypePage.FindStringSubmatch(rawURL); len(m) == 3 && m[2] != "" {
		n, _ := strconv.Atoi(m[2])
		if n > 0 {
			return n
		}
	}
	return 1
}

func Slugify(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.ReplaceAll(name, " ", "-")
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if s == "" {
		return "cat"
	}
	return s
}
