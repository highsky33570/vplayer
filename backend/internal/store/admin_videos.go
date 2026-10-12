package store

import (
	"strings"

	"github.com/tycdn/vplayer/internal/model"
)

const adminVideoSelect = `
		SELECT id, COALESCE(source_system,''), COALESCE(source_id,''), category_id, COALESCE(source_category_id,''),
		       title, COALESCE(description,''), COALESCE(year,''), COALESCE(area,''), COALESCE(director,''),
		       COALESCE(actors,''), COALESCE(rating,0), cover_key, COALESCE(cover_r2_key,''), COALESCE(poster_source_url,''),
		       duration_sec, view_count, hls_master_key, COALESCE(playback_source,''), COALESCE(play_sid,1), COALESCE(play_nid,1),
		       COALESCE(playback_url,''), source_updated_at, last_synced_at, COALESCE(sync_status,'ok'),
		       COALESCE(is_active,1), status, created_at
		FROM videos`

// ListAdminVideosFiltered lists videos for admin regardless of status.
// active=nil means all; true/false filters is_active. limit is clamped to 1..100.
func (m *MySQL) ListAdminVideosFiltered(categoryID uint64, query string, active *bool, limit, offset int) ([]model.Video, error) {
	if limit <= 0 {
		limit = 24
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	q := adminVideoSelect + ` WHERE 1=1`
	args := []any{}
	if categoryID > 0 {
		q += ` AND category_id=?`
		args = append(args, categoryID)
	}
	if active != nil {
		q += ` AND COALESCE(is_active,1)=?`
		args = append(args, boolToInt(*active))
	}
	if query = strings.TrimSpace(query); query != "" {
		like := "%" + query + "%"
		q += ` AND title LIKE ?`
		args = append(args, like)
	}
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
func (m *MySQL) CountAdminVideosFiltered(categoryID uint64, query string, active *bool) (int, error) {
	q := `SELECT COUNT(*) FROM videos WHERE 1=1`
	args := []any{}
	if categoryID > 0 {
		q += ` AND category_id=?`
		args = append(args, categoryID)
	}
	if active != nil {
		q += ` AND COALESCE(is_active,1)=?`
		args = append(args, boolToInt(*active))
	}
	if query = strings.TrimSpace(query); query != "" {
		like := "%" + query + "%"
		q += ` AND title LIKE ?`
		args = append(args, like)
	}
	var n int
	if err := m.DB.QueryRow(q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
