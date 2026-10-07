package catalogsync

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/tycdn/vplayer/internal/media"
	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/olehdtv"
	"github.com/tycdn/vplayer/internal/store"
)

type OleHdTvSyncService struct {
	Store   *store.MySQL
	Adapter olehdtv.SourceAdapter
	Posters *media.PosterSyncer
	Log     *slog.Logger
	Batch   int
	Retries int
	Timeout time.Duration
}

func NewOleHdTvSyncService(st *store.MySQL, ad olehdtv.SourceAdapter, posters *media.PosterSyncer) *OleHdTvSyncService {
	return &OleHdTvSyncService{
		Store:   st,
		Adapter: ad,
		Posters: posters,
		Log:     slog.Default().With("component", "olehdtv-sync"),
		Batch:   50,
		Retries: 3,
		Timeout: 2 * time.Minute,
	}
}

func (s *OleHdTvSyncService) Run(ctx context.Context, trigger string) (model.SyncSummary, error) {
	sum := model.SyncSummary{}
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	s.Log.Info("sync started", "adapter", s.Adapter.Name(), "trigger", trigger)

	var categories []olehdtv.SourceCategory
	err := s.withRetry(ctx, "categories", func(c context.Context) error {
		var e error
		categories, e = s.Adapter.ListCategories(c)
		return e
	})
	if err != nil {
		_ = s.Store.InsertSyncRun(olehdtv.SourceSystem, trigger, sum, err.Error())
		return sum, err
	}

	catMap := map[string]uint64{}
	for _, c := range categories {
		slug := c.Slug
		if slug == "" {
			slug = "src-" + c.SourceID
		}
		id, created, updated, err := s.Store.UpsertCategoryResult(store.CategoryUpsert{
			SourceSystem: olehdtv.SourceSystem,
			SourceID:     c.SourceID,
			Name:         c.Name,
			Slug:         NormalizeSlug(slug),
			Sort:         c.Sort,
			IsActive:     true,
		})
		if err != nil {
			s.Log.Error("category upsert failed", "source_id", c.SourceID, "err", err)
			sum.Failed++
			continue
		}
		catMap[c.SourceID] = id
		if created {
			sum.Created++
		} else if updated {
			sum.Updated++
		} else {
			sum.Unchanged++
		}
	}

	var videos []olehdtv.SourceVideo
	err = s.withRetry(ctx, "videos", func(c context.Context) error {
		var e error
		videos, e = s.Adapter.ListVideos(c)
		return e
	})
	if err != nil {
		_ = s.Store.InsertSyncRun(olehdtv.SourceSystem, trigger, sum, err.Error())
		return sum, err
	}

	keep := make([]string, 0, len(videos))
	for i, v := range videos {
		if i > 0 && s.Batch > 0 && i%s.Batch == 0 {
			s.Log.Info("batch progress", "processed", i, "total", len(videos))
		}
		sum.Scanned++
		keep = append(keep, v.SourceID)

		catID := catMap[v.SourceCategoryID]
		if catID == 0 {
			if c, e := s.Store.GetCategoryBySource(olehdtv.SourceSystem, v.SourceCategoryID); e == nil && c != nil {
				catID = c.ID
			}
		}

		var srcUpdated *time.Time
		if !v.SourceUpdatedAt.IsZero() {
			t := v.SourceUpdatedAt
			srcUpdated = &t
		}

		playSrc, sid, nid, playURL := primaryPlayback(v)

		existing, _ := s.Store.GetVideoBySource(olehdtv.SourceSystem, v.SourceID)
		coverKey := ""
		if existing != nil {
			coverKey = existing.CoverR2Key
			if coverKey == "" {
				coverKey = existing.CoverKey
			}
		}

		id, created, updated, err := s.Store.UpsertVideoResult(store.VideoUpsert{
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
			Status:           statusFor(v),
		})
		if err != nil {
			s.Log.Error("video upsert failed", "source_id", v.SourceID, "err", err)
			sum.Failed++
			continue
		}
		if created {
			sum.Created++
		} else if updated {
			sum.Updated++
		} else {
			sum.Unchanged++
		}

		_ = s.Store.TouchVideoSeen(id)
		for _, ep := range v.Episodes {
			if err := s.Store.UpsertEpisode(store.EpisodeUpsert{
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
				IsActive:       ep.PlaybackStatus != "unavailable" || ep.PlaybackURL != "",
			}); err != nil {
				s.Log.Error("episode upsert failed", "source_id", v.SourceID, "sid", ep.SID, "nid", ep.NID, "err", err)
				sum.Failed++
			}
		}

		if v.PosterURL != "" && s.Posters != nil {
			res, err := s.Posters.Sync(ctx, olehdtv.SourceSystem, v.SourceID, v.PosterURL)
			if err != nil {
				s.Log.Warn("poster sync failed", "source_id", v.SourceID, "err", err)
				sum.PosterFailed++
				_ = s.Store.MarkVideoSyncError(id, "poster: "+err.Error())
			} else if res != nil {
				if err := s.Store.UpdateVideoPoster(id, res.Key, v.PosterURL); err != nil {
					s.Log.Warn("poster key save failed", "source_id", v.SourceID, "err", err)
					sum.PosterFailed++
				} else {
					sum.PosterOK++
				}
			}
		}
	}

	deactivated, err := s.Store.DeactivateMissingVideos(olehdtv.SourceSystem, keep)
	if err != nil {
		s.Log.Warn("deactivate missing failed", "err", err)
	} else {
		sum.Deactivated = deactivated
	}

	if err := s.Store.InsertSyncRun(olehdtv.SourceSystem, trigger, sum, ""); err != nil {
		s.Log.Warn("sync_run insert failed", "err", err)
	}

	s.Log.Info("sync finished",
		"scanned", sum.Scanned,
		"created", sum.Created,
		"updated", sum.Updated,
		"unchanged", sum.Unchanged,
		"failed", sum.Failed,
		"deactivated", sum.Deactivated,
		"poster_ok", sum.PosterOK,
		"poster_failed", sum.PosterFailed,
	)
	return sum, nil
}

func primaryPlayback(v olehdtv.SourceVideo) (source string, sid, nid int, url string) {
	if len(v.Episodes) == 0 {
		return "", 1, 1, ""
	}
	ep := v.Episodes[0]
	for _, e := range v.Episodes {
		if e.SID == 1 && e.NID == 1 {
			ep = e
			break
		}
	}
	return ep.PlaybackSource, ep.SID, ep.NID, ep.PlaybackURL
}

func statusFor(v olehdtv.SourceVideo) string {
	if !v.IsActive {
		return "disabled"
	}
	if len(v.Episodes) == 0 {
		return "draft"
	}
	return "ready"
}

func (s *OleHdTvSyncService) withRetry(ctx context.Context, label string, fn func(context.Context) error) error {
	var err error
	for attempt := 0; attempt < s.Retries; attempt++ {
		err = fn(ctx)
		if err == nil {
			return nil
		}
		s.Log.Warn("retry", "step", label, "attempt", attempt+1, "err", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * 200 * time.Millisecond):
		}
	}
	return err
}

func NormalizeSlug(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, " ", "-")
	return s
}
