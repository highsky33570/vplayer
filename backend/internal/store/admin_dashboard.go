package store

import (
	"database/sql"
	"strings"
	"time"
)

// AdminSyncRunSummary is a read-only snapshot of the newest sync_runs row.
type AdminSyncRunSummary struct {
	ID               uint64     `json:"id"`
	SourceSystem     string     `json:"source_system"`
	TriggerType      string     `json:"trigger_type"`
	StartedAt        time.Time  `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	Scanned          int        `json:"scanned"`
	CreatedCount     int        `json:"created_count"`
	UpdatedCount     int        `json:"updated_count"`
	UnchangedCount   int        `json:"unchanged_count"`
	FailedCount      int        `json:"failed_count"`
	DeactivatedCount int        `json:"deactivated_count"`
	// HasError is true when sync_runs.error_text is non-empty.
	// The raw error text is never carried on this struct (dashboard must not expose it).
	HasError bool `json:"has_error"`
}

// AdminDashboardStats is lightweight operational counts for the admin home.
type AdminDashboardStats struct {
	UsersTotal      int `json:"users_total"`
	VideosTotal     int `json:"videos_total"`
	VideosActive    int `json:"videos_active"`
	CategoriesTotal int `json:"categories_total"`
	// MembersTotal counts effective members: unexpired expires_at, or lifetime is_member with no expiry.
	MembersTotal     int                  `json:"members_total"`
	VideosMembership int                  `json:"videos_membership"`
	OrdersPaidTotal  int                  `json:"orders_paid_total"`
	RevenuePaidUSD   float64              `json:"revenue_paid_usd"`
	RevenuePaidUSDT  float64              `json:"revenue_paid_usdt"`
	LatestSync       *AdminSyncRunSummary `json:"latest_sync"`
}

func (m *MySQL) AdminDashboardStats() (*AdminDashboardStats, error) {
	out := &AdminDashboardStats{}
	if err := m.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&out.UsersTotal); err != nil {
		return nil, err
	}
	if err := m.DB.QueryRow(`SELECT COUNT(*) FROM videos`).Scan(&out.VideosTotal); err != nil {
		return nil, err
	}
	if err := m.DB.QueryRow(`SELECT COUNT(*) FROM videos WHERE COALESCE(is_active,1)=1`).Scan(&out.VideosActive); err != nil {
		return nil, err
	}
	if err := m.DB.QueryRow(`SELECT COUNT(*) FROM categories`).Scan(&out.CategoriesTotal); err != nil {
		return nil, err
	}
	if err := m.DB.QueryRow(`
		SELECT COUNT(*) FROM users
		WHERE (membership_expires_at IS NOT NULL AND membership_expires_at > ?)
		   OR (membership_expires_at IS NULL AND COALESCE(is_member,0)=1)`, time.Now()).Scan(&out.MembersTotal); err != nil {
		return nil, err
	}
	if err := m.DB.QueryRow(`SELECT COUNT(*) FROM videos WHERE COALESCE(membership_required,0)=1`).Scan(&out.VideosMembership); err != nil {
		return nil, err
	}
	if err := m.DB.QueryRow(`SELECT COUNT(*) FROM membership_orders WHERE status=?`, MembershipOrderPaid).Scan(&out.OrdersPaidTotal); err != nil {
		return nil, err
	}
	usd, usdt, err := m.SumPaidMembershipRevenue()
	if err != nil {
		return nil, err
	}
	out.RevenuePaidUSD = usd
	out.RevenuePaidUSDT = usdt
	sync, err := m.latestSyncRunSummary()
	if err != nil {
		return nil, err
	}
	out.LatestSync = sync
	return out, nil
}

func (m *MySQL) latestSyncRunSummary() (*AdminSyncRunSummary, error) {
	var s AdminSyncRunSummary
	var finished sql.NullTime
	var errText sql.NullString
	err := m.DB.QueryRow(`
		SELECT id, source_system, trigger_type, started_at, finished_at,
		       scanned, created_count, updated_count, unchanged_count, failed_count, deactivated_count,
		       error_text
		FROM sync_runs
		ORDER BY started_at DESC, id DESC
		LIMIT 1`).Scan(
		&s.ID, &s.SourceSystem, &s.TriggerType, &s.StartedAt, &finished,
		&s.Scanned, &s.CreatedCount, &s.UpdatedCount, &s.UnchangedCount, &s.FailedCount, &s.DeactivatedCount,
		&errText,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if finished.Valid {
		t := finished.Time
		s.FinishedAt = &t
	}
	// Derive has_error from DB error_text without retaining the raw message.
	if errText.Valid && strings.TrimSpace(errText.String) != "" {
		s.HasError = true
	}
	return &s, nil
}
