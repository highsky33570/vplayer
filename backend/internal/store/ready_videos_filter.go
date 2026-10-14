package store

import "strings"

// maxCategorySlugLen matches categories.slug VARCHAR(64).
const maxCategorySlugLen = 64

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

// AppendReadyVideosWhere adds shared public list filters to a SQL fragment that
// already contains WHERE … ready/active predicates. Exclusion is applied here
// (before any ORDER BY / LIMIT / OFFSET the caller appends).
func AppendReadyVideosWhere(q string, args []any, categoryID uint64, query, excludeCategorySlug string) (string, []any) {
	if categoryID > 0 {
		q += ` AND category_id=?`
		args = append(args, categoryID)
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
