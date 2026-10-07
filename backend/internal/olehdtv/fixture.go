package olehdtv

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type fixtureFile struct {
	Categories []struct {
		SourceID string `json:"source_id"`
		Name     string `json:"name"`
		Slug     string `json:"slug"`
		Sort     int    `json:"sort"`
	} `json:"categories"`
	Videos []struct {
		SourceID         string  `json:"source_id"`
		SourceCategoryID string  `json:"source_category_id"`
		Title            string  `json:"title"`
		Description      string  `json:"description"`
		Year             string  `json:"year"`
		Area             string  `json:"area"`
		Director         string  `json:"director"`
		Actors           string  `json:"actors"`
		Rating           float64 `json:"rating"`
		PosterURL        string  `json:"poster_url"`
		ViewCount        int64   `json:"view_count"`
		DurationSec      int     `json:"duration_sec"`
		SourceUpdatedAt  string  `json:"source_updated_at"`
		IsActive         bool    `json:"is_active"`
		Episodes         []struct {
			SID            int    `json:"sid"`
			NID            int    `json:"nid"`
			Title          string `json:"title"`
			PlaybackSource string `json:"playback_source"`
			PlaybackURL    string `json:"playback_url"`
		} `json:"episodes"`
	} `json:"videos"`
}

type FixtureAdapter struct {
	path string
	data *CatalogSnapshot
}

func NewFixtureAdapter(path string) (*FixtureAdapter, error) {
	if path == "" {
		path = "testdata/olehdtv_sample.json"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("olehdtv fixture: %w", err)
	}
	snap, err := ParseFixtureJSON(raw)
	if err != nil {
		return nil, err
	}
	return &FixtureAdapter{path: path, data: snap}, nil
}

func ParseFixtureJSON(raw []byte) (*CatalogSnapshot, error) {
	var f fixtureFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("olehdtv fixture json: %w", err)
	}
	out := &CatalogSnapshot{
		Categories: make([]SourceCategory, 0, len(f.Categories)),
		Videos:     make([]SourceVideo, 0, len(f.Videos)),
	}
	for _, c := range f.Categories {
		out.Categories = append(out.Categories, SourceCategory{
			SourceID: c.SourceID,
			Name:     c.Name,
			Slug:     c.Slug,
			Sort:     c.Sort,
		})
	}
	for _, v := range f.Videos {
		updated := time.Time{}
		if v.SourceUpdatedAt != "" {
			if t, err := time.Parse(time.RFC3339, v.SourceUpdatedAt); err == nil {
				updated = t
			}
		}
		eps := make([]SourceEpisode, 0, len(v.Episodes))
		for _, e := range v.Episodes {
			eps = append(eps, SourceEpisode{
				SID:            e.SID,
				NID:            e.NID,
				Title:          e.Title,
				PlaybackSource: e.PlaybackSource,
				PlaybackURL:    e.PlaybackURL,
			})
		}
		out.Videos = append(out.Videos, SourceVideo{
			SourceID:         v.SourceID,
			SourceCategoryID: v.SourceCategoryID,
			Title:            v.Title,
			Description:      v.Description,
			Year:             v.Year,
			Area:             v.Area,
			Director:         v.Director,
			Actors:           v.Actors,
			Rating:           v.Rating,
			PosterURL:        v.PosterURL,
			ViewCount:        v.ViewCount,
			DurationSec:      v.DurationSec,
			SourceUpdatedAt:  updated,
			IsActive:         v.IsActive,
			Episodes:         eps,
		})
	}
	return out, nil
}

func (a *FixtureAdapter) Name() string { return "fixture:" + a.path }

func (a *FixtureAdapter) ListCategories(ctx context.Context) ([]SourceCategory, error) {
	_ = ctx
	return a.data.Categories, nil
}

func (a *FixtureAdapter) ListVideos(ctx context.Context) ([]SourceVideo, error) {
	_ = ctx
	return a.data.Videos, nil
}

// NewExportFileAdapter loads the same JSON shape from an authorized export dump.
func NewExportFileAdapter(path string) (*FixtureAdapter, error) {
	if path == "" {
		return nil, fmt.Errorf("olehdtv export_file: OLEHDTV_EXPORT_PATH is required")
	}
	return NewFixtureAdapter(path)
}
