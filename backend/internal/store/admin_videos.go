package store

import (
	"fmt"
	"strings"

	"github.com/tycdn/vplayer/internal/model"
)

const adminVideoSelect = `SELECT ` + videoScanSelect + ` FROM videos`

// appendAdminVideosWhere applies the same discovery gates as the public catalog:
// hide OLEHDTV VIP蓝光影院 rows, hide membership-only / unplayable titles.
// Admin should not list titles users cannot play.
// browse mirrors public category/leaf filtering (parent id or parent+leaf_source_id).
func appendAdminVideosWhere(q string, args []any, browse CategoryBrowseFilter, query string, active, isHot *bool) (string, []any) {
	excl, exclArgs := AdminExcludeOleHdTvVIPSQL("")
	q += excl
	args = append(args, exclArgs...)
	if browse.FailClosed {
		q += ` AND 1=0`
	} else if browse.LeafSourceID != "" {
		q += ` AND category_id=? AND leaf_source_id=?`
		args = append(args, browse.CanonicalCategoryID, browse.LeafSourceID)
	} else if browse.CanonicalCategoryID > 0 {
		q += ` AND category_id=?`
		args = append(args, browse.CanonicalCategoryID)
	}
	if active != nil {
		q += ` AND COALESCE(is_active,1)=?`
		args = append(args, boolToInt(*active))
	}
	if isHot != nil {
		q += ` AND COALESCE(is_hot,0)=?`
		args = append(args, boolToInt(*isHot))
	}
	if query = strings.TrimSpace(query); query != "" {
		like := "%" + query + "%"
		q += ` AND title LIKE ?`
		args = append(args, like)
	}
	q += MembershipOnlyCatalogExclusionSQL("videos.id")
	q += PlayableEpisodeCatalogSQL("videos.id")
	return q, args
}

// ListAdminVideosFiltered lists admin videos that are playable (or admin VIP showcase).
// active/isHot=nil means all; true/false filters the column. limit is clamped to 1..100.
func (m *MySQL) ListAdminVideosFiltered(categoryID uint64, query string, active, isHot *bool, limit, offset int) ([]model.Video, error) {
	if limit <= 0 {
		limit = 24
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	browse, err := m.resolveCategoryBrowse(categoryID)
	if err != nil {
		return nil, err
	}
	q, args := appendAdminVideosWhere(adminVideoSelect+` WHERE 1=1`, nil, browse, query, active, isHot)
	q += ` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := m.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Video
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// CountAdminVideosFiltered counts videos matching the same filters as ListAdminVideosFiltered.
func (m *MySQL) CountAdminVideosFiltered(categoryID uint64, query string, active, isHot *bool) (int, error) {
	browse, err := m.resolveCategoryBrowse(categoryID)
	if err != nil {
		return 0, err
	}
	q, args := appendAdminVideosWhere(`SELECT COUNT(*) FROM videos WHERE 1=1`, nil, browse, query, active, isHot)
	var n int
	if err := m.DB.QueryRow(q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// AdminVideoUpdate holds optional editable catalog fields. Nil pointers are left untouched.
type AdminVideoUpdate struct {
	Title              *string
	Description        *string
	Year               *string
	Director           *string
	Actors             *string
	CategoryID         *uint64
	IsActive           *bool
	Status             *string
	PosterSourceURL    *string
	MembershipRequired *bool
	IsHot              *bool
}

// AdminUpdateVideoFields updates only the editable catalog columns present in u.
// Sync / source / playback keys are never touched. When PosterSourceURL changes,
// cover_key and cover_r2_key are cleared so the new source URL is what clients see
// (withCoverURL otherwise prefers imported keys over poster_source_url).
// Returns false when no row exists for id.
func (m *MySQL) AdminUpdateVideoFields(id uint64, u AdminVideoUpdate) (bool, error) {
	if id == 0 {
		return false, fmt.Errorf("video id required")
	}
	sets := []string{}
	args := []any{}
	add := func(col string, v any) {
		sets = append(sets, col+`=?`)
		args = append(args, v)
	}
	if u.Title != nil {
		add("title", *u.Title)
	}
	if u.Description != nil {
		add("description", *u.Description)
	}
	if u.Year != nil {
		add("year", *u.Year)
	}
	if u.Director != nil {
		add("director", *u.Director)
	}
	if u.Actors != nil {
		add("actors", *u.Actors)
	}
	if u.CategoryID != nil {
		add("category_id", *u.CategoryID)
	}
	if u.IsActive != nil {
		add("is_active", boolToInt(*u.IsActive))
	}
	if u.Status != nil {
		add("status", *u.Status)
	}
	if u.PosterSourceURL != nil {
		add("poster_source_url", *u.PosterSourceURL)
		add("cover_key", "")
		add("cover_r2_key", "")
	}
	if u.MembershipRequired != nil {
		add("membership_required", boolToInt(*u.MembershipRequired))
	}
	if u.IsHot != nil {
		add("is_hot", boolToInt(*u.IsHot))
	}
	if len(sets) == 0 {
		var n int
		if err := m.DB.QueryRow(`SELECT COUNT(*) FROM videos WHERE id=?`, id).Scan(&n); err != nil {
			return false, err
		}
		return n > 0, nil
	}
	args = append(args, id)
	if _, err := m.DB.Exec(`UPDATE videos SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...); err != nil {
		return false, err
	}
	var n int
	if err := m.DB.QueryRow(`SELECT COUNT(*) FROM videos WHERE id=?`, id).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}
