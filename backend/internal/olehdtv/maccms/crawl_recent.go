package maccms

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// Hard caps for maccms_recent (first live bounded version).
const (
	RecentMaxLimit            = 20
	RecentMinDelayMs          = 1000
	RecentMaxConcurrency      = 2
	RecentMaxPlayPerVideo     = 3
	RecentMaxPlayTotal        = 20
	RecentListingPagesPerCat  = 1 // page 1 only — never /page/2+
)

// RecentCategoryAllowlist is the homepage navigation set (excludes 体育直播/短剧).
var RecentCategoryAllowlist = map[string]struct{}{
	"电影": {}, "连续剧": {}, "剧集": {}, "综艺": {}, "动漫": {},
	"午夜影院": {}, "VIP蓝光影院": {},
}

// RequestBudget reports HTTP usage for a bounded recent crawl.
type RequestBudget struct {
	ListingRequests   int  `json:"listing_requests"`
	DetailRequests    int  `json:"detail_requests"`
	PlayRequests      int  `json:"play_requests"`
	TotalHTTPRequests int  `json:"total_http_requests"`
	EnrichmentCapped  bool `json:"enrichment_capped"`
	PlaySkipped       int  `json:"play_skipped"`
}

// RecentCrawlResult is the output of Engine.CrawlRecent.
type RecentCrawlResult struct {
	Categories       []NavCategory
	Items            []DetailMeta
	Budget           RequestBudget
	OrderingNote     string
	EnrichmentCapped bool
	Selected         int
	CandidatesSeen   int
}

// ValidateRecentLimit enforces 1..20 for maccms_recent.
func ValidateRecentLimit(limit int) error {
	if limit <= 0 {
		return fmt.Errorf("maccms_recent: limit must be > 0")
	}
	if limit > RecentMaxLimit {
		return fmt.Errorf("maccms_recent: limit %d exceeds hard max %d", limit, RecentMaxLimit)
	}
	return nil
}

// ClampRecentHTTPSettings forces safe delay/concurrency for live recent discovery.
func ClampRecentHTTPSettings(delayMs, concurrency, retries int) (delayOut, concOut, retriesOut int) {
	delayOut = delayMs
	if delayOut < RecentMinDelayMs {
		delayOut = RecentMinDelayMs
	}
	concOut = concurrency
	if concOut <= 0 || concOut > RecentMaxConcurrency {
		concOut = RecentMaxConcurrency
	}
	retriesOut = retries
	if retriesOut <= 0 {
		retriesOut = 3
	}
	return delayOut, concOut, retriesOut
}

func IsRecentAllowedCategory(name string) bool {
	_, ok := RecentCategoryAllowlist[strings.TrimSpace(name)]
	return ok
}

func FilterRecentCategories(cats []NavCategory) []NavCategory {
	out := make([]NavCategory, 0, len(cats))
	seen := map[string]bool{}
	for _, c := range cats {
		if !IsRecentAllowedCategory(c.Name) {
			continue
		}
		// Prefer 连续剧 over 剧集 if both appear.
		if c.Name == "剧集" {
			if seen["连续剧"] {
				continue
			}
		}
		if seen[c.Name] || seen[c.SourceID] {
			continue
		}
		seen[c.Name] = true
		seen[c.SourceID] = true
		out = append(out, c)
	}
	return out
}

// CrawlRecent discovers up to `limit` videos using page-1 listings only.
// It MUST NOT call Crawl() and never requests /page/2 or beyond.
func (e *Engine) CrawlRecent(ctx context.Context, limit int) (RecentCrawlResult, error) {
	var out RecentCrawlResult
	out.OrderingNote = "best_effort_listing_order: page-1 cards interleaved across allowed categories; no reliable listing timestamp"
	if err := ValidateRecentLimit(limit); err != nil {
		return out, err
	}
	if e.Fetcher == nil || e.BaseURL == "" {
		return out, fmt.Errorf("maccms recent: fetcher and base URL required")
	}
	if e.Log == nil {
		e.Log = slog.Default().With("component", "maccms-recent")
	}
	conc := e.Concurrency
	if conc <= 0 || conc > RecentMaxConcurrency {
		conc = RecentMaxConcurrency
	}

	var budgetMu sync.Mutex
	budget := &out.Budget
	fetch := func(pageURL string, kind string) (string, error) {
		body, err := e.Fetcher.Fetch(ctx, pageURL)
		budgetMu.Lock()
		budget.TotalHTTPRequests++
		switch kind {
		case "listing":
			budget.ListingRequests++
		case "detail":
			budget.DetailRequests++
		case "play":
			budget.PlayRequests++
		}
		budgetMu.Unlock()
		return body, err
	}

	homeURL := strings.TrimRight(e.BaseURL, "/") + "/"
	home, err := fetch(homeURL, "listing")
	if err != nil {
		home, err = fetch(strings.TrimRight(e.BaseURL, "/")+"/index.html", "listing")
		if err != nil {
			return out, fmt.Errorf("home: %w", err)
		}
	}
	allCats := ParseNavigationCategories(home, e.BaseURL)
	cats := FilterRecentCategories(allCats)
	out.Categories = cats
	if len(cats) == 0 {
		return out, fmt.Errorf("maccms recent: no allowed categories found in navigation")
	}

	// Page-1 listings only (RecentListingPagesPerCat == 1).
	perCat := make([][]CatalogItem, len(cats))
	for i, cat := range cats {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		start := cat.URL
		if start == "" {
			start = fmt.Sprintf("%s/index.php/vod/type/id/%s.html", strings.TrimRight(e.BaseURL, "/"), cat.SourceID)
		}
		// Refuse any URL that already points at page>=2.
		if ExtractPageNumber(start) > 1 || strings.Contains(strings.ToLower(start), "/page/2") {
			return out, fmt.Errorf("maccms recent: refusing non-page-1 listing URL %s", start)
		}
		body, ferr := fetch(start, "listing")
		if ferr != nil {
			e.Log.Warn("listing fetch failed", "category", cat.Name, "err", ferr)
			continue
		}
		cards, _ := ParseCatalogPage(body, start, cat.SourceID)
		// Defensive: never follow pagination — ignore NextURL entirely.
		_ = RecentListingPagesPerCat
		perCat[i] = cards
		out.CandidatesSeen += len(cards)
	}

	selected := selectInterleaved(perCat, limit)
	out.Selected = len(selected)

	// Details only for selected candidates (bounded concurrency).
	type job struct{ card CatalogItem }
	jobs := make(chan job)
	var mu sync.Mutex
	var wg sync.WaitGroup
	gate := &playBudgetGate{remain: RecentMaxPlayTotal}
	for w := 0; w < conc; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if ctx.Err() != nil {
					continue
				}
				durl := j.card.DetailURL
				if durl == "" {
					durl = fmt.Sprintf("%s/index.php/vod/detail/id/%s.html", strings.TrimRight(e.BaseURL, "/"), j.card.VodID)
				}
				body, ferr := fetch(durl, "detail")
				if ferr != nil {
					e.Log.Warn("detail fetch failed", "vod_id", j.card.VodID, "err", ferr)
					meta := DetailMeta{
						VodID: j.card.VodID, Title: j.card.Title, DetailURL: durl,
						PosterURL: j.card.PosterURL, TypeID: j.card.TypeID,
					}
					mu.Lock()
					out.Items = append(out.Items, meta)
					mu.Unlock()
					continue
				}
				meta := MergeDetailWithCatalog(j.card, ParseDetailPage(body, durl))
				capped, skipped, _ := e.enrichEpisodesBounded(ctx, meta.Episodes, gate, fetch)
				meta.Episodes = capped
				mu.Lock()
				if skipped > 0 {
					budgetMu.Lock()
					budget.PlaySkipped += skipped
					budget.EnrichmentCapped = true
					budgetMu.Unlock()
					out.EnrichmentCapped = true
				}
				out.Items = append(out.Items, meta)
				mu.Unlock()
			}
		}()
	}
	for _, c := range selected {
		select {
		case <-ctx.Done():
		case jobs <- job{card: c}:
		}
	}
	close(jobs)
	wg.Wait()

	out.Budget = *budget
	out.Budget.EnrichmentCapped = out.Budget.EnrichmentCapped || out.EnrichmentCapped
	out.EnrichmentCapped = out.Budget.EnrichmentCapped
	return out, nil
}

// selectInterleaved round-robins page-1 cards across categories until limit unique vod IDs.
func selectInterleaved(perCat [][]CatalogItem, limit int) []CatalogItem {
	seen := map[string]bool{}
	out := make([]CatalogItem, 0, limit)
	idx := make([]int, len(perCat))
	for len(out) < limit {
		progress := false
		for i := range perCat {
			for idx[i] < len(perCat[i]) {
				c := perCat[i][idx[i]]
				idx[i]++
				if c.VodID == "" || seen[c.VodID] {
					continue
				}
				seen[c.VodID] = true
				out = append(out, c)
				progress = true
				break
			}
			if len(out) >= limit {
				break
			}
		}
		if !progress {
			break
		}
	}
	return out
}

// playBudgetGate atomically reserves global play-page slots across workers.
type playBudgetGate struct {
	mu     sync.Mutex
	remain int
	used   int
}

func (g *playBudgetGate) tryTake() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.remain <= 0 {
		return false
	}
	g.remain--
	g.used++
	return true
}

func (e *Engine) enrichEpisodesBounded(
	ctx context.Context,
	eps []EpisodeRef,
	gate *playBudgetGate,
	fetch func(url, kind string) (string, error),
) (out []EpisodeRef, skipped, used int) {
	_ = ctx
	out = make([]EpisodeRef, len(eps))
	copy(out, eps)
	perVideo := 0
	for i := range out {
		if out[i].PlayURL == "" {
			continue
		}
		if perVideo >= RecentMaxPlayPerVideo {
			skipped++
			out[i].EnrichmentSkipped = true
			continue
		}
		// Atomic global reservation — prevents concurrent workers from exceeding RecentMaxPlayTotal.
		if !gate.tryTake() {
			skipped++
			out[i].EnrichmentSkipped = true
			continue
		}
		body, err := fetch(out[i].PlayURL, "play")
		used++
		perVideo++
		if err != nil {
			out[i].Available = false
			continue
		}
		if p, ok := ParsePlayerAAAA(body); ok {
			ApplyPlayerToEpisode(&out[i], p)
		} else {
			out[i].Available = false
		}
	}
	return out, skipped, used
}
