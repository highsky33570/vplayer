package store

import (
	"strings"

	"github.com/tycdn/vplayer/internal/model"
)

// maxCategorySlugLen matches categories.slug VARCHAR(64).
const maxCategorySlugLen = 64

// CategoryBrowseFilter describes how GET /api/v1/videos?category_id= is applied.
//
// Top-level (ParentID nil): CanonicalCategoryID = requested id, LeafSourceID empty
// → videos.category_id = ?
//
// Child (ParentID set): CanonicalCategoryID = parent_id, LeafSourceID = child.source_id
// → videos.category_id = parent_id AND videos.leaf_source_id = source_id
//
// FailClosed: invalid child (missing parent or source_id) → match nothing.
type CategoryBrowseFilter struct {
	CanonicalCategoryID uint64
	LeafSourceID        string
	FailClosed          bool
}

// NormalizeExcludeCategorySlug bounds an optional public list exclude slug.
// Empty / whitespace means "no exclusion". Overlong values are rejected as empty
// (fail closed: do not apply a truncated filter).
func NormalizeExcludeCategorySlug(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || len(s) > maxCategorySlugLen {
		return ""
	}
	return s
}

// CategoryBrowseFromRow maps a requested local category id + loaded row to a browse filter.
// Uses persisted parent_id / source_id only — never numeric ID band heuristics.
// cat == nil (unknown id): keep legacy behavior category_id = requestedID (typically empty).
func CategoryBrowseFromRow(requestedID uint64, cat *model.Category) CategoryBrowseFilter {
	if requestedID == 0 {
		return CategoryBrowseFilter{}
	}
	if cat == nil {
		return CategoryBrowseFilter{CanonicalCategoryID: requestedID}
	}
	if cat.ParentID == nil {
		return CategoryBrowseFilter{CanonicalCategoryID: requestedID}
	}
	parentID := *cat.ParentID
	sourceID := strings.TrimSpace(cat.SourceID)
	if parentID == 0 || sourceID == "" {
		return CategoryBrowseFilter{FailClosed: true}
	}
	return CategoryBrowseFilter{
		CanonicalCategoryID: parentID,
		LeafSourceID:        sourceID,
	}
}

// AppendReadyVideosWhere adds shared public list filters to a SQL fragment that
// already contains WHERE … ready/active predicates. Exclusion is applied here
// (before any ORDER BY / LIMIT / OFFSET the caller appends).
func AppendReadyVideosWhere(q string, args []any, browse CategoryBrowseFilter, query, excludeCategorySlug string) (string, []any) {
	if browse.FailClosed {
		q += ` AND 1=0`
	} else if browse.LeafSourceID != "" {
		q += ` AND category_id=? AND leaf_source_id=?`
		args = append(args, browse.CanonicalCategoryID, browse.LeafSourceID)
	} else if browse.CanonicalCategoryID > 0 {
		q += ` AND category_id=?`
		args = append(args, browse.CanonicalCategoryID)
	}
	if query = strings.TrimSpace(query); query != "" {
		like := "%" + query + "%"
		q += ` AND (title LIKE ? OR COALESCE(description,'') LIKE ? OR COALESCE(actors,'') LIKE ?)`
		args = append(args, like, like, like)
	}
	if slug := NormalizeExcludeCategorySlug(excludeCategorySlug); slug != "" {
		// Resolve by categories.slug (parameterized). Never interpolate slug into SQL.
		q += ` AND category_id NOT IN (SELECT id FROM categories WHERE slug=?)`
		args = append(args, slug)
	}
	return q, args
}

// VideoMatchesBrowse mirrors AppendReadyVideosWhere category predicates for unit tests.
func VideoMatchesBrowse(v model.Video, browse CategoryBrowseFilter) bool {
	if browse.FailClosed {
		return false
	}
	if browse.LeafSourceID != "" {
		return v.CategoryID == browse.CanonicalCategoryID && v.LeafSourceID == browse.LeafSourceID
	}
	if browse.CanonicalCategoryID > 0 {
		return v.CategoryID == browse.CanonicalCategoryID
	}
	return true
}
