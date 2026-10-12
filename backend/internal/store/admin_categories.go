package store

import (
	"database/sql"
	"strings"
	"time"
)

// AdminCategory is a read-only admin view of a category with video_count.
// Raw sync_error is never retained; HasError is derived from it.
type AdminCategory struct {
	ID           uint64
	Name         string
	Slug         string
	Sort         int
	IsActive     bool
	SourceSystem string
	SourceID     string
	LastSyncedAt *time.Time
	SyncStatus   string
	HasError     bool
	CreatedAt    time.Time
	VideoCount   int
}

const adminCategorySelect = `
		SELECT c.id, c.name, c.slug, c.sort,
		       COALESCE(c.source_system,''), COALESCE(c.source_id,''),
		       c.last_synced_at, COALESCE(c.sync_status,'ok'), COALESCE(c.sync_error,''),
		       COALESCE(c.is_active,1), c.created_at,
		       COUNT(v.id) AS video_count
		FROM categories c
		LEFT JOIN videos v ON v.category_id = c.id`

// ListAdminCategories returns all categories (active and inactive) with video counts.
// Ordering matches consumer ListCategories: sort ASC, id ASC.
func (m *MySQL) ListAdminCategories() ([]AdminCategory, error) {
	q := adminCategorySelect + `
		GROUP BY c.id
		ORDER BY c.sort ASC, c.id ASC`
	rows, err := m.DB.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminCategory
	for rows.Next() {
		c, err := scanAdminCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// GetAdminCategoryByID returns one category with video_count, or nil if missing.
func (m *MySQL) GetAdminCategoryByID(id uint64) (*AdminCategory, error) {
	row := m.DB.QueryRow(adminCategorySelect+`
		WHERE c.id=?
		GROUP BY c.id`, id)
	c, err := scanAdminCategory(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

type adminCategoryScannable interface {
	Scan(dest ...any) error
}

func scanAdminCategory(row adminCategoryScannable) (*AdminCategory, error) {
	var c AdminCategory
	var last sql.NullTime
	var active int
	var errText string
	err := row.Scan(
		&c.ID, &c.Name, &c.Slug, &c.Sort,
		&c.SourceSystem, &c.SourceID,
		&last, &c.SyncStatus, &errText,
		&active, &c.CreatedAt, &c.VideoCount,
	)
	if err != nil {
		return nil, err
	}
	c.IsActive = active == 1
	if last.Valid {
		t := last.Time
		c.LastSyncedAt = &t
	}
	if strings.TrimSpace(errText) != "" {
		c.HasError = true
	}
	return &c, nil
}
