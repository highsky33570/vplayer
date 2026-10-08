package catalogsync

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tycdn/vplayer/internal/media"
	"github.com/tycdn/vplayer/internal/olehdtv"
	"github.com/tycdn/vplayer/internal/store"
)

// ImportSummary is a richer report for the JSONL importer.
type ImportSummary struct {
	DryRun bool `json:"dry_run"`

	CategoriesCreated int `json:"categories_created"`
	CategoriesUpdated int `json:"categories_updated"`
	CategoriesSkipped int `json:"categories_skipped"`
	CategoriesFailed  int `json:"categories_failed"`

	VideosCreated int `json:"videos_created"`
	VideosUpdated int `json:"videos_updated"`
	VideosSkipped int `json:"videos_skipped"`
	VideosFailed  int `json:"videos_failed"`

	EpisodesCreated     int `json:"episodes_created"`
	EpisodesUpdated     int `json:"episodes_updated"`
	EpisodesUnavailable int `json:"episodes_unavailable"`
	EpisodesSkipped     int `json:"episodes_skipped"`
	EpisodesFailed      int `json:"episodes_failed"`

	PosterUploaded int `json:"poster_uploaded"`
	PosterReused   int `json:"poster_reused"`
	PosterFailed   int `json:"poster_failed"`
	PosterSkipped  int `json:"poster_skipped"`

	Validation *olehdtv.JSONLValidation `json:"validation,omitempty"`
	Error      string                   `json:"error,omitempty"`
}

type JSONLImporter struct {
	Store           *store.MySQL
	Posters         *media.PosterSyncer
	Log             *slog.Logger
	ProgressEvery   int
	FetchPosters    bool
	DeactivateMissing bool
}

func NewJSONLImporter(st *store.MySQL, posters *media.PosterSyncer) *JSONLImporter {
	return &JSONLImporter{
		Store:         st,
		Posters:       posters,
		Log:           slog.Default().With("component", "olehdtv-jsonl-import"),
		ProgressEvery: 500,
	}
}

// RunImport imports from a local JSONL root. dryRun validates only (no DB/R2 writes).
func (imp *JSONLImporter) RunImport(ctx context.Context, root string, dryRun bool) (ImportSummary, error) {
	sum := ImportSummary{DryRun: dryRun}

	val, err := olehdtv.ValidateJSONLImport(root)
	if err != nil {
		sum.Error = err.Error()
		return sum, err
	}
	sum.Validation = val
	if !val.OK {
		sum.Error = "validation failed"
		return sum, fmt.Errorf("validation failed: duplicates=%d/%d parse_errors=%d missing_category=%d missing_title=%d",
			len(val.DuplicateVodIDs), len(val.DuplicateEpisodes), len(val.ParseErrors), val.MissingCategory, val.MissingTitle)
	}

	if dryRun {
		imp.Log.Info("dry-run ok",
			"categories", val.CategoryCount,
			"videos", val.VideoCount,
			"episodes", val.EpisodeCount,
			"episodes_with_stream", val.EpisodesWithStream,
			"episodes_without_stream", val.EpisodesNoStream,
			"orphan_episodes", val.OrphanEpisodes,
		)
		// Estimate poster plan without fetching.
		sum.PosterSkipped = val.VideoCount
		sum.EpisodesUnavailable = val.EpisodesNoStream
		return sum, nil
	}

	if imp.Store == nil {
		return sum, fmt.Errorf("mysql store required for real import")
	}

	cats, err := olehdtv.LoadJSONLCategories(root)
	if err != nil {
		sum.Error = err.Error()
		return sum, err
	}

	catMap := map[string]uint64{}
	for _, c := range cats {
		id, created, updated, err := imp.Store.UpsertCategoryResult(store.CategoryUpsert{
			SourceSystem: olehdtv.SourceSystem,
			SourceID:     c.SourceID,
			Name:         c.Name,
			Slug:         NormalizeSlug(c.Slug),
			Sort:         c.Sort,
			IsActive:     true,
		})
		if err != nil {
			imp.Log.Error("category upsert failed", "source_id", c.SourceID, "err", err)
			sum.CategoriesFailed++
			continue
		}
		catMap[c.SourceID] = id
		if created {
			sum.CategoriesCreated++
		} else if updated {
			sum.CategoriesUpdated++
		} else {
			sum.CategoriesSkipped++
		}
	}

	epsByVod, _, err := olehdtv.LoadEpisodesByVod(root)
	if err != nil {
		sum.Error = err.Error()
		return sum, err
	}

	keep := make([]string, 0, val.VideoCount)
	err = olehdtv.IterVideosJSONL(root, epsByVod, func(v olehdtv.SourceVideo, idx int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if imp.ProgressEvery > 0 && idx%imp.ProgressEvery == 0 {
			imp.Log.Info(fmt.Sprintf("Videos: %d / %d", idx, val.VideoCount))
		}
		keep = append(keep, v.SourceID)

		catID := catMap[v.SourceCategoryID]
		if catID == 0 {
			if c, e := imp.Store.GetCategoryBySource(olehdtv.SourceSystem, v.SourceCategoryID); e == nil && c != nil {
				catID = c.ID
			}
		}

		var srcUpdated *time.Time
		if !v.SourceUpdatedAt.IsZero() {
			t := v.SourceUpdatedAt
			srcUpdated = &t
		}

		playSrc, sid, nid, playURL := primaryPlaybackPreferStream(v)

		existing, _ := imp.Store.GetVideoBySource(olehdtv.SourceSystem, v.SourceID)
		coverKey := ""
		if existing != nil {
			coverKey = existing.CoverR2Key
			if coverKey == "" {
				coverKey = existing.CoverKey
			}
		}

		id, created, updated, err := imp.Store.UpsertVideoResult(store.VideoUpsert{
			SourceSystem:     olehdtv.SourceSystem,
			SourceID:         v.SourceID,
			CategoryID:       catID,
			SourceCategoryID: v.SourceCategoryID,
			Title:            v.Title,
			Description:      v.Description,
			Year:             v.Year,
			Area:             v.Area,
			Director:         v.Director,
			Actors:           v.Actors,
			Rating:           v.Rating,
			PosterSourceURL:  v.PosterURL,
			CoverR2Key:       coverKey,
			CoverKey:         coverKey,
			DurationSec:      v.DurationSec,
			ViewCount:        v.ViewCount,
			PlaybackSource:   playSrc,
			PlaySID:          sid,
			PlayNID:          nid,
			PlaybackURL:      playURL,
			SourceUpdatedAt:  srcUpdated,
			IsActive:         v.IsActive,
			Status:           statusForJSONL(v),
		})
		if err != nil {
			imp.Log.Error("video upsert failed", "source_id", v.SourceID, "err", err)
			sum.VideosFailed++
			return nil
		}
		if created {
			sum.VideosCreated++
		} else if updated {
			sum.VideosUpdated++
		} else {
			sum.VideosSkipped++
		}
		_ = imp.Store.TouchVideoSeen(id)

		for _, ep := range v.Episodes {
			if ep.PlaybackStatus == "unavailable" || ep.PlaybackURL == "" {
				sum.EpisodesUnavailable++
			}
			before, _ := imp.Store.GetEpisodeBySource(olehdtv.SourceSystem, v.SourceID, ep.SID, ep.NID)
			if err := imp.Store.UpsertEpisode(store.EpisodeUpsert{
				VideoID:        id,
				SourceSystem:   olehdtv.SourceSystem,
				SourceVideoID:  v.SourceID,
				SID:            ep.SID,
				NID:            ep.NID,
				Title:          ep.Title,
				PlaybackSource: ep.PlaybackSource,
				PlaybackURL:    ep.PlaybackURL,
				PlayPageURL:    ep.PlayPageURL,
				PlaybackStatus: ep.PlaybackStatus,
				IsActive:       ep.PlaybackURL != "",
			}); err != nil {
				imp.Log.Error("episode upsert failed", "source_id", v.SourceID, "sid", ep.SID, "nid", ep.NID, "err", err)
				sum.EpisodesFailed++
				continue
			}
			if before == nil {
				sum.EpisodesCreated++
			} else {
				sum.EpisodesUpdated++
			}
		}

		if v.PosterURL == "" {
			sum.PosterSkipped++
		} else if !imp.FetchPosters || imp.Posters == nil {
			sum.PosterSkipped++
		} else {
			res, err := imp.Posters.Sync(ctx, olehdtv.SourceSystem, v.SourceID, v.PosterURL)
			if err != nil {
				imp.Log.Warn("poster sync failed", "source_id", v.SourceID, "err", err)
				sum.PosterFailed++
				_ = imp.Store.MarkVideoSyncError(id, "poster: "+err.Error())
			} else if res != nil {
				if err := imp.Store.UpdateVideoPoster(id, res.Key, v.PosterURL); err != nil {
					sum.PosterFailed++
				} else if res.Skipped {
					sum.PosterReused++
				} else {
					sum.PosterUploaded++
				}
			}
		}
		return nil
	})
	if err != nil {
		sum.Error = err.Error()
		return sum, err
	}

	if imp.ProgressEvery > 0 {
		imp.Log.Info(fmt.Sprintf("Videos: %d / %d", val.VideoCount, val.VideoCount))
		imp.Log.Info(fmt.Sprintf("Episodes: %d / %d", val.EpisodeCount, val.EpisodeCount))
	}

	if imp.DeactivateMissing {
		n, err := imp.Store.DeactivateMissingVideos(olehdtv.SourceSystem, keep)
		if err != nil {
			imp.Log.Warn("deactivate missing failed", "err", err)
		} else {
			imp.Log.Info("deactivated missing videos", "count", n)
		}
	}

	return sum, nil
}

func primaryPlaybackPreferStream(v olehdtv.SourceVideo) (source string, sid, nid int, url string) {
	if len(v.Episodes) == 0 {
		return "", 1, 1, ""
	}
	var fallback *olehdtv.SourceEpisode
	for i := range v.Episodes {
		e := &v.Episodes[i]
		if fallback == nil {
			fallback = e
		}
		if e.PlaybackURL != "" {
			if e.SID == 1 && e.NID == 1 {
				return e.PlaybackSource, e.SID, e.NID, e.PlaybackURL
			}
		}
	}
	for i := range v.Episodes {
		e := &v.Episodes[i]
		if e.PlaybackURL != "" {
			return e.PlaybackSource, e.SID, e.NID, e.PlaybackURL
		}
	}
	if fallback != nil {
		return fallback.PlaybackSource, fallback.SID, fallback.NID, fallback.PlaybackURL
	}
	return "", 1, 1, ""
}

func statusForJSONL(v olehdtv.SourceVideo) string {
	if !v.IsActive {
		return "disabled"
	}
	if len(v.Episodes) == 0 {
		return "draft"
	}
	// Ready if at least one playable stream exists; otherwise still ready so
	// catalog/detail work, and play returns "no playback url" for unavailable.
	return "ready"
}
