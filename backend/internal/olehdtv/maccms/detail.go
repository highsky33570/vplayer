package maccms

import (
	"regexp"
	"strconv"
)

var (
	reH1Title     = regexp.MustCompile(`(?is)<h1[^>]*>(.*?)</h1>`)
	reMetaDesc    = regexp.MustCompile(`(?is)<meta[^>]+name=["']description["'][^>]+content=["']([^"']*)["']`)
	reLabeled     = regexp.MustCompile(`(?is)(?:主演|演员|actors?)\s*[:：]</?\w*>?\s*([^<]{1,200})|(?:导演|director)\s*[:：]</?\w*>?\s*([^<]{1,120})|(?:地区|area|region)\s*[:：]</?\w*>?\s*([^<]{1,80})|(?:年份|year)\s*[:：]</?\w*>?\s*([^<]{1,20})|(?:状态|更新|remarks?)\s*[:：]</?\w*>?\s*([^<]{1,80})`)
	reYearLoose   = regexp.MustCompile(`(?i)(?:年份|year)[^0-9]{0,20}(19\d{2}|20\d{2})`)
	reAreaLoose   = regexp.MustCompile(`(?is)(?:地区|area)[^<\n]{0,10}[:：]\s*([^<\n|]{1,40})`)
	reDirector    = regexp.MustCompile(`(?is)(?:导演|director)[^<\n]{0,10}[:：]\s*([^<\n|]{1,80})`)
	reActors      = regexp.MustCompile(`(?is)(?:主演|演员|actors?)[^<\n]{0,10}[:：]\s*([^<\n]{1,200})`)
	reStatusLoose = regexp.MustCompile(`(?is)(?:状态|更新)[^<\n]{0,10}[:：]\s*([^<\n|]{1,80})`)
	reContent     = regexp.MustCompile(`(?is)(?:class=["'][^"']*(?:content|desc|detail|sketch|txt)[^"']*["'][^>]*>)([\s\S]+?)</`)
	rePlayLink    = regexp.MustCompile(`(?is)<a[^>]+href=["']([^"']*vod/play/id/\d+/sid/\d+/nid/\d+[^"']*)["'][^>]*>(.*?)</a>`)
	reScore       = regexp.MustCompile(`(?i)(?:评分|score)[^0-9]{0,12}(\d+(?:\.\d+)?)`)
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
	VodID     string
	SID       int
	NID       int
	Label     string
	PlayURL   string
	From      string
	StreamURL string // from player_aaaa when present
	Available bool
}

// ParseDetailPage extracts MacCMS detail metadata and episode play links.
func ParseDetailPage(html, detailURL string) DetailMeta {
	vodID, _ := ExtractVodID(detailURL)
	meta := DetailMeta{
		VodID:     vodID,
		DetailURL: detailURL,
		PosterURL: BestPoster(html, detailURL),
	}
	if m := reH1Title.FindStringSubmatch(html); len(m) == 2 {
		meta.Title = cleanText(m[1])
	}
	if meta.Title == "" {
		if m := reTitleAttr.FindStringSubmatch(html); len(m) == 2 {
			meta.Title = cleanText(m[1])
		}
	}
	if m := reMetaDesc.FindStringSubmatch(html); len(m) == 2 {
		meta.Description = cleanText(m[1])
	}
	if meta.Description == "" {
		if m := reContent.FindStringSubmatch(html); len(m) == 2 {
			meta.Description = cleanText(m[1])
		}
	}
	if m := reYearLoose.FindStringSubmatch(html); len(m) == 2 {
		meta.Year = m[1]
	}
	if m := reAreaLoose.FindStringSubmatch(html); len(m) == 2 {
		meta.Area = cleanText(m[1])
	}
	if m := reDirector.FindStringSubmatch(html); len(m) == 2 {
		meta.Director = cleanText(m[1])
	}
	if m := reActors.FindStringSubmatch(html); len(m) == 2 {
		meta.Actors = cleanText(m[1])
	}
	if m := reStatusLoose.FindStringSubmatch(html); len(m) == 2 {
		meta.Status = cleanText(m[1])
	}
	if m := reScore.FindStringSubmatch(html); len(m) == 2 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			meta.Rating = v
		}
	}
	if tid, ok := ExtractTypeID(detailURL); ok {
		meta.TypeID = tid
	}
	// type id from breadcrumb links
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
