package olehdtv

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
)

// HTMLDumpAdapter imports a MacCMS catalog from a local HTML dump directory
// (authorized offline export / test fixtures). Does not crawl live websites.
type HTMLDumpAdapter struct {
	Root    string
	BaseURL string
	engine  *maccms.Engine
	cats    []SourceCategory
	videos  []SourceVideo
	stats   maccms.CrawlStats
	loaded  bool
}

func NewHTMLDumpAdapter(root, baseURL string) (*HTMLDumpAdapter, error) {
	if root == "" {
		root = filepath.Join("testdata", "maccms_html")
	}
	if baseURL == "" {
		baseURL = "https://maccms.local"
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	fetcher, err := maccms.NewSiteOneFetcher(abs, baseURL)
	if err != nil {
		return nil, err
	}
	eng := maccms.NewEngine(fetcher, baseURL)
	eng.FetchPlay = true
	return &HTMLDumpAdapter{Root: abs, BaseURL: baseURL, engine: eng}, nil
}

func (a *HTMLDumpAdapter) Name() string { return "html_dump:" + a.Root }

func (a *HTMLDumpAdapter) ensure(ctx context.Context) error {
	if a.loaded {
		return nil
	}
	cats, details, stats, err := a.engine.Crawl(ctx)
	if err != nil {
		return err
	}
	a.stats = stats
	a.cats = make([]SourceCategory, 0, len(cats))
	for _, c := range cats {
		slug := maccms.Slugify(c.Name)
		if slug == "cat" {
			slug = "type-" + c.SourceID
		}
		// map common Chinese names to stable slugs matching VPlayer seeds
		switch c.Name {
		case "电影":
			slug = "movie"
		case "连续剧", "剧集":
			slug = "tv"
		case "动漫":
			slug = "anime"
		case "综艺":
			slug = "variety"
		}
		a.cats = append(a.cats, SourceCategory{
			SourceID: c.SourceID,
			Name:     c.Name,
			Slug:     slug,
			Sort:     c.Sort,
		})
	}
	a.videos = make([]SourceVideo, 0, len(details))
	for _, d := range details {
		eps := make([]SourceEpisode, 0, len(d.Episodes))
		for _, e := range d.Episodes {
			status := "ok"
			url := e.StreamURL
			if !e.Available || url == "" {
				status = "unavailable"
				url = ""
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
			poster = strings.TrimRight(a.BaseURL, "/") + poster
		}
		a.videos = append(a.videos, SourceVideo{
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
		})
	}
	a.loaded = true
	return nil
}

func (a *HTMLDumpAdapter) Stats() maccms.CrawlStats { return a.stats }

func (a *HTMLDumpAdapter) ListCategories(ctx context.Context) ([]SourceCategory, error) {
	if err := a.ensure(ctx); err != nil {
		return nil, err
	}
	return a.cats, nil
}

func (a *HTMLDumpAdapter) ListVideos(ctx context.Context) ([]SourceVideo, error) {
	if err := a.ensure(ctx); err != nil {
		return nil, err
	}
	if len(a.videos) == 0 {
		return nil, fmt.Errorf("html_dump: no videos discovered under %s", a.Root)
	}
	return a.videos, nil
}
