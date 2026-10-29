package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// AdminUser is a read-only admin view of a user. password_hash is never selected.
// IsMember is the effective membership (unexpired expires_at, or lifetime flag with no expiry).
type AdminUser struct {
	ID                  uint64
	Email               string
	Nickname            string
	IsAdmin             bool
	IsMember            bool
	MembershipExpiresAt *time.Time
	LatestPlanSlug      string
	LatestPlanName      string
	CreatedAt           time.Time
}

const adminUserSelect = `
		SELECT id, email, COALESCE(nickname,''), COALESCE(is_admin,0), COALESCE(is_member,0), membership_expires_at, created_at
		FROM users`

// ListAdminUsersFiltered lists users for admin with optional q / is_admin filters.
// isAdmin=nil means all; true/false filters is_admin. limit is clamped to 1..100.
// Search uses parameterized LIKE on email and nickname; %, _, and \ remain SQL wildcards
// (same behavior as Admin Videos title search — not escaped).
func (m *MySQL) ListAdminUsersFiltered(query string, isAdmin *bool, limit, offset int) ([]AdminUser, error) {
	if limit <= 0 {
		limit = 24
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	q := adminUserSelect + ` WHERE 1=1`
	args := []any{}
	if isAdmin != nil {
		q += ` AND COALESCE(is_admin,0)=?`
		args = append(args, boolToInt(*isAdmin))
	}
	if query = strings.TrimSpace(query); query != "" {
		like := "%" + query + "%"
		q += ` AND (email LIKE ? OR COALESCE(nickname,'') LIKE ?)`
		args = append(args, like, like)
	}
	q += ` ORDER BY id ASC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := m.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminUser
	for rows.Next() {
		u, err := scanAdminUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for i := range out {
		if err := m.fillAdminUserLatestPlan(&out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// CountAdminUsersFiltered counts users matching the same filters as ListAdminUsersFiltered.
func (m *MySQL) CountAdminUsersFiltered(query string, isAdmin *bool) (int, error) {
	q := `SELECT COUNT(*) FROM users WHERE 1=1`
	args := []any{}
	if isAdmin != nil {
		q += ` AND COALESCE(is_admin,0)=?`
		args = append(args, boolToInt(*isAdmin))
	}
	if query = strings.TrimSpace(query); query != "" {
		like := "%" + query + "%"
		q += ` AND (email LIKE ? OR COALESCE(nickname,'') LIKE ?)`
		args = append(args, like, like)
	}
	var n int
	if err := m.DB.QueryRow(q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// GetAdminUserByID returns one admin user view, or nil if missing.
func (m *MySQL) GetAdminUserByID(id uint64) (*AdminUser, error) {
	row := m.DB.QueryRow(adminUserSelect+` WHERE id=? LIMIT 1`, id)
	u, err := scanAdminUser(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := m.fillAdminUserLatestPlan(u); err != nil {
		return nil, err
	}
	return u, nil
}

// fillAdminUserLatestPlan loads the plan of the user's most recent paid membership order.
func (m *MySQL) fillAdminUserLatestPlan(u *AdminUser) error {
	err := m.DB.QueryRow(`
		SELECT plan_slug, plan_name
		FROM membership_orders
		WHERE user_id=? AND status=?
		ORDER BY paid_at DESC, id DESC
		LIMIT 1`, u.ID, MembershipOrderPaid).Scan(&u.LatestPlanSlug, &u.LatestPlanName)
	if err == sql.ErrNoRows {
		return nil
	}
	return err
}

type adminUserScannable interface {
	Scan(dest ...any) error
}

func scanAdminUser(row adminUserScannable) (*AdminUser, error) {
	var u AdminUser
	var isAdmin, isMember int
	var exp sql.NullTime
	err := row.Scan(&u.ID, &u.Email, &u.Nickname, &isAdmin, &isMember, &exp, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	u.IsAdmin = isAdmin == 1
	if exp.Valid {
		t := exp.Time
		u.MembershipExpiresAt = &t
	}
	u.IsMember = adminEffectiveMember(isMember == 1, u.MembershipExpiresAt, time.Now())
	return &u, nil
}

// adminEffectiveMember mirrors EffectiveMember without the admin override:
// an expiry date decides when set, otherwise the lifetime is_member flag applies.
func adminEffectiveMember(flag bool, expiresAt *time.Time, now time.Time) bool {
	if expiresAt != nil {
		return expiresAt.After(now)
	}
	return flag
}

// ErrLastAdmin is returned when a change would leave the site without any admin.
var ErrLastAdmin = errors.New("cannot remove the last admin")

// AdminUserUpdate holds editable user fields. Nickname is always written.
// IsAdmin nil leaves the flag alone. SetMembership=true writes IsMember/MembershipExpiresAt.
type AdminUserUpdate struct {
	Nickname            string
	IsAdmin             *bool
	SetMembership       bool
	IsMember            bool
	MembershipExpiresAt *time.Time
}

// UpdateAdminUser updates nickname and optionally admin / membership fields.
// Demoting the last admin returns ErrLastAdmin; a missing user returns ErrUserNotFound.
func (m *MySQL) UpdateAdminUser(id uint64, u AdminUserUpdate) error {
	if id == 0 {
		return ErrUserNotFound
	}
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var curAdmin int
	if err := tx.QueryRow(`SELECT COALESCE(is_admin,0) FROM users WHERE id=? FOR UPDATE`, id).Scan(&curAdmin); err != nil {
		if err == sql.ErrNoRows {
			return ErrUserNotFound
		}
		return err
	}
	sets := []string{`nickname=?`}
	args := []any{u.Nickname}
	if u.IsAdmin != nil {
		if !*u.IsAdmin && curAdmin == 1 {
			if err := guardNotLastAdmin(tx); err != nil {
				return err
			}
		}
		sets = append(sets, `is_admin=?`)
		args = append(args, boolToInt(*u.IsAdmin))
	}
	if u.SetMembership {
		sets = append(sets, `is_member=?`)
		args = append(args, boolToInt(u.IsMember))
		if u.IsMember && u.MembershipExpiresAt != nil {
			sets = append(sets, `membership_expires_at=?`)
			args = append(args, *u.MembershipExpiresAt)
		} else {
			sets = append(sets, `membership_expires_at=NULL`)
		}
	}
	args = append(args, id)
	if _, err := tx.Exec(`UPDATE users SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteAdminUser removes a user and their personal library rows.
// Deleting the last admin returns ErrLastAdmin; a missing user returns ErrUserNotFound.
// Orders and comments are kept for accounting / thread integrity.
func (m *MySQL) DeleteAdminUser(id uint64) error {
	if id == 0 {
		return ErrUserNotFound
	}
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var curAdmin int
	if err := tx.QueryRow(`SELECT COALESCE(is_admin,0) FROM users WHERE id=? FOR UPDATE`, id).Scan(&curAdmin); err != nil {
		if err == sql.ErrNoRows {
			return ErrUserNotFound
		}
		return err
	}
	if curAdmin == 1 {
		if err := guardNotLastAdmin(tx); err != nil {
			return err
		}
	}
	for _, q := range []string{
		`DELETE FROM user_favorites WHERE user_id=?`,
		`DELETE FROM user_favorite_folders WHERE user_id=?`,
		`DELETE FROM user_watch_history WHERE user_id=?`,
		`DELETE FROM user_notifications WHERE user_id=?`,
		`UPDATE user_notifications SET actor_user_id=NULL WHERE actor_user_id=?`,
	} {
		if _, err := tx.Exec(q, id); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM users WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// guardNotLastAdmin returns ErrLastAdmin when at most one admin exists (the target).
func guardNotLastAdmin(tx *sql.Tx) error {
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM users WHERE COALESCE(is_admin,0)=1 FOR UPDATE`).Scan(&n); err != nil {
		return err
	}
	if n <= 1 {
		return ErrLastAdmin
	}
	return nil
}
