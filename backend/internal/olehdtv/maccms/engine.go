package maccms

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PageFetcher loads HTML for a URL (http or file:// / local path mapping).
type PageFetcher interface {
	Fetch(ctx context.Context, pageURL string) (body string, err error)
}

type HTTPFetcher struct {
	Client    *http.Client
	UserAgent string
	Delay     time.Duration
	Retries   int
	Log       *slog.Logger
	mu        sync.Mutex
	lastAt    time.Time
}

func NewHTTPFetcher(timeout time.Duration, delayMs, retries int, ua string) *HTTPFetcher {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if retries <= 0 {
		retries = 3
	}
	if ua == "" {
		ua = "VPlayerCatalogSync/1.0"
	}
	return &HTTPFetcher{
		Client:    &http.Client{Timeout: timeout},
		UserAgent: ua,
		Delay:     time.Duration(delayMs) * time.Millisecond,
		Retries:   retries,
		Log:       slog.Default().With("component", "maccms-http"),
	}
}

func (h *HTTPFetcher) Fetch(ctx context.Context, pageURL string) (string, error) {
	var last error
	for attempt := 0; attempt < h.Retries; attempt++ {
		h.throttle()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", h.UserAgent)
		resp, err := h.Client.Do(req)
		if err != nil {
			last = err
			sleepBackoff(ctx, attempt)
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if readErr != nil {
			last = readErr
			sleepBackoff(ctx, attempt)
			continue
		}
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			last = fmt.Errorf("http %d", resp.StatusCode)
			h.Log.Warn("transient http error", "url", pageURL, "status", resp.StatusCode, "attempt", attempt+1)
			sleepBackoff(ctx, attempt)
			continue
		}
		if resp.StatusCode >= 400 {
			return "", fmt.Errorf("http %d for %s", resp.StatusCode, pageURL)
		}
		return string(body), nil
	}
	return "", last
}

func (h *HTTPFetcher) throttle() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.Delay <= 0 {
		return
	}
	wait := h.Delay - time.Since(h.lastAt)
	if wait > 0 {
		time.Sleep(wait)
	}
	h.lastAt = time.Now()
}

func sleepBackoff(ctx context.Context, attempt int) {
	d := time.Duration(1<<attempt) * 200 * time.Millisecond
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// FileFetcher maps URLs onto a local dump directory for authorized offline imports/tests.
type FileFetcher struct {
	Root    string
	BaseURL string
}

func (f *FileFetcher) Fetch(ctx context.Context, pageURL string) (string, error) {
	_ = ctx
	rel := f.mapURL(pageURL)
	if rel == "" {
		return "", fmt.Errorf("no local mapping for %s", pageURL)
	}
	p := filepath.Join(f.Root, filepath.FromSlash(rel))
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (f *FileFetcher) mapURL(pageURL string) string {
	u := strings.TrimSpace(pageURL)
	base := strings.TrimRight(f.BaseURL, "/")
	if base != "" && strings.HasPrefix(u, base) {
		u = strings.TrimPrefix(u, base)
	}
	u = strings.TrimPrefix(u, "https://www.olehdtv.com")
	u = strings.TrimPrefix(u, "http://www.olehdtv.com")
	u = strings.TrimPrefix(u, "/")
	if u == "" || u == "index.php" || u == "index.html" {
		return "home.html"
	}
	// /index.php/vod/type/id/1.html -> type/1/page-1.html
	if tid, ok := ExtractTypeID(pageURL); ok {
		page := ExtractPageNumber(pageURL)
		return filepath.ToSlash(filepath.Join("type", tid, fmt.Sprintf("page-%d.html", page)))
	}
	if vod, sid, nid, ok := ExtractPlayRef(pageURL); ok {
		return filepath.ToSlash(filepath.Join("play", fmt.Sprintf("%s-s%d-n%d.html", vod, sid, nid)))
	}
	if vod, ok := ExtractVodID(pageURL); ok {
		return filepath.ToSlash(filepath.Join("detail", vod+".html"))
	}
	// direct relative path ending .html
	if strings.HasSuffix(u, ".html") {
		return u
	}
	return ""
}

// Engine walks MacCMS site structure via PageFetcher (file dump or authorized HTTP).
type Engine struct {
	Fetcher      PageFetcher
	BaseURL      string
	MaxPages     int
	Concurrency  int
	Log          *slog.Logger
	FetchPlay    bool // fetch play pages for player_aaaa when listed
}

type CrawlStats struct {
	Categories      int
	CategoryPages   int
	VideosFound     int
	EpisodesFound   int
	PagesFetched    int
	FetchErrors     int
	ParseErrors     int
}

func NewEngine(fetcher PageFetcher, baseURL string) *Engine {
	return &Engine{
		Fetcher:     fetcher,
		BaseURL:     strings.TrimRight(baseURL, "/"),
		MaxPages:    500,
		Concurrency: 2,
		Log:         slog.Default().With("component", "maccms-engine"),
		FetchPlay:   true,
	}
}

// Crawl builds a full catalog snapshot from home → types → pages → details.
func (e *Engine) Crawl(ctx context.Context) (categories []NavCategory, items []DetailMeta, stats CrawlStats, err error) {
	homeURL := e.BaseURL + "/"
	home, err := e.Fetcher.Fetch(ctx, homeURL)
	if err != nil {
		// try home.html mapping via bare home
		home, err = e.Fetcher.Fetch(ctx, e.BaseURL+"/index.html")
		if err != nil {
			return nil, nil, stats, fmt.Errorf("home: %w", err)
		}
	}
	stats.PagesFetched++
	categories = ParseNavigationCategories(home, e.BaseURL)
	stats.Categories = len(categories)

	visitedPages := map[string]bool{}
	vodSeen := map[string]CatalogItem{}

	for _, cat := range categories {
		if err := ctx.Err(); err != nil {
			return categories, nil, stats, err
		}
		start := cat.URL
		if start == "" {
			start = fmt.Sprintf("%s/index.php/vod/type/id/%s.html", e.BaseURL, cat.SourceID)
		}
		pageURL := start
		for page := 0; page < e.MaxPages; page++ {
			key := normalizeVisitKey(pageURL)
			if visitedPages[key] {
				e.Log.Warn("pagination loop detected", "url", pageURL)
				break
			}
			MarkVisited(visitedPages, pageURL)
			body, ferr := e.Fetcher.Fetch(ctx, pageURL)
			if ferr != nil {
				stats.FetchErrors++
				e.Log.Warn("category page fetch failed", "url", pageURL, "err", ferr)
				break
			}
			stats.PagesFetched++
			stats.CategoryPages++
			cards, pag := ParseCatalogPage(body, pageURL, cat.SourceID)
			for _, c := range cards {
				if _, ok := vodSeen[c.VodID]; !ok {
					vodSeen[c.VodID] = c
				}
			}
			if !pag.HasNext || pag.NextURL == "" || DetectPaginationLoop(visitedPages, pag.NextURL) {
				break
			}
			pageURL = pag.NextURL
		}
	}
	stats.VideosFound = len(vodSeen)

	// Detail pages
	type job struct{ card CatalogItem }
	jobs := make(chan job)
	var mu sync.Mutex
	var wg sync.WaitGroup
	workers := e.Concurrency
	if workers < 1 {
		workers = 1
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if ctx.Err() != nil {
					continue
				}
				durl := j.card.DetailURL
				if durl == "" {
					durl = fmt.Sprintf("%s/index.php/vod/detail/id/%s.html", e.BaseURL, j.card.VodID)
				}
				body, ferr := e.Fetcher.Fetch(ctx, durl)
				mu.Lock()
				stats.PagesFetched++
				mu.Unlock()
				if ferr != nil {
					mu.Lock()
					stats.FetchErrors++
					mu.Unlock()
					e.Log.Warn("detail fetch failed", "vod_id", j.card.VodID, "err", ferr)
					// keep minimal card info
					meta := DetailMeta{
						VodID: j.card.VodID, Title: j.card.Title, DetailURL: durl,
						PosterURL: j.card.PosterURL, TypeID: j.card.TypeID,
					}
					mu.Lock()
					items = append(items, meta)
					mu.Unlock()
					continue
				}
				meta := ParseDetailPage(body, durl)
				if meta.TypeID == "" {
					meta.TypeID = j.card.TypeID
				}
				if meta.Title == "" {
					meta.Title = j.card.Title
				}
				if meta.PosterURL == "" {
					meta.PosterURL = j.card.PosterURL
				}
				if e.FetchPlay {
					meta.Episodes = e.enrichEpisodes(ctx, meta.Episodes, &stats)
				}
				mu.Lock()
				stats.EpisodesFound += len(meta.Episodes)
				items = append(items, meta)
				mu.Unlock()
			}
		}()
	}
	for _, c := range vodSeen {
		select {
		case <-ctx.Done():
		case jobs <- job{card: c}:
		}
	}
	close(jobs)
	wg.Wait()
	return categories, items, stats, nil
}

func (e *Engine) enrichEpisodes(ctx context.Context, eps []EpisodeRef, stats *CrawlStats) []EpisodeRef {
	out := make([]EpisodeRef, len(eps))
	copy(out, eps)
	for i := range out {
		if out[i].PlayURL == "" {
			continue
		}
		body, err := e.Fetcher.Fetch(ctx, out[i].PlayURL)
		stats.PagesFetched++
		if err != nil {
			stats.FetchErrors++
			out[i].Available = false
			continue
		}
		if p, ok := ParsePlayerAAAA(body); ok {
			ApplyPlayerToEpisode(&out[i], p)
		} else {
			// play page without player config — keep reference, mark stream unavailable
			out[i].Available = false
		}
	}
	return out
}
