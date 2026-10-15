package catalogsync

import (
	"fmt"
	"strings"
	"time"

	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/olehdtv"
	"github.com/tycdn/vplayer/internal/store"
)

// CatalogWriter is the DB surface for applying a SourceVideo into catalog tables.
type CatalogWriter interface {
	GetVideoBySource(system, sourceID string) (*model.Video, error)
	GetCategoryBySource(system, sourceID string) (*model.Category, error)
	InsertVideoCatalog(in store.VideoUpsert) (uint64, error)
	UpdateVideoCatalog(id uint64, in store.VideoUpsert) error
	GetEpisodeForSync(system, sourceVideoID string, sid, nid int) (*store.EpisodeSyncRow, error)
	InsertEpisodeCatalog(in store.EpisodeUpsert) error
	UpdateEpisodeCatalog(id uint64, in store.EpisodeUpsert) error
}

// CatalogApplyResult reports what ApplySourceVideoCatalog did (or would do on dry-run).
type CatalogApplyResult struct {
	VideoID            uint64
	NewVideo           bool
	UpdatedVideo       bool
	UnchangedVideo     bool
	NewEpisodes        int
	UpdatedEpisodes    int
	UnchangedEpisodes  int
	MediaSourceChanged int
	Actions            []ItemAction
}

// ApplySourceVideoCatalog upserts a SourceVideo and its episodes using the same
// rules as incremental sync (including playback preservation).
// catMap maps source_category_id → local category id; GetCategoryBySource is a fallback.
func ApplySourceVideoCatalog(
	st CatalogWriter,
	catMap map[string]uint64,
	v olehdtv.SourceVideo,
	dryRun bool,
) (CatalogApplyResult, error) {
	var out CatalogApplyResult
	if st == nil {
		return out, fmt.Errorf("catalog writer required")
	}

	srcCatID := strings.TrimSpace(v.SourceCategoryID)
	catID := uint64(0)
	if srcCatID != "" {
		catID = catMap[srcCatID]
		if catID == 0 {
			if c, e := st.GetCategoryBySource(olehdtv.SourceSystem, srcCatID); e == nil && c != nil {
				catID = c.ID
			}
		}
	}

	var srcUpdated *time.Time
	if !v.SourceUpdatedAt.IsZero() {
		t := v.SourceUpdatedAt
		srcUpdated = &t
	}
	playSrc, sid, nid, playURL := primaryPlayback(v)
	existing, err := st.GetVideoBySource(olehdtv.SourceSystem, v.SourceID)
	if err != nil {
		return out, err
	}

	coverKey := ""
	if existing != nil {
		coverKey = existing.CoverR2Key
		if coverKey == "" {
			coverKey = existing.CoverKey
		}
		// Detail-only / partial discovery may omit TypeID — never zero an existing category.
		if srcCatID == "" {
			srcCatID = existing.SourceCategoryID
			catID = existing.CategoryID
		} else if catID == 0 {
			catID = existing.CategoryID
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
		SourceCategoryID: srcCatID,
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
		out.NewVideo = true
		out.Actions = append(out.Actions, ItemAction{Action: "NEW VIDEO", SourceID: v.SourceID, Title: v.Title})
		if dryRun {
			videoID = 0
		} else {
			videoID, err = st.InsertVideoCatalog(in)
			if err != nil {
				return out, err
			}
		}
	case VideoCatalogChanged(existing, in):
		out.UpdatedVideo = true
		out.Actions = append(out.Actions, ItemAction{Action: "UPDATE VIDEO", SourceID: v.SourceID, Title: v.Title})
		videoID = existing.ID
		if !dryRun {
			if err := st.UpdateVideoCatalog(existing.ID, in); err != nil {
				return out, err
			}
		}
	default:
		out.UnchangedVideo = true
		out.Actions = append(out.Actions, ItemAction{Action: "UNCHANGED", SourceID: v.SourceID, Title: v.Title, Detail: "video"})
		videoID = existing.ID
	}
	out.VideoID = videoID

	for _, ep := range v.Episodes {
		if err := applySourceEpisode(st, &out, videoID, v.SourceID, ep, dryRun); err != nil {
			return out, err
		}
	}
	return out, nil
}

func applySourceEpisode(
	st CatalogWriter,
	out *CatalogApplyResult,
	videoID uint64,
	sourceVideoID string,
	ep olehdtv.SourceEpisode,
	dryRun bool,
) error {
	status := ep.PlaybackStatus
	if status == "" {
		status = "ok"
	}
	active := ep.PlaybackURL != "" && status != "unavailable" && status != "pending_enrichment"
	existing, err := st.GetEpisodeForSync(olehdtv.SourceSystem, sourceVideoID, ep.SID, ep.NID)
	if err != nil {
		return err
	}

	if existing != nil && mustPreserveExistingPlayback(existing, ep) {
		titleOrPage := existing.Title != ep.Title || (ep.PlayPageURL != "" && existing.PlayPageURL != ep.PlayPageURL)
		if !titleOrPage {
			out.UnchangedEpisodes++
			out.Actions = append(out.Actions, ItemAction{
				Action: "UNCHANGED", SourceID: sourceVideoID, SID: ep.SID, NID: ep.NID, Title: ep.Title,
				Detail: "preserve_existing_playback",
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
		out.UpdatedEpisodes++
		out.Actions = append(out.Actions, ItemAction{
			Action: "UPDATE EPISODE", SourceID: sourceVideoID, SID: ep.SID, NID: ep.NID, Title: ep.Title,
			Detail: "metadata_only_preserve_playback",
		})
		if dryRun {
			return nil
		}
		return st.UpdateEpisodeCatalog(existing.ID, inSoft)
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
		out.NewEpisodes++
		out.Actions = append(out.Actions, ItemAction{
			Action: "NEW EPISODE", SourceID: sourceVideoID, SID: ep.SID, NID: ep.NID, Title: ep.Title,
		})
		if dryRun {
			return nil
		}
		if videoID == 0 {
			return fmt.Errorf("missing video id for new episode %s s%d n%d", sourceVideoID, ep.SID, ep.NID)
		}
		return st.InsertEpisodeCatalog(in)
	}

	mediaChanged := EpisodeMediaSourceChanged(existing, ep)
	if mediaChanged {
		out.MediaSourceChanged++
	}

	if !EpisodeCatalogChanged(existing, ep) {
		out.UnchangedEpisodes++
		out.Actions = append(out.Actions, ItemAction{
			Action: "UNCHANGED", SourceID: sourceVideoID, SID: ep.SID, NID: ep.NID, Title: ep.Title, Detail: "episode",
		})
		return nil
	}

	out.UpdatedEpisodes++
	act := ItemAction{
		Action: "UPDATE EPISODE", SourceID: sourceVideoID, SID: ep.SID, NID: ep.NID, Title: ep.Title,
		MediaSrcChanged: mediaChanged,
	}
	if mediaChanged {
		act.Detail = "MEDIA_SOURCE_CHANGED"
	}
	out.Actions = append(out.Actions, act)

	if dryRun {
		return nil
	}
	in.VideoID = existing.VideoID
	if videoID != 0 {
		in.VideoID = videoID
	}
	return st.UpdateEpisodeCatalog(existing.ID, in)
}
