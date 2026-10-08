package catalogsync

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/olehdtv"
	"github.com/tycdn/vplayer/internal/store"
)

// IncrementalStore is the DB surface used by incremental sync (mockable in tests).
type IncrementalStore interface {
	TryAdvisoryLock(name string, timeoutSec int) (bool, error)
	ReleaseAdvisoryLock(name string) error
	UpsertCategoryResult(in store.CategoryUpsert) (id uint64, created, updated bool, err error)
	GetCategoryBySource(system, sourceID string) (*model.Category, error)
	GetVideoBySource(system, sourceID string) (*model.Video, error)
	InsertVideoCatalog(in store.VideoUpsert) (uint64, error)
	UpdateVideoCatalog(id uint64, in store.VideoUpsert) error
	GetEpisodeForSync(system, sourceVideoID string, sid, nid int) (*store.EpisodeSyncRow, error)
	InsertEpisodeCatalog(in store.EpisodeUpsert) error
	UpdateEpisodeCatalog(id uint64, in store.EpisodeUpsert) error
	InsertSyncRunDetailed(system, trigger string, scanned, created, updated, unchanged, failed, deactivated int, summary any, errText string) error
}

// ItemAction is a concise per-item plan/result line.
type ItemAction struct {
	Action          string `json:"action"` // NEW VIDEO | UPDATE VIDEO | ...
	SourceID        string `json:"source_id,omitempty"`
	SID             int    `json:"sid,omitempty"`
	NID             int    `json:"nid,omitempty"`
	Title           string `json:"title,omitempty"`
	MediaSrcChanged bool   `json:"media_source_changed,omitempty"`
	Detail          string `json:"detail,omitempty"`
}

// IncrementalSummary is the dry-run / real-run report.
type IncrementalSummary struct {
	DryRun            bool                `json:"dry_run"`
	Adapter           string              `json:"adapter"`
	Limit             int                 `json:"limit"`
	Discovered        int                 `json:"discovered"`
	NewVideos         int                 `json:"new_videos"`
	UpdatedVideos     int                 `json:"updated_videos"`
	UnchangedVideos   int                 `json:"unchanged_videos"`
	NewEpisodes       int                 `json:"new_episodes"`
	UpdatedEpisodes   int                 `json:"updated_episodes"`
	UnchangedEpisodes int                 `json:"unchanged_episodes"`
	Failures          int                 `json:"failures"`
	MediaSourceChanged int                `json:"media_source_changed"`
	DeactivateMissingCalled bool          `json:"deactivate_missing_called"`
	LockAcquired      bool                `json:"lock_acquired"`
	Watermark         olehdtv.RecentMeta  `json:"watermark"`
	ListingRequests   int                 `json:"listing_requests,omitempty"`
	DetailRequests    int                 `json:"detail_requests,omitempty"`
	PlayRequests      int                 `json:"play_requests,omitempty"`
	TotalHTTPRequests int                 `json:"total_http_requests,omitempty"`
	EnrichmentCapped  bool                `json:"enrichment_capped,omitempty"`
	Actions           []ItemAction        `json:"actions,omitempty"`
	Error             string              `json:"error,omitempty"`
}

// IncrementalSync performs bounded OLEHDTV catalog synchronization.
type IncrementalSync struct {
	Store  IncrementalStore
	Source olehdtv.RecentSource
	Log    *slog.Logger
	LockName string
}

func NewIncrementalSync(st IncrementalStore, src olehdtv.RecentSource) *IncrementalSync {
	return &IncrementalSync{
		Store:    st,
		Source:   src,
		Log:      slog.Default().With("component", "olehdtv-incremental"),
		LockName: store.IncrementalSyncLockName,
	}
}

// Run executes incremental sync. dryRun performs zero DB writes (except advisory lock get/release).
func (s *IncrementalSync) Run(ctx context.Context, limit int, dryRun bool) (IncrementalSummary, error) {
	sum := IncrementalSummary{
		DryRun:   dryRun,
		Adapter:  s.Source.Name(),
		Limit:    limit,
		Actions:  make([]ItemAction, 0, limit*2),
		DeactivateMissingCalled: false,
	}
	if limit <= 0 {
		sum.Error = "limit must be > 0"
		return sum, fmt.Errorf("%s", sum.Error)
	}
	if s.Store == nil || s.Source == nil {
		sum.Error = "store and source are required"
		return sum, fmt.Errorf("%s", sum.Error)
	}

	got, err := s.Store.TryAdvisoryLock(s.LockName, 0)
	if err != nil {
		sum.Error = "advisory lock error: " + err.Error()
		return sum, err
	}
	if !got {
		sum.Error = "another incremental sync is already running"
		return sum, fmt.Errorf("%s", sum.Error)
	}
	sum.LockAcquired = true
	defer func() {
		_ = s.Store.ReleaseAdvisoryLock(s.LockName)
	}()

	cats, err := s.Source.ListCategories(ctx)
	if err != nil {
		sum.Failures++
		sum.Error = "list categories: " + err.Error()
		return sum, err
	}

	catMap := map[string]uint64{}
	for _, c := range cats {
		slug := c.Slug
		if slug == "" {
			slug = "src-" + c.SourceID
		}
		if dryRun {
			existing, _ := s.Store.GetCategoryBySource(olehdtv.SourceSystem, c.SourceID)
			if existing != nil {
				catMap[c.SourceID] = existing.ID
			}
			continue
		}
		id, _, _, err := s.Store.UpsertCategoryResult(store.CategoryUpsert{
			SourceSystem: olehdtv.SourceSystem,
			SourceID:     c.SourceID,
			Name:         c.Name,
			Slug:         NormalizeSlug(slug),
			Sort:         c.Sort,
			IsActive:     true,
		})
		if err != nil {
			s.Log.Error("category upsert failed", "source_id", c.SourceID, "err", err)
			sum.Failures++
			continue
		}
		catMap[c.SourceID] = id
	}

	videos, meta, err := s.Source.ListRecentVideos(ctx, limit)
	if err != nil {
		sum.Failures++
		sum.Error = "list recent videos: " + err.Error()
		return sum, err
	}
	sum.Watermark = meta
	sum.ListingRequests = meta.ListingRequests
	sum.DetailRequests = meta.DetailRequests
	sum.PlayRequests = meta.PlayRequests
	sum.TotalHTTPRequests = meta.TotalHTTPRequests
	sum.EnrichmentCapped = meta.EnrichmentCapped
	sum.Discovered = len(videos)

	for _, v := range videos {
		if err := ctx.Err(); err != nil {
			sum.Error = err.Error()
			sum.Failures++
			break
		}
		if err := s.processVideo(ctx, &sum, catMap, v, dryRun); err != nil {
			s.Log.Error("video failed", "source_id", v.SourceID, "err", err)
			sum.Failures++
			sum.Actions = append(sum.Actions, ItemAction{
				Action:   "FAILURE",
				SourceID: v.SourceID,
				Title:    v.Title,
				Detail:   err.Error(),
			})
		}
	}

	if !dryRun {
		errText := ""
		if sum.Failures > 0 {
			errText = fmt.Sprintf("%d failures", sum.Failures)
		}
		_ = s.Store.InsertSyncRunDetailed(
			olehdtv.SourceSystem,
			"incremental",
			sum.Discovered,
			sum.NewVideos,
			sum.UpdatedVideos,
			sum.UnchangedVideos,
			sum.Failures,
			0,
			sum,
			errText,
		)
	}

	if sum.Failures > 0 {
		return sum, fmt.Errorf("incremental sync completed with %d failures", sum.Failures)
	}
	return sum, nil
}

func (s *IncrementalSync) processVideo(ctx context.Context, sum *IncrementalSummary, catMap map[string]uint64, v olehdtv.SourceVideo, dryRun bool) error {
	_ = ctx
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
	existing, err := s.Store.GetVideoBySource(olehdtv.SourceSystem, v.SourceID)
	if err != nil {
		return err
	}

	coverKey := ""
	if existing != nil {
		coverKey = existing.CoverR2Key
		if coverKey == "" {
			coverKey = existing.CoverKey
		}
		// Partial recent discovery must not clear an existing primary playback URL.
		if playURL == "" {
			playURL = existing.PlaybackURL
			if playSrc == "" {
				playSrc = existing.PlaybackSource
			}
			if sid == 0 {
				sid = existing.PlaySID
			}
			if nid == 0 {
				nid = existing.PlayNID
			}
		}
	}

	in := store.VideoUpsert{
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
	}

	var videoID uint64
	switch {
	case existing == nil:
		sum.NewVideos++
		sum.Actions = append(sum.Actions, ItemAction{Action: "NEW VIDEO", SourceID: v.SourceID, Title: v.Title})
		if dryRun {
			videoID = 0
		} else {
			videoID, err = s.Store.InsertVideoCatalog(in)
			if err != nil {
				return err
			}
		}
	case VideoCatalogChanged(existing, in):
		sum.UpdatedVideos++
		sum.Actions = append(sum.Actions, ItemAction{Action: "UPDATE VIDEO", SourceID: v.SourceID, Title: v.Title})
		videoID = existing.ID
		if !dryRun {
			if err := s.Store.UpdateVideoCatalog(existing.ID, in); err != nil {
				return err
			}
		}
	default:
		sum.UnchangedVideos++
		sum.Actions = append(sum.Actions, ItemAction{Action: "UNCHANGED", SourceID: v.SourceID, Title: v.Title, Detail: "video"})
		videoID = existing.ID
		// intentionally no UPDATE — preserves updated_at / created_at
	}

	for _, ep := range v.Episodes {
		if err := s.processEpisode(sum, videoID, v.SourceID, ep, dryRun); err != nil {
			return err
		}
	}
	return nil
}

func (s *IncrementalSync) processEpisode(sum *IncrementalSummary, videoID uint64, sourceVideoID string, ep olehdtv.SourceEpisode, dryRun bool) error {
	status := ep.PlaybackStatus
	if status == "" {
		status = "ok"
	}
	active := ep.PlaybackURL != "" && status != "unavailable" && status != "pending_enrichment"
	existing, err := s.Store.GetEpisodeForSync(olehdtv.SourceSystem, sourceVideoID, ep.SID, ep.NID)
	if err != nil {
		return err
	}

	// Partial / capped observation: never infer disappearance, never wipe stream, never deactivate.
	if existing != nil && ep.PlaybackURL == "" && (status == "pending_enrichment" || status == "observed") {
		titleOrPage := existing.Title != ep.Title || (ep.PlayPageURL != "" && existing.PlayPageURL != ep.PlayPageURL)
		if !titleOrPage {
			sum.UnchangedEpisodes++
			sum.Actions = append(sum.Actions, ItemAction{
				Action: "UNCHANGED", SourceID: sourceVideoID, SID: ep.SID, NID: ep.NID, Title: ep.Title,
				Detail: "episode_partial_observation",
			})
			return nil
		}
		inSoft := store.EpisodeUpsert{
			VideoID:        existing.VideoID,
			SourceSystem:   olehdtv.SourceSystem,
			SourceVideoID:  sourceVideoID,
			SID:            ep.SID,
			NID:            ep.NID,
			Title:          ep.Title,
			PlaybackSource: existing.PlaybackSource,
			PlaybackURL:    existing.PlaybackURL,
			PlayPageURL:    firstNonEmptyStr(ep.PlayPageURL, existing.PlayPageURL),
			PlaybackStatus: existing.PlaybackStatus,
			IsActive:       existing.IsActive,
		}
		sum.UpdatedEpisodes++
		sum.Actions = append(sum.Actions, ItemAction{
			Action: "UPDATE EPISODE", SourceID: sourceVideoID, SID: ep.SID, NID: ep.NID, Title: ep.Title,
			Detail: "metadata_only_partial_observation",
		})
		if dryRun {
			return nil
		}
		return s.Store.UpdateEpisodeCatalog(existing.ID, inSoft)
	}

	in := store.EpisodeUpsert{
		VideoID:        videoID,
		SourceSystem:   olehdtv.SourceSystem,
		SourceVideoID:  sourceVideoID,
		SID:            ep.SID,
		NID:            ep.NID,
		Title:          ep.Title,
		PlaybackSource: ep.PlaybackSource,
		PlaybackURL:    ep.PlaybackURL,
		PlayPageURL:    ep.PlayPageURL,
		PlaybackStatus: status,
		IsActive:       active,
	}

	if existing == nil {
		sum.NewEpisodes++
		sum.Actions = append(sum.Actions, ItemAction{
			Action: "NEW EPISODE", SourceID: sourceVideoID, SID: ep.SID, NID: ep.NID, Title: ep.Title,
		})
		if dryRun {
			return nil
		}
		if videoID == 0 {
			return fmt.Errorf("missing video id for new episode %s s%d n%d", sourceVideoID, ep.SID, ep.NID)
		}
		return s.Store.InsertEpisodeCatalog(in)
	}

	mediaChanged := EpisodeMediaSourceChanged(existing, ep)
	if mediaChanged {
		sum.MediaSourceChanged++
	}

	if !EpisodeCatalogChanged(existing, ep) {
		sum.UnchangedEpisodes++
		sum.Actions = append(sum.Actions, ItemAction{
			Action: "UNCHANGED", SourceID: sourceVideoID, SID: ep.SID, NID: ep.NID, Title: ep.Title, Detail: "episode",
		})
		return nil
	}

	sum.UpdatedEpisodes++
	act := ItemAction{
		Action: "UPDATE EPISODE", SourceID: sourceVideoID, SID: ep.SID, NID: ep.NID, Title: ep.Title,
		MediaSrcChanged: mediaChanged,
	}
	if mediaChanged {
		act.Detail = "MEDIA_SOURCE_CHANGED"
	}
	sum.Actions = append(sum.Actions, act)

	if dryRun {
		return nil
	}
	in.VideoID = existing.VideoID
	if videoID != 0 {
		in.VideoID = videoID
	}
	return s.Store.UpdateEpisodeCatalog(existing.ID, in)
}

func firstNonEmptyStr(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// Ensure IncrementalStore is satisfied by *store.MySQL at compile time.
var _ IncrementalStore = (*store.MySQL)(nil)

// RedactActionTitle keeps logs free of huge URLs (titles only).
func RedactDetail(s string) string {
	if strings.Contains(strings.ToLower(s), "authorization") || strings.Contains(s, "Credential=") {
		return "(redacted)"
	}
	return s
}
