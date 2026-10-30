package store

import (
	"database/sql"
	"errors"
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
	ParentID     *uint64
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
		SELECT c.id, c.name, c.slug, c.sort, c.parent_id,
		       COALESCE(c.source_system,''), COALESCE(c.source_id,''),
		       c.last_synced_at, COALESCE(c.sync_status,'ok'), COALESCE(c.sync_error,''),
		       COALESCE(c.is_active,1), c.created_at,
		       COUNT(v.id) AS video_count
		FROM categories c
		LEFT JOIN videos v ON v.category_id = c.id`

// ListAdminCategories returns all categories (active and inactive) with video counts.
// Ordered by id ASC (sequential ids in the admin UI).
func (m *MySQL) ListAdminCategories() ([]AdminCategory, error) {
	q := adminCategorySelect + `
		GROUP BY c.id
		ORDER BY c.id ASC`
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

// AdminCategoryUpdate holds optional editable category fields. Nil pointers are left untouched.
type AdminCategoryUpdate struct {
	Name     *string
	Sort     *int
	IsActive *bool
	ParentID *uint64 // move subcategory under another top-level main
}

// AdminCategoryCreate is the input for inserting a subcategory under a main.
type AdminCategoryCreate struct {
	Name     string
	ParentID uint64
	Sort     int
	Slug     string
	SourceID string
}

// UpdateAdminCategory updates name / sort / is_active / parent_id. Returns false when id does not exist.
func (m *MySQL) UpdateAdminCategory(id uint64, u AdminCategoryUpdate) (bool, error) {
	if id == 0 {
		return false, nil
	}
	sets := []string{}
	args := []any{}
	if u.Name != nil {
		sets = append(sets, `name=?`)
		args = append(args, *u.Name)
	}
	if u.Sort != nil {
		sets = append(sets, `sort=?`)
		args = append(args, *u.Sort)
	}
	if u.IsActive != nil {
		sets = append(sets, `is_active=?`)
		args = append(args, boolToInt(*u.IsActive))
	}
	if u.ParentID != nil {
		sets = append(sets, `parent_id=?`)
		args = append(args, *u.ParentID)
	}
	if len(sets) > 0 {
		args = append(args, id)
		if _, err := m.DB.Exec(`UPDATE categories SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...); err != nil {
			return false, err
		}
	}
	var n int
	if err := m.DB.QueryRow(`SELECT COUNT(*) FROM categories WHERE id=?`, id).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// CreateAdminCategory inserts a subcategory. Caller must validate parent is top-level.
func (m *MySQL) CreateAdminCategory(in AdminCategoryCreate) (uint64, error) {
	if in.ParentID == 0 || strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Slug) == "" {
		return 0, errors.New("invalid category create")
	}
	res, err := m.DB.Exec(`
		INSERT INTO categories (name, slug, sort, source_system, source_id, parent_id, sync_status, is_active)
		VALUES (?, ?, ?, 'admin', ?, ?, 'ok', 1)`,
		strings.TrimSpace(in.Name), strings.TrimSpace(in.Slug), in.Sort, strings.TrimSpace(in.SourceID), in.ParentID)
	if err != nil {
		return 0, err
	}
	lid, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return uint64(lid), nil
}

// CategoryParentID returns the parent_id of a category (nil = top-level), or ok=false if missing.
func (m *MySQL) CategoryParentID(id uint64) (parentID *uint64, ok bool, err error) {
	if id == 0 {
		return nil, false, nil
	}
	var p sql.NullInt64
	err = m.DB.QueryRow(`SELECT parent_id FROM categories WHERE id=?`, id).Scan(&p)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if p.Valid && p.Int64 > 0 {
		v := uint64(p.Int64)
		return &v, true, nil
	}
	return nil, true, nil
}

// DeactivateAdminSubcategory sets is_active=0 for a child category only.
// Returns false when id is missing or is a top-level main.
func (m *MySQL) DeactivateAdminSubcategory(id uint64) (bool, error) {
	if id == 0 {
		return false, nil
	}
	res, err := m.DB.Exec(`UPDATE categories SET is_active=0 WHERE id=? AND parent_id IS NOT NULL`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// CleanupDuplicateCategories merges categories that share the same trimmed name
// (case-sensitive). The keeper is the one with the most videos (tie: lowest id);
// the keeper is (re)activated, the others are deactivated, and their videos and
// child categories are reassigned to the keeper.
// merged = duplicate name groups handled; deactivated = categories newly deactivated;
// videosMoved = videos reassigned.
func (m *MySQL) CleanupDuplicateCategories() (merged, deactivated, videosMoved int, err error) {
	cats, err := m.ListAdminCategories()
	if err != nil {
		return 0, 0, 0, err
	}
	groups := map[string][]AdminCategory{}
	order := []string{}
	for _, c := range cats {
		key := strings.TrimSpace(c.Name)
		if key == "" {
			continue
		}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], c)
	}
	tx, err := m.DB.Begin()
	if err != nil {
		return 0, 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, key := range order {
		g := groups[key]
		if len(g) < 2 {
			continue
		}
		keeper := g[0]
		for _, c := range g[1:] {
			if c.VideoCount > keeper.VideoCount || (c.VideoCount == keeper.VideoCount && c.ID < keeper.ID) {
				keeper = c
			}
		}
		if _, err := tx.Exec(`UPDATE categories SET is_active=1 WHERE id=?`, keeper.ID); err != nil {
			return 0, 0, 0, err
		}
		for _, c := range g {
			if c.ID == keeper.ID {
				continue
			}
			res, err := tx.Exec(`UPDATE videos SET category_id=? WHERE category_id=?`, keeper.ID, c.ID)
			if err != nil {
				return 0, 0, 0, err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				videosMoved += int(n)
			}
			if _, err := tx.Exec(`UPDATE categories SET parent_id=? WHERE parent_id=? AND id<>?`, keeper.ID, c.ID, keeper.ID); err != nil {
				return 0, 0, 0, err
			}
			if _, err := tx.Exec(`UPDATE categories SET is_active=0 WHERE id=?`, c.ID); err != nil {
				return 0, 0, 0, err
			}
			if c.IsActive {
				deactivated++
			}
		}
		merged++
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, 0, err
	}
	return merged, deactivated, videosMoved, nil
}

type adminCategoryScannable interface {
	Scan(dest ...any) error
}

func scanAdminCategory(row adminCategoryScannable) (*AdminCategory, error) {
	var c AdminCategory
	var last sql.NullTime
	var parentID sql.NullInt64
	var active int
	var errText string
	err := row.Scan(
		&c.ID, &c.Name, &c.Slug, &c.Sort, &parentID,
		&c.SourceSystem, &c.SourceID,
		&last, &c.SyncStatus, &errText,
		&active, &c.CreatedAt, &c.VideoCount,
	)
	if err != nil {
		return nil, err
	}
	c.IsActive = active == 1
	c.ParentID = nullUint64Ptr(parentID)
	if last.Valid {
		t := last.Time
		c.LastSyncedAt = &t
	}
	if strings.TrimSpace(errText) != "" {
		c.HasError = true
	}
	return &c, nil
}
