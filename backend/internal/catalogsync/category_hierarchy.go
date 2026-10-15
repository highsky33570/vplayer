package catalogsync

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/olehdtv"
	"github.com/tycdn/vplayer/internal/store"
)

// categorySyncStore is the store surface used by two-phase hierarchy upsert.
type categorySyncStore interface {
	UpsertCategoryResult(in store.CategoryUpsert) (id uint64, created, updated bool, err error)
	GetCategoryBySource(system, sourceID string) (*model.Category, error)
}

// CategoryUpsertStats counts outcomes for a two-phase category sync.
type CategoryUpsertStats struct {
	Created   int
	Updated   int
	Unchanged int
	Failed    int
}

// UpsertCategoriesTwoPhase merges verified OLEHDTV children, upserts top-level
// categories first, then children with resolved parent_id. Children whose
// ParentSourceID cannot be resolved are counted as Failed and are not inserted
// as top-level rows.
//
// dryRun: populate catMap from existing rows only (no writes).
func UpsertCategoriesTwoPhase(
	st categorySyncStore,
	cats []olehdtv.SourceCategory,
	dryRun bool,
	log *slog.Logger,
) (catMap map[string]uint64, stats CategoryUpsertStats, err error) {
	if log == nil {
		log = slog.Default()
	}
	merged := olehdtv.MergeVerifiedHierarchy(cats)
	catMap = make(map[string]uint64, len(merged))

	var tops, children []olehdtv.SourceCategory
	for _, c := range merged {
		if strings.TrimSpace(c.ParentSourceID) == "" {
			tops = append(tops, c)
		} else {
			children = append(children, c)
		}
	}

	for _, c := range tops {
		id, created, updated, uerr := upsertOneCategory(st, c, nil, dryRun, log)
		if uerr != nil {
			stats.Failed++
			log.Error("category upsert failed", "source_id", c.SourceID, "err", uerr)
			continue
		}
		if id > 0 {
			catMap[c.SourceID] = id
		}
		tallyCategoryStats(&stats, created, updated, dryRun)
	}

	for _, c := range children {
		parentSrc := strings.TrimSpace(c.ParentSourceID)
		parentID, ok := catMap[parentSrc]
		if !ok || parentID == 0 {
			// Resolve from DB when parent was not in this batch's top list map
			// (e.g. dry-run partial) — still required before insert.
			existingParent, gerr := st.GetCategoryBySource(olehdtv.SourceSystem, parentSrc)
			if gerr != nil {
				stats.Failed++
				log.Error("category parent lookup failed", "source_id", c.SourceID, "parent_source_id", parentSrc, "err", gerr)
				continue
			}
			if existingParent == nil || existingParent.ID == 0 {
				stats.Failed++
				log.Error("category parent unresolved; child not inserted as top-level",
					"source_id", c.SourceID, "parent_source_id", parentSrc,
					"err", fmt.Errorf("parent source_id %s not found", parentSrc))
				continue
			}
			parentID = existingParent.ID
			catMap[parentSrc] = parentID
		}
		pid := parentID
		id, created, updated, uerr := upsertOneCategory(st, c, &pid, dryRun, log)
		if uerr != nil {
			stats.Failed++
			log.Error("category upsert failed", "source_id", c.SourceID, "parent_source_id", parentSrc, "err", uerr)
			continue
		}
		if id > 0 {
			catMap[c.SourceID] = id
		}
		tallyCategoryStats(&stats, created, updated, dryRun)
	}

	return catMap, stats, nil
}

func tallyCategoryStats(stats *CategoryUpsertStats, created, updated, dryRun bool) {
	if dryRun {
		return
	}
	switch {
	case created:
		stats.Created++
	case updated:
		stats.Updated++
	default:
		stats.Unchanged++
	}
}

func upsertOneCategory(
	st categorySyncStore,
	c olehdtv.SourceCategory,
	parentID *uint64,
	dryRun bool,
	log *slog.Logger,
) (id uint64, created, updated bool, err error) {
	_ = log
	slug := c.Slug
	if slug == "" {
		if strings.TrimSpace(c.ParentSourceID) != "" {
			slug = olehdtv.ChildCategorySlug(c.SourceID)
		} else {
			slug = "src-" + c.SourceID
		}
	}
	if dryRun {
		existing, gerr := st.GetCategoryBySource(olehdtv.SourceSystem, c.SourceID)
		if gerr != nil {
			return 0, false, false, gerr
		}
		if existing != nil {
			return existing.ID, false, false, nil
		}
		return 0, false, false, nil
	}
	return st.UpsertCategoryResult(store.CategoryUpsert{
		SourceSystem: olehdtv.SourceSystem,
		SourceID:     c.SourceID,
		Name:         c.Name,
		Slug:         NormalizeSlug(slug),
		Sort:         c.Sort,
		IsActive:     true,
		ParentID:     parentID,
	})
}
