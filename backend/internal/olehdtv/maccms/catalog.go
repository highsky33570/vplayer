package maccms

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	reDetailLink = regexp.MustCompile(`(?is)<a[^>]+href=["']([^"']*vod/detail/id/\d+[^"']*)["'][^>]*>`)
	reTitleAttr  = regexp.MustCompile(`(?i)title=["']([^"']+)["']`)
	reNextPage   = regexp.MustCompile(`(?is)<a[^>]+href=["']([^"']+)["'][^>]*(?:class=["'][^"']*page.?next[^"']*["']|rel=["']next["'])[^>]*>|<a[^>]*(?:class=["'][^"']*page.?next[^"']*["']|rel=["']next["'])[^>]+href=["']([^"']+)["']`)
	rePageLinks  = regexp.MustCompile(`(?is)<a[^>]+href=["']([^"']*vod/type/id/\d+(?:/page/\d+)?[^"']*)["'][^>]*>`)
	reHiddenPage = regexp.MustCompile(`(?i)name=["']?page["']?[^>]*value=["']?(\d+)`)
)

type CatalogItem struct {
	VodID    string
	Title    string
	DetailURL string
	PosterURL string
	Status   string
	TypeID   string
}

type Pagination struct {
	Current   int
	NextURL   string
	PageURLs  []string
	HasNext   bool
}

// ParseCatalogPage extracts video cards and pagination from a MacCMS type listing page.
func ParseCatalogPage(html, pageURL, typeID string) ([]CatalogItem, Pagination) {
	items := parseCatalogItems(html, pageURL, typeID)
	pag := parsePagination(html, pageURL)
	return DedupCatalogItems(items), pag
}

func parseCatalogItems(html, pageURL, typeID string) []CatalogItem {
	seen := map[string]bool{}
	var out []CatalogItem

	// Prefer detail links anywhere on page.
	for _, m := range reDetailLink.FindAllStringSubmatch(html, -1) {
		href := AbsoluteURL(pageURL, m[1])
		id, ok := ExtractVodID(href)
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		title := ""
		if tm := reTitleAttr.FindStringSubmatch(m[0]); len(tm) == 2 {
			title = cleanText(tm[1])
		}
		poster := ExtractPosterNear(html, href)
		out = append(out, CatalogItem{
			VodID:     id,
			Title:     title,
			DetailURL: href,
			PosterURL: poster,
			TypeID:    typeID,
		})
	}

	for i := range out {
		if out[i].Title == "" {
			out[i].Title = "vod-" + out[i].VodID
		}
	}
	return out
}

func parsePagination(html, pageURL string) Pagination {
	cur := ExtractPageNumber(pageURL)
	pag := Pagination{Current: cur}
	seen := map[string]bool{}
	var next string
	if m := reNextPage.FindStringSubmatch(html); len(m) > 0 {
		href := m[1]
		if href == "" {
			href = m[2]
		}
		next = AbsoluteURL(pageURL, href)
	}
	for _, m := range rePageLinks.FindAllStringSubmatch(html, -1) {
		u := AbsoluteURL(pageURL, m[1])
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		pag.PageURLs = append(pag.PageURLs, u)
		pn := ExtractPageNumber(u)
		if next == "" && pn == cur+1 {
			next = u
		}
	}
	if next == "" {
		// Synthesize next page URL if page+1 link pattern exists in HTML as text page numbers.
		if strings.Contains(html, "/page/"+strconv.Itoa(cur+1)) ||
			strings.Contains(html, "page/"+strconv.Itoa(cur+1)+".html") {
			candidate := synthesizeNextPage(pageURL, cur+1)
			if candidate != "" && strings.Contains(html, strconv.Itoa(cur+1)) {
				next = candidate
			}
		}
	}
	pag.NextURL = next
	pag.HasNext = next != "" && next != pageURL
	return pag
}

func synthesizeNextPage(pageURL string, page int) string {
	if page <= 1 {
		return pageURL
	}
	if strings.Contains(pageURL, "/page/") {
		return rePageNum.ReplaceAllString(pageURL, "/page/"+strconv.Itoa(page))
	}
	// .../id/1.html -> .../id/1/page/2.html
	if i := strings.LastIndex(pageURL, ".html"); i > 0 {
		base := pageURL[:i]
		return base + "/page/" + strconv.Itoa(page) + ".html"
	}
	return ""
}

func DedupCatalogItems(items []CatalogItem) []CatalogItem {
	seen := map[string]bool{}
	out := make([]CatalogItem, 0, len(items))
	for _, it := range items {
		if it.VodID == "" || seen[it.VodID] {
			continue
		}
		seen[it.VodID] = true
		out = append(out, it)
	}
	return out
}

// DetectPaginationLoop returns true if next URL was already visited.
func DetectPaginationLoop(visited map[string]bool, nextURL string) bool {
	if nextURL == "" {
		return false
	}
	return visited[normalizeVisitKey(nextURL)]
}

func normalizeVisitKey(u string) string {
	u = strings.TrimSpace(strings.ToLower(u))
	u = strings.TrimRight(u, "/")
	return u
}

func MarkVisited(visited map[string]bool, u string) {
	visited[normalizeVisitKey(u)] = true
}
