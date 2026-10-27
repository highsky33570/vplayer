package store

import (
	"database/sql"
	"strings"
	"time"
)

// AdminUser is a read-only admin view of a user. password_hash is never selected.
type AdminUser struct {
	ID        uint64
	Email     string
	Nickname  string
	IsAdmin   bool
	IsMember  bool
	CreatedAt time.Time
}

const adminUserSelect = `
		SELECT id, email, COALESCE(nickname,''), COALESCE(is_admin,0), COALESCE(is_member,0), created_at
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
	q += ` ORDER BY id DESC LIMIT ? OFFSET ?`
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
	return out, rows.Err()
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
	return u, nil
}

type adminUserScannable interface {
	Scan(dest ...any) error
}

func scanAdminUser(row adminUserScannable) (*AdminUser, error) {
	var u AdminUser
	var isAdmin, isMember int
	err := row.Scan(&u.ID, &u.Email, &u.Nickname, &isAdmin, &isMember, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	u.IsAdmin = isAdmin == 1
	u.IsMember = isMember == 1
	return &u, nil
}
