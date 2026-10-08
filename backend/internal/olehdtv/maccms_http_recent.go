package olehdtv

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
)

// MacCMSHTTPRecent is a bounded live RecentSource (OLEHDTV_SOURCE_MODE=maccms_recent).
// It uses Engine.CrawlRecent only — never Engine.Crawl().
type MacCMSHTTPRecent struct {
	BaseURL     string
	engine      *maccms.Engine
	lastBudget  maccms.RequestBudget
	lastNote    string
	catsCache   []SourceCategory
}

// NewMacCMSHTTPRecent builds a safe HTTP recent adapter. Delay/concurrency are clamped.
func NewMacCMSHTTPRecent(baseURL string, delayMs, timeoutSec, concurrency, retries int) (*MacCMSHTTPRecent, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("maccms_recent: OLEHDTV_PUBLIC_BASE_URL is required")
	}
	delayMs, concurrency, retries = maccms.ClampRecentHTTPSettings(delayMs, concurrency, retries)
	timeout := time.Duration(timeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	fetcher := maccms.NewHTTPFetcher(timeout, delayMs, retries, "VPlayerCatalogSync/1.0-recent")
	eng := maccms.NewEngine(fetcher, baseURL)
	eng.Concurrency = concurrency
	eng.FetchPlay = false // enrichment handled inside CrawlRecent with hard caps
	eng.MaxPages = 1      // defensive; CrawlRecent ignores pagination anyway
	return &MacCMSHTTPRecent{BaseURL: baseURL, engine: eng}, nil
}

func (a *MacCMSHTTPRecent) Name() string { return "maccms_recent:" + a.BaseURL }

func (a *MacCMSHTTPRecent) ListCategories(ctx context.Context) ([]SourceCategory, error) {
	if len(a.catsCache) > 0 {
		return a.catsCache, nil
	}
	// Lightweight: home nav only (one listing request). Full recent run will refetch home.
	delayMs, _, retries := maccms.ClampRecentHTTPSettings(1000, 2, 3)
	f := maccms.NewHTTPFetcher(20*time.Second, delayMs, retries, "VPlayerCatalogSync/1.0-recent")
	body, err := f.Fetch(ctx, a.BaseURL+"/")
	if err != nil {
		body, err = f.Fetch(ctx, a.BaseURL+"/index.html")
		if err != nil {
			return nil, err
		}
	}
	nav := maccms.FilterRecentCategories(maccms.ParseNavigationCategories(body, a.BaseURL))
	out := make([]SourceCategory, 0, len(nav))
	for _, c := range nav {
		slug := maccms.Slugify(c.Name)
		switch c.Name {
		case "电影":
			slug = "movie"
		case "连续剧", "剧集":
			slug = "tv"
		case "综艺":
			slug = "variety"
		case "动漫":
			slug = "anime"
		case "午夜影院":
			slug = "midnight"
		case "VIP蓝光影院":
			slug = "vip-bluray"
		}
		out = append(out, SourceCategory{
			SourceID: c.SourceID,
			Name:     c.Name,
			Slug:     slug,
			Sort:     c.Sort,
		})
	}
	a.catsCache = out
	return out, nil
}

func (a *MacCMSHTTPRecent) ListRecentVideos(ctx context.Context, limit int) ([]SourceVideo, RecentMeta, error) {
	if err := maccms.ValidateRecentLimit(limit); err != nil {
		return nil, RecentMeta{}, err
	}
	res, err := a.engine.CrawlRecent(ctx, limit)
	if err != nil {
		return nil, RecentMeta{}, err
	}
	a.lastBudget = res.Budget
	a.lastNote = res.OrderingNote
	a.catsCache = nil
	cats := make([]SourceCategory, 0, len(res.Categories))
	for _, c := range res.Categories {
		slug := maccms.Slugify(c.Name)
		switch c.Name {
		case "电影":
			slug = "movie"
		case "连续剧", "剧集":
			slug = "tv"
		case "综艺":
			slug = "variety"
		case "动漫":
			slug = "anime"
		case "午夜影院":
			slug = "midnight"
		case "VIP蓝光影院":
			slug = "vip-bluray"
		}
		cats = append(cats, SourceCategory{SourceID: c.SourceID, Name: c.Name, Slug: slug, Sort: c.Sort})
	}
	a.catsCache = cats

	videos := make([]SourceVideo, 0, len(res.Items))
	for _, d := range res.Items {
		eps := make([]SourceEpisode, 0, len(d.Episodes))
		for _, e := range d.Episodes {
			url := strings.TrimSpace(e.StreamURL)
			status := "ok"
			switch {
			case url != "":
				status = "ok"
			case e.EnrichmentSkipped:
				// Bounded run intentionally omitted play-page fetch.
				status = "pending_enrichment"
			case !e.Available:
				status = "unavailable"
			default:
				// Listed on detail page without a resolved stream yet.
				status = "pending_enrichment"
			}
			eps = append(eps, SourceEpisode{
				SID:            e.SID,
				NID:            e.NID,
				Title:          e.Label,
				PlaybackSource: e.From,
				PlaybackURL:    url,
				PlayPageURL:    e.PlayURL,
				PlaybackStatus: status,
			})
		}
		poster := d.PosterURL
		if poster != "" && strings.HasPrefix(poster, "/") {
			poster = a.BaseURL + poster
		}
		videos = append(videos, SourceVideo{
			SourceID:         d.VodID,
			SourceCategoryID: d.TypeID,
			Title:            d.Title,
			Description:      d.Description,
			Year:             d.Year,
			Area:             d.Area,
			Director:         d.Director,
			Actors:           d.Actors,
			Rating:           d.Rating,
			PosterURL:        poster,
			IsActive:         true,
			Episodes:         eps,
			DetailURL:        d.DetailURL,
			StatusText:       d.Status,
			// No invented timestamps — detail parser has no reliable update time.
		})
	}

	meta := RecentMeta{
		ReliableWatermark: false,
		Note:              res.OrderingNote,
		ListingRequests:   res.Budget.ListingRequests,
		DetailRequests:    res.Budget.DetailRequests,
		PlayRequests:      res.Budget.PlayRequests,
		TotalHTTPRequests: res.Budget.TotalHTTPRequests,
		EnrichmentCapped:  res.Budget.EnrichmentCapped,
		PlaySkipped:       res.Budget.PlaySkipped,
	}
	if len(videos) > 0 {
		meta.MaxSourceID = videos[0].SourceID
		for _, v := range videos {
			if sourceIDGreater(v.SourceID, meta.MaxSourceID) {
				meta.MaxSourceID = v.SourceID
			}
		}
	}
	return videos, meta, nil
}

// LastBudget returns the HTTP budget from the most recent ListRecentVideos call.
func (a *MacCMSHTTPRecent) LastBudget() maccms.RequestBudget { return a.lastBudget }
