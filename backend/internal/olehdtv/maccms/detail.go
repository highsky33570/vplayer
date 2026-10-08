package maccms

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

var (
	reH1Title = regexp.MustCompile(`(?is)<h1\b([^>]*)>(.*?)</h1>`)
	reHTitle  = regexp.MustCompile(`(?is)<h([123])\b([^>]*)>(.*?)</h[123]>`)
	reDocTitle = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reOGTitle  = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:title["'][^>]+content=["']([^"']+)["']|<meta[^>]+content=["']([^"']+)["'][^>]+property=["']og:title["']`)
	reOGDesc   = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:description["'][^>]+content=["']([^"']+)["']|<meta[^>]+content=["']([^"']+)["'][^>]+property=["']og:description["']`)
	reOGImage  = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:image["'][^>]+content=["']([^"']+)["']|<meta[^>]+content=["']([^"']+)["'][^>]+property=["']og:image["']`)
	reJSONLD   = regexp.MustCompile(`(?is)<script[^>]+type=["']application/ld\+json["'][^>]*>(.*?)</script>`)
	reMetaDesc = regexp.MustCompile(`(?is)<meta[^>]+name=["']description["'][^>]+content=["']([^"']*)["']|<meta[^>]+content=["']([^"']*)["'][^>]+name=["']description["']`)
	reLabeled  = regexp.MustCompile(`(?is)(?:主演|演员|actors?)\s*[:：]</?\w*>?\s*([^<]{1,200})|(?:导演|director)\s*[:：]</?\w*>?\s*([^<]{1,120})|(?:地区|area|region)\s*[:：]</?\w*>?\s*([^<]{1,80})|(?:年份|year)\s*[:：]</?\w*>?\s*([^<]{1,20})|(?:状态|更新|remarks?)\s*[:：]</?\w*>?\s*([^<]{1,80})`)
	reYearLoose   = regexp.MustCompile(`(?i)(?:年份|year)[^0-9]{0,20}(19\d{2}|20\d{2})`)
	reAreaLoose   = regexp.MustCompile(`(?is)(?:地区|area)[^<\n]{0,10}[:：]\s*([^<\n|]{1,40})`)
	reDirector    = regexp.MustCompile(`(?is)(?:导演|director)[^<\n]{0,10}[:：]\s*([^<\n|]{1,80})`)
	reActors      = regexp.MustCompile(`(?is)(?:主演|演员|actors?)[^<\n]{0,10}[:：]\s*([^<\n]{1,200})`)
	reStatusLoose = regexp.MustCompile(`(?is)(?:状态|更新)[^<\n]{0,10}[:：]\s*([^<\n|]{1,80})`)
	reContent     = regexp.MustCompile(`(?is)(?:class=["'][^"']*(?:content|desc|detail|sketch|txt)[^"']*["'][^>]*>)([\s\S]+?)</`)
	rePlayLink    = regexp.MustCompile(`(?is)<a[^>]+href=["']([^"']*vod/play/id/\d+/sid/\d+/nid/\d+[^"']*)["'][^>]*>(.*?)</a>`)
	reScore       = regexp.MustCompile(`(?i)(?:评分|score)[^0-9]{0,12}(\d+(?:\.\d+)?)`)
	reDetailRegion = regexp.MustCompile(`(?is)<(?:div|section)[^>]+class=["'][^"']*(?:myui-content__detail|stui-content__detail|content__detail|vod-detail|detail-info|myui-panel__bd)[^"']*["'][^>]*>`)
	reTitleClass   = regexp.MustCompile(`(?i)\b(?:title|page-title|vod-title|detail-title|movie-title|myui-content__title)\b`)
)

type DetailMeta struct {
	VodID       string
	Title       string
	Description string
	Year        string
	Area        string
	Director    string
	Actors      string
	Status      string
	PosterURL   string
	DetailURL   string
	TypeID      string
	Rating      float64
	Episodes    []EpisodeRef
}

type EpisodeRef struct {
	VodID             string
	SID               int
	NID               int
	Label             string
	PlayURL           string
	From              string
	StreamURL         string // from player_aaaa when present
	Available         bool
	EnrichmentSkipped bool // play-page fetch intentionally skipped (budget cap)
}

// ParseDetailPage extracts MacCMS detail metadata and episode play links.
func ParseDetailPage(html, detailURL string) DetailMeta {
	vodID, _ := ExtractVodID(detailURL)
	meta := DetailMeta{
		VodID:     vodID,
		DetailURL: detailURL,
	}
	meta.Title = extractDetailTitle(html)
	meta.Description = extractDetailDescription(html)
	meta.PosterURL = extractDetailPoster(html, detailURL)

	// Strip executable/hidden markup before labeled field regexes so script text
	// cannot contaminate year/area/director/actors/status captures.
	scope := StripNonVisibleElements(detailContentScope(html))
	if m := reYearLoose.FindStringSubmatch(scope); len(m) == 2 {
		meta.Year = m[1]
	}
	if m := reAreaLoose.FindStringSubmatch(scope); len(m) == 2 {
		meta.Area = cleanText(m[1])
		if IsGenericSiteTitle(meta.Area) || len([]rune(meta.Area)) > 40 {
			meta.Area = ""
		}
	}
	if m := reDirector.FindStringSubmatch(scope); len(m) == 2 {
		meta.Director = cleanText(m[1])
		if IsGenericSiteTitle(meta.Director) {
			meta.Director = ""
		}
	}
	if m := reActors.FindStringSubmatch(scope); len(m) == 2 {
		meta.Actors = cleanText(m[1])
		if IsGenericSiteTitle(meta.Actors) {
			meta.Actors = ""
		}
	}
	if m := reStatusLoose.FindStringSubmatch(scope); len(m) == 2 {
		meta.Status = cleanText(m[1])
		if IsGenericSiteTitle(meta.Status) {
			meta.Status = ""
		}
	}
	if m := reScore.FindStringSubmatch(scope); len(m) == 2 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			meta.Rating = v
		}
	}
	if tid, ok := ExtractTypeID(detailURL); ok {
		meta.TypeID = tid
	}
	if meta.TypeID == "" {
		for _, c := range ParseNavigationCategories(html, detailURL) {
			meta.TypeID = c.SourceID
			break
		}
	}
	meta.Episodes = ParseEpisodeLinks(html, detailURL)
	meta.Episodes = DedupEpisodes(meta.Episodes)
	_ = reLabeled
	return meta
}

// MergeDetailWithCatalog applies detail fields onto catalog card data without
// letting generic site chrome overwrite a valid listing title/poster.
func MergeDetailWithCatalog(card CatalogItem, detail DetailMeta) DetailMeta {
	out := detail
	if out.VodID == "" {
		out.VodID = card.VodID
	}
	if out.TypeID == "" {
		out.TypeID = card.TypeID
	}
	if out.DetailURL == "" {
		out.DetailURL = card.DetailURL
	}
	out.Title = ChooseContentTitle(detail.Title, card.Title)
	if out.PosterURL == "" {
		out.PosterURL = card.PosterURL
	}
	if out.Title == "" {
		out.Title = card.Title
	}
	return out
}

// ChooseContentTitle prefers a content-specific detail title; otherwise keeps catalog.
func ChooseContentTitle(detailTitle, catalogTitle string) string {
	detailTitle = strings.TrimSpace(detailTitle)
	catalogTitle = strings.TrimSpace(catalogTitle)
	if IsValidContentTitle(detailTitle) {
		return detailTitle
	}
	if IsValidContentTitle(catalogTitle) {
		return catalogTitle
	}
	if catalogTitle != "" && !IsGenericSiteTitle(catalogTitle) {
		return catalogTitle
	}
	if detailTitle != "" && !IsGenericSiteTitle(detailTitle) {
		return detailTitle
	}
	return catalogTitle
}

// IsValidContentTitle reports whether t looks like a real vod/movie title.
func IsValidContentTitle(t string) bool {
	t = strings.TrimSpace(t)
	if t == "" || IsGenericSiteTitle(t) {
		return false
	}
	if strings.HasPrefix(strings.ToLower(t), "vod-") {
		return false
	}
	if isChromeLabel(t) {
		return false
	}
	return true
}

// IsGenericSiteTitle detects site-branding / SEO chrome titles (not content titles).
func IsGenericSiteTitle(t string) bool {
	t = strings.TrimSpace(t)
	if t == "" {
		return true
	}
	compact := strings.ReplaceAll(t, " ", "")
	compact = strings.ReplaceAll(compact, "　", "")
	low := strings.ToLower(compact)

	// Exact / near-exact OLEHDTV branding observed in live dry-run.
	if strings.Contains(compact, "欧乐影院") && (strings.Contains(compact, "面向海外") || strings.Contains(compact, "在线视频媒体平台") || strings.Contains(compact, "海量高清")) {
		return true
	}
	// Generic MacCMS / video-portal SEO boilerplate.
	brandSignals := 0
	for _, sig := range []string{
		"在线视频媒体平台",
		"海量高清视频",
		"面向海外华人",
		"高清视频在线观看",
		"免费在线观看影视",
		"最新电影电视剧",
	} {
		if strings.Contains(compact, sig) {
			brandSignals++
		}
	}
	if brandSignals >= 1 && (strings.Contains(compact, "影院") || strings.Contains(compact, "影视") || strings.Contains(low, "maccms")) {
		return true
	}
	if brandSignals >= 2 {
		return true
	}
	// Very long titles that are mostly portal slogans.
	if len([]rune(t)) >= 28 && (strings.Contains(compact, "在线观看") && strings.Contains(compact, "平台")) {
		return true
	}
	// Bare site/logo names in header h1 (e.g. "欧乐影院") — not a vod title.
	if matched, _ := regexp.MatchString(`^[\p{Han}\w]{1,16}(影院|影视|视频网?|传媒)$`, compact); matched {
		return true
	}
	return false
}

func extractDetailTitle(html string) string {
	// 1) Headings inside detail content region (skip header branding h1).
	if t := firstValidHeading(html); t != "" {
		return t
	}
	// 2) JSON-LD name
	if t := titleFromJSONLD(html); IsValidContentTitle(t) {
		return sanitizeContentTitle(t)
	}
	// 3) og:title
	if m := reOGTitle.FindStringSubmatch(html); len(m) > 0 {
		t := sanitizeContentTitle(cleanText(firstGroup(m[1], m[2])))
		if IsValidContentTitle(t) {
			return t
		}
	}
	// 4) Document <title> with site-suffix stripped — never raw branding.
	if m := reDocTitle.FindStringSubmatch(html); len(m) == 2 {
		t := sanitizeContentTitle(cleanText(m[1]))
		if IsValidContentTitle(t) {
			return t
		}
	}
	return ""
}

func firstValidHeading(html string) string {
	try := func(chunk string, requireClass bool) string {
		for _, m := range reHTitle.FindAllStringSubmatch(chunk, -1) {
			attrs, inner := m[2], m[3]
			if requireClass && !reTitleClass.MatchString(attrs) {
				continue
			}
			t := sanitizeContentTitle(cleanText(inner))
			if IsValidContentTitle(t) {
				return t
			}
		}
		return ""
	}
	scope := detailContentScope(html)
	if scope != html {
		if t := try(scope, false); t != "" {
			return t
		}
	}
	// Prefer headings whose class looks content-specific.
	if t := try(html, true); t != "" {
		return t
	}
	// Last: any non-generic heading (rejects site branding via IsValidContentTitle).
	return try(html, false)
}

func titleFromJSONLD(html string) string {
	for _, m := range reJSONLD.FindAllStringSubmatch(html, -1) {
		raw := strings.TrimSpace(m[1])
		if raw == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(raw), &obj); err == nil {
			if name := jsonLDName(obj); name != "" {
				return name
			}
			continue
		}
		var arr []map[string]any
		if err := json.Unmarshal([]byte(raw), &arr); err == nil {
			for _, o := range arr {
				if name := jsonLDName(o); name != "" {
					return name
				}
			}
		}
	}
	return ""
}

func jsonLDName(obj map[string]any) string {
	if obj == nil {
		return ""
	}
	typ, _ := obj["@type"].(string)
	typ = strings.ToLower(typ)
	switch typ {
	case "movie", "tvseries", "tvepisode", "videoobject", "movietheater", "":
		// allow empty type if name looks content-like
	default:
		if typ != "movie" && !strings.Contains(typ, "video") && !strings.Contains(typ, "movie") && !strings.Contains(typ, "tv") {
			// still try name — many sites omit precise @type
		}
	}
	if name, ok := obj["name"].(string); ok {
		return cleanText(name)
	}
	if name, ok := obj["headline"].(string); ok {
		return cleanText(name)
	}
	return ""
}

func extractDetailDescription(html string) string {
	if m := reOGDesc.FindStringSubmatch(html); len(m) > 0 {
		d := cleanText(firstGroup(m[1], m[2]))
		if isValidDescription(d) {
			return d
		}
	}
	if m := reMetaDesc.FindStringSubmatch(html); len(m) > 0 {
		d := cleanText(firstGroup(m[1], m[2]))
		if isValidDescription(d) {
			return d
		}
	}
	scope := detailContentScope(html)
	if m := reContent.FindStringSubmatch(scope); len(m) == 2 {
		d := cleanText(m[1])
		if isValidDescription(d) {
			return d
		}
	}
	return ""
}

func isValidDescription(d string) bool {
	d = strings.TrimSpace(d)
	if d == "" || IsGenericSiteTitle(d) {
		return false
	}
	if strings.Contains(d, "在线视频媒体平台") || strings.Contains(d, "面向海外华人的在线") {
		return false
	}
	return true
}

func extractDetailPoster(html, detailURL string) string {
	if m := reOGImage.FindStringSubmatch(html); len(m) > 0 {
		u := AbsoluteURL(detailURL, firstGroup(m[1], m[2]))
		if u != "" && !isChromeAsset(u) {
			return u
		}
	}
	scope := detailContentScope(html)
	if p := BestPoster(scope, detailURL); p != "" {
		return p
	}
	return BestPoster(html, detailURL)
}

func detailContentScope(html string) string {
	loc := reDetailRegion.FindStringIndex(html)
	if loc == nil {
		return html
	}
	start := loc[1]
	end := start + 12000
	if end > len(html) {
		end = len(html)
	}
	chunk := html[start:end]
	if len(chunk) < 40 {
		return html
	}
	return chunk
}

func stripSiteTitleSuffix(t string) string {
	t = strings.TrimSpace(t)
	if t == "" {
		return ""
	}
	// Common patterns: "《Movie》高清在线观看 - 站点名" / "Movie_站点名"
	for _, sep := range []string{" - ", " – ", " — ", " | ", "_", "－"} {
		if i := strings.LastIndex(t, sep); i > 0 {
			left := strings.TrimSpace(t[:i])
			right := strings.TrimSpace(t[i+len(sep):])
			if left != "" && (IsGenericSiteTitle(right) || looksLikeSiteName(right)) {
				t = left
				break
			}
		}
	}
	t = strings.TrimSpace(t)
	t = strings.TrimPrefix(t, "《")
	t = strings.TrimSuffix(t, "》")
	for _, suffix := range []string{"高清在线观看", "在线观看", "高清下载", "免费观看"} {
		t = strings.TrimSuffix(t, suffix)
		t = strings.TrimSpace(t)
	}
	return strings.TrimSpace(t)
}

func looksLikeSiteName(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.Contains(s, "影院") || strings.Contains(s, "影视") || strings.Contains(strings.ToLower(s), "maccms") {
		return true
	}
	if len([]rune(s)) <= 12 && !strings.Contains(s, "第") {
		return true
	}
	return false
}

func sanitizeContentTitle(t string) string {
	t = strings.TrimSpace(t)
	t = stripSiteTitleSuffix(t)
	// Drop trailing score fragments accidentally left in heading text.
	t = regexp.MustCompile(`(?i)\s*\d+(?:\.\d+)?\s*分?\s*$`).ReplaceAllString(t, "")
	return strings.TrimSpace(t)
}

func firstGroup(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func ParseEpisodeLinks(html, baseURL string) []EpisodeRef {
	var out []EpisodeRef
	for _, m := range rePlayLink.FindAllStringSubmatch(html, -1) {
		href := AbsoluteURL(baseURL, m[1])
		vodID, sid, nid, ok := ExtractPlayRef(href)
		if !ok {
			continue
		}
		label := cleanText(m[2])
		if label == "" {
			label = "第" + strconv.Itoa(nid) + "集"
		}
		out = append(out, EpisodeRef{
			VodID:     vodID,
			SID:       sid,
			NID:       nid,
			Label:     label,
			PlayURL:   href,
			Available: true,
		})
	}
	return out
}

func DedupEpisodes(eps []EpisodeRef) []EpisodeRef {
	seen := map[string]bool{}
	out := make([]EpisodeRef, 0, len(eps))
	for _, e := range eps {
		key := e.VodID + "|" + strconv.Itoa(e.SID) + "|" + strconv.Itoa(e.NID)
		if e.VodID == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}
	return out
}
