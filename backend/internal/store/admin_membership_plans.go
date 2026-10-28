package store

import (
	"database/sql"
	"errors"
	"strings"
)

// ErrMembershipPlanExists is returned when creating a plan with a duplicate slug.
var ErrMembershipPlanExists = errors.New("membership plan slug already exists")

// AdminMembershipPlan is an admin view of a plan (active or not).
type AdminMembershipPlan struct {
	ID           uint64
	Slug         string
	Name         string
	PriceUSD     float64
	DurationDays int
	Sort         int
	IsActive     bool
}

// AdminMembershipPlanUpdate holds optional editable plan fields (slug is immutable).
type AdminMembershipPlanUpdate struct {
	Name         *string
	PriceUSD     *float64
	DurationDays *int
	Sort         *int
	IsActive     *bool
}

const adminMembershipPlanSelect = `
		SELECT id, slug, name, price_usd, duration_days, sort, COALESCE(is_active,1)
		FROM membership_plans`

func scanAdminMembershipPlan(row adminUserScannable) (*AdminMembershipPlan, error) {
	var p AdminMembershipPlan
	var active int
	if err := row.Scan(&p.ID, &p.Slug, &p.Name, &p.PriceUSD, &p.DurationDays, &p.Sort, &active); err != nil {
		return nil, err
	}
	p.IsActive = active == 1
	return &p, nil
}

// ListAllMembershipPlans returns every plan including inactive ones.
func (m *MySQL) ListAllMembershipPlans() ([]AdminMembershipPlan, error) {
	rows, err := m.DB.Query(adminMembershipPlanSelect + ` ORDER BY sort ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminMembershipPlan
	for rows.Next() {
		p, err := scanAdminMembershipPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetMembershipPlanByID loads one plan (active or not), or nil if missing.
func (m *MySQL) GetMembershipPlanByID(id uint64) (*AdminMembershipPlan, error) {
	p, err := scanAdminMembershipPlan(m.DB.QueryRow(adminMembershipPlanSelect+` WHERE id=? LIMIT 1`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// GetMembershipPlanBySlugAny loads one plan by slug regardless of is_active, or nil if missing.
func (m *MySQL) GetMembershipPlanBySlugAny(slug string) (*AdminMembershipPlan, error) {
	p, err := scanAdminMembershipPlan(m.DB.QueryRow(adminMembershipPlanSelect+` WHERE slug=? LIMIT 1`, strings.TrimSpace(slug)))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// UpdateMembershipPlan updates editable plan fields. Returns false when id does not exist.
func (m *MySQL) UpdateMembershipPlan(id uint64, u AdminMembershipPlanUpdate) (bool, error) {
	if id == 0 {
		return false, nil
	}
	sets := []string{}
	args := []any{}
	if u.Name != nil {
		sets = append(sets, `name=?`)
		args = append(args, *u.Name)
	}
	if u.PriceUSD != nil {
		sets = append(sets, `price_usd=?`)
		args = append(args, *u.PriceUSD)
	}
	if u.DurationDays != nil {
		sets = append(sets, `duration_days=?`)
		args = append(args, *u.DurationDays)
	}
	if u.Sort != nil {
		sets = append(sets, `sort=?`)
		args = append(args, *u.Sort)
	}
	if u.IsActive != nil {
		sets = append(sets, `is_active=?`)
		args = append(args, boolToInt(*u.IsActive))
	}
	if len(sets) > 0 {
		args = append(args, id)
		if _, err := m.DB.Exec(`UPDATE membership_plans SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...); err != nil {
			return false, err
		}
	}
	var n int
	if err := m.DB.QueryRow(`SELECT COUNT(*) FROM membership_plans WHERE id=?`, id).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// CreateMembershipPlan inserts a plan and returns its id. Duplicate slug returns ErrMembershipPlanExists.
func (m *MySQL) CreateMembershipPlan(p AdminMembershipPlan) (uint64, error) {
	res, err := m.DB.Exec(`
		INSERT INTO membership_plans (slug, name, price_usd, duration_days, sort, is_active)
		VALUES (?,?,?,?,?,?)`,
		strings.TrimSpace(p.Slug), p.Name, p.PriceUSD, p.DurationDays, p.Sort, boolToInt(p.IsActive))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return 0, ErrMembershipPlanExists
		}
		return 0, err
	}
	id, err := res.LastInsertId()
	return uint64(id), err
}
