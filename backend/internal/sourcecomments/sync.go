package sourcecomments

import (
	"context"
	"fmt"
	"time"

	"github.com/tycdn/vplayer/internal/olehdtv/commenthtml"
	"github.com/tycdn/vplayer/internal/store"
)

// Mode selects a full page walk or a newest-page incremental walk.
type Mode int

const (
	// ModeFull reads every public comment page for one video.
	ModeFull Mode = iota
	// ModeIncremental stops after a page whose parent comment ids are already stored.
	// New replies on older parents live on those parents' pages, so this mode does not
	// see them. A later ModeFull pass is required for that.
	ModeIncremental
)

const maxCommentPages = 500

// PageFetcher loads one anonymous public comment page as HTML.
type PageFetcher interface {
	FetchCommentPage(ctx context.Context, sourceVideoID string, page int) (string, error)
}

// Result counts one single-video sync. It never includes a catalog status change.
type Result struct {
	PagesFetched int
	Discovered   int
	Inserted     int
	Updated      int
	Skipped      int
	Errors       []string
}

// Sync reads public source comments for one local video and upserts them.
// It does not read or write video catalog status, and nothing calls it during startup.
func Sync(ctx context.Context, repo store.SourceCommentRepo, fetcher PageFetcher, videoID uint64, sourceSystem, sourceVideoID string, mode Mode, now time.Time) (Result, error) {
	var result Result
	if repo == nil || fetcher == nil {
		return result, fmt.Errorf("comment sync is not configured")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	last := 1
	for page := 1; page <= last; page++ {
		if page > maxCommentPages {
			err := fmt.Errorf("comment page limit %d exceeded", maxCommentPages)
			result.Errors = append(result.Errors, err.Error())
			return result, err
		}
		raw, err := fetcher.FetchCommentPage(ctx, sourceVideoID, page)
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			return result, err
		}
		parsed, err := commenthtml.Parse(raw, sourceVideoID)
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			return result, err
		}
		result.PagesFetched++
		result.Discovered += len(parsed.Comments)
		if parsed.LastPage > last {
			last = parsed.LastPage
		}
		parentIDs := parentIDs(parsed.Comments)
		known, err := repo.ExistingSourceCommentIDs(sourceSystem, parentIDs)
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			return result, err
		}
		allKnown := len(parentIDs) > 0
		for _, id := range parentIDs {
			if _, ok := known[id]; !ok {
				allKnown = false
				break
			}
		}
		inserted, updated, skipped, err := repo.UpsertSourceComments(videoID, sourceSystem, toInputs(parsed.Comments), now)
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			return result, err
		}
		result.Inserted += inserted
		result.Updated += updated
		result.Skipped += skipped
		if mode == ModeIncremental && (allKnown || len(parentIDs) == 0) {
			break
		}
		if parsed.LastPage == 0 && len(parsed.Comments) == 0 {
			break
		}
	}
	return result, nil
}

func parentIDs(rows []commenthtml.SourceComment) []string {
	var ids []string
	for _, row := range rows {
		if row.ParentSourceCommentID == "" && row.SourceCommentID != "" {
			ids = append(ids, row.SourceCommentID)
		}
	}
	return ids
}

func toInputs(rows []commenthtml.SourceComment) []store.SourceCommentInput {
	out := make([]store.SourceCommentInput, 0, len(rows))
	for _, row := range rows {
		out = append(out, store.SourceCommentInput{
			SourceCommentID:       row.SourceCommentID,
			SourceVideoID:         row.SourceVideoID,
			ParentSourceCommentID: row.ParentSourceCommentID,
			AuthorName:            row.AuthorName,
			AvatarPath:            row.AvatarPath,
			Body:                  row.Body,
			SourceCreatedRaw:      row.SourceCreatedRaw,
			DiggUp:                row.DiggUp,
			DiggDown:              row.DiggDown,
		})
	}
	return out
}
