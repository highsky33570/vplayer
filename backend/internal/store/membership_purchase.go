package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/tycdn/vplayer/internal/model"
)

// MembershipPlan is a sellable 大会员 package.
type MembershipPlan struct {
	ID           uint64
	Slug         string
	Name         string
	PriceUSD     float64
	DurationDays int
	Sort         int
}

// MembershipOrder is a crypto checkout row.
type MembershipOrder struct {
	ID                     uint64
	OrderNo                string
	UserID                 uint64
	PlanSlug               string
	PlanName               string
	DurationDays           int
	AmountUSD              float64
	Currency               string
	Status                 string
	GatewayProvider        string
	GatewayTradeID         string
	GatewayPaymentURL      string
	PayAddress             string
	AmountUSDT             float64
	GatewayToken           string
	GatewayNetwork         string
	GatewayStatus          string
	BlockTxID              string
	ExpireAt               time.Time
	PaidAt                 *time.Time
	MembershipGrantedUntil *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

const (
	MembershipOrderPending = "pending"
	MembershipOrderPaid    = "paid"
	MembershipOrderExpired = "expired"
	MembershipOrderCancel  = "cancelled"
)

// ListActiveMembershipPlans returns sellable plans ordered for the VIP UI.
func (m *MySQL) ListActiveMembershipPlans() ([]MembershipPlan, error) {
	rows, err := m.DB.Query(`
		SELECT id, slug, name, price_usd, duration_days, sort
		FROM membership_plans
		WHERE is_active=1
		ORDER BY sort ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MembershipPlan
	for rows.Next() {
		var p MembershipPlan
		if err := rows.Scan(&p.ID, &p.Slug, &p.Name, &p.PriceUSD, &p.DurationDays, &p.Sort); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetMembershipPlanBySlug loads one active plan.
func (m *MySQL) GetMembershipPlanBySlug(slug string) (*MembershipPlan, error) {
	slug = strings.TrimSpace(slug)
	var p MembershipPlan
	err := m.DB.QueryRow(`
		SELECT id, slug, name, price_usd, duration_days, sort
		FROM membership_plans
		WHERE slug=? AND is_active=1
		LIMIT 1`, slug).Scan(&p.ID, &p.Slug, &p.Name, &p.PriceUSD, &p.DurationDays, &p.Sort)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// InsertMembershipOrder creates a pending checkout row.
func (m *MySQL) InsertMembershipOrder(o MembershipOrder) (uint64, error) {
	res, err := m.DB.Exec(`
		INSERT INTO membership_orders (
		  order_no, user_id, plan_slug, plan_name, duration_days, amount_usd, currency, status,
		  gateway_provider, gateway_trade_id, gateway_payment_url, pay_address, amount_usdt,
		  gateway_token, gateway_network, gateway_status, expire_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		o.OrderNo, o.UserID, o.PlanSlug, o.PlanName, o.DurationDays, o.AmountUSD, o.Currency, o.Status,
		o.GatewayProvider, membershipNullStr(o.GatewayTradeID), membershipNullStr(o.GatewayPaymentURL), membershipNullStr(o.PayAddress),
		membershipNullFloat(o.AmountUSDT), membershipNullStr(o.GatewayToken), membershipNullStr(o.GatewayNetwork), membershipNullStr(o.GatewayStatus),
		o.ExpireAt,
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return uint64(id), err
}

// UpdateMembershipOrderGateway fills payment URL / address after EPUSDT create.
func (m *MySQL) UpdateMembershipOrderGateway(orderNo string, tradeID, paymentURL, address string, amountUSDT float64, token, network, gatewayStatus string) error {
	_, err := m.DB.Exec(`
		UPDATE membership_orders SET
		  gateway_trade_id=?, gateway_payment_url=?, pay_address=?, amount_usdt=?,
		  gateway_token=?, gateway_network=?, gateway_status=?, updated_at=NOW()
		WHERE order_no=?`,
		membershipNullStr(tradeID), membershipNullStr(paymentURL), membershipNullStr(address), membershipNullFloat(amountUSDT),
		membershipNullStr(token), membershipNullStr(network), membershipNullStr(gatewayStatus), orderNo,
	)
	return err
}

// GetMembershipOrderByNo loads an order by public order_no.
func (m *MySQL) GetMembershipOrderByNo(orderNo string) (*MembershipOrder, error) {
	return m.scanMembershipOrder(`SELECT `+membershipOrderCols+` FROM membership_orders WHERE order_no=? LIMIT 1`, orderNo)
}

// GetMembershipOrderByID loads by primary key.
func (m *MySQL) GetMembershipOrderByID(id uint64) (*MembershipOrder, error) {
	return m.scanMembershipOrder(`SELECT `+membershipOrderCols+` FROM membership_orders WHERE id=? LIMIT 1`, id)
}

const membershipOrderCols = `
  id, order_no, user_id, plan_slug, plan_name, duration_days, amount_usd, currency, status,
  gateway_provider, COALESCE(gateway_trade_id,''), COALESCE(gateway_payment_url,''), COALESCE(pay_address,''),
  COALESCE(amount_usdt,0), COALESCE(gateway_token,''), COALESCE(gateway_network,''), COALESCE(gateway_status,''),
  COALESCE(block_txid,''), expire_at, paid_at, membership_granted_until, created_at, updated_at`

func (m *MySQL) scanMembershipOrder(q string, args ...any) (*MembershipOrder, error) {
	var o MembershipOrder
	var paid, granted sql.NullTime
	err := m.DB.QueryRow(q, args...).Scan(
		&o.ID, &o.OrderNo, &o.UserID, &o.PlanSlug, &o.PlanName, &o.DurationDays, &o.AmountUSD, &o.Currency, &o.Status,
		&o.GatewayProvider, &o.GatewayTradeID, &o.GatewayPaymentURL, &o.PayAddress,
		&o.AmountUSDT, &o.GatewayToken, &o.GatewayNetwork, &o.GatewayStatus,
		&o.BlockTxID, &o.ExpireAt, &paid, &granted, &o.CreatedAt, &o.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if paid.Valid {
		t := paid.Time
		o.PaidAt = &t
	}
	if granted.Valid {
		t := granted.Time
		o.MembershipGrantedUntil = &t
	}
	return &o, nil
}

// ListPendingMembershipOrdersForReconcile returns pending orders newer than lookback.
func (m *MySQL) ListPendingMembershipOrdersForReconcile(lookback time.Duration, limit int) ([]MembershipOrder, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	since := time.Now().Add(-lookback)
	rows, err := m.DB.Query(`
		SELECT `+membershipOrderCols+`
		FROM membership_orders
		WHERE status=? AND created_at>=?
		ORDER BY id ASC
		LIMIT ?`, MembershipOrderPending, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMembershipOrderRows(rows)
}

func scanMembershipOrderRows(rows *sql.Rows) ([]MembershipOrder, error) {
	var out []MembershipOrder
	for rows.Next() {
		var o MembershipOrder
		var paid, granted sql.NullTime
		if err := rows.Scan(
			&o.ID, &o.OrderNo, &o.UserID, &o.PlanSlug, &o.PlanName, &o.DurationDays, &o.AmountUSD, &o.Currency, &o.Status,
			&o.GatewayProvider, &o.GatewayTradeID, &o.GatewayPaymentURL, &o.PayAddress,
			&o.AmountUSDT, &o.GatewayToken, &o.GatewayNetwork, &o.GatewayStatus,
			&o.BlockTxID, &o.ExpireAt, &paid, &granted, &o.CreatedAt, &o.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if paid.Valid {
			t := paid.Time
			o.PaidAt = &t
		}
		if granted.Valid {
			t := granted.Time
			o.MembershipGrantedUntil = &t
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ExpireStaleMembershipOrders marks pending past expire_at as expired.
func (m *MySQL) ExpireStaleMembershipOrders(now time.Time) (int64, error) {
	res, err := m.DB.Exec(`
		UPDATE membership_orders SET status=?, gateway_status='expired', updated_at=NOW()
		WHERE status=? AND expire_at<?`, MembershipOrderExpired, MembershipOrderPending, now)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SettleMembershipOrderPaid marks paid and extends the user's membership atomically.
// Returns the new membership_expires_at. Idempotent if already paid.
func (m *MySQL) SettleMembershipOrderPaid(orderNo, tradeID, address, token, blockTxID string, actualAmount float64, now time.Time) (time.Time, error) {
	tx, err := m.DB.Begin()
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var o MembershipOrder
	var paid, granted sql.NullTime
	err = tx.QueryRow(`
		SELECT id, order_no, user_id, plan_slug, plan_name, duration_days, amount_usd, currency, status,
		       COALESCE(gateway_trade_id,''), expire_at, paid_at, membership_granted_until
		FROM membership_orders WHERE order_no=? FOR UPDATE`, orderNo).Scan(
		&o.ID, &o.OrderNo, &o.UserID, &o.PlanSlug, &o.PlanName, &o.DurationDays, &o.AmountUSD, &o.Currency, &o.Status,
		&o.GatewayTradeID, &o.ExpireAt, &paid, &granted,
	)
	if err == sql.ErrNoRows {
		return time.Time{}, fmt.Errorf("order not found")
	}
	if err != nil {
		return time.Time{}, err
	}
	if o.Status == MembershipOrderPaid {
		if granted.Valid {
			return granted.Time, nil
		}
		return now, nil
	}
	if o.Status != MembershipOrderPending && o.Status != MembershipOrderExpired && o.Status != MembershipOrderCancel {
		return time.Time{}, fmt.Errorf("order not settleable: %s", o.Status)
	}

	var curExp sql.NullTime
	var isMember int
	if err := tx.QueryRow(`SELECT membership_expires_at, COALESCE(is_member,0) FROM users WHERE id=? FOR UPDATE`, o.UserID).
		Scan(&curExp, &isMember); err != nil {
		return time.Time{}, err
	}
	base := now
	if curExp.Valid && curExp.Time.After(base) {
		base = curExp.Time
	}
	until := base.Add(time.Duration(o.DurationDays) * 24 * time.Hour)

	if _, err := tx.Exec(`
		UPDATE users SET is_member=1, membership_expires_at=? WHERE id=?`, until, o.UserID); err != nil {
		return time.Time{}, err
	}
	if _, err := tx.Exec(`
		UPDATE membership_orders SET
		  status=?, gateway_trade_id=COALESCE(NULLIF(?,''), gateway_trade_id),
		  pay_address=COALESCE(NULLIF(?,''), pay_address),
		  gateway_token=COALESCE(NULLIF(?,''), gateway_token),
		  amount_usdt=?, block_txid=COALESCE(NULLIF(?,''), block_txid),
		  gateway_status='paid', paid_at=?, membership_granted_until=?, updated_at=NOW()
		WHERE id=?`,
		MembershipOrderPaid, tradeID, address, token, actualAmount, blockTxID, now, until, o.ID,
	); err != nil {
		return time.Time{}, err
	}
	if err := tx.Commit(); err != nil {
		return time.Time{}, err
	}
	return until, nil
}

// MarkMembershipOrderExpired sets a single pending order expired.
func (m *MySQL) MarkMembershipOrderExpired(orderNo string) error {
	_, err := m.DB.Exec(`
		UPDATE membership_orders SET status=?, gateway_status='expired', updated_at=NOW()
		WHERE order_no=? AND status=?`, MembershipOrderExpired, orderNo, MembershipOrderPending)
	return err
}

// GetUserMembership loads membership fields for auth responses.
func (m *MySQL) GetUserMembership(userID uint64) (isMember bool, expiresAt *time.Time, err error) {
	var flag int
	var exp sql.NullTime
	err = m.DB.QueryRow(`SELECT COALESCE(is_member,0), membership_expires_at FROM users WHERE id=?`, userID).
		Scan(&flag, &exp)
	if err != nil {
		return false, nil, err
	}
	if exp.Valid {
		t := exp.Time
		expiresAt = &t
	}
	return flag == 1, expiresAt, nil
}

func membershipNullStr(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func membershipNullFloat(f float64) any {
	if f <= 0 {
		return nil
	}
	return f
}

// EffectiveMember reports whether the user may access membership_required titles.
func EffectiveMember(u *model.User, now time.Time) bool {
	if u == nil {
		return false
	}
	if u.IsAdmin {
		return true
	}
	if u.MembershipExpiresAt != nil {
		return u.MembershipExpiresAt.After(now)
	}
	return u.IsMember
}

// MembershipDaysRemaining returns whole days left (ceil-ish via hour padding), or 0.
func MembershipDaysRemaining(expiresAt *time.Time, now time.Time) int {
	if expiresAt == nil || !expiresAt.After(now) {
		return 0
	}
	d := expiresAt.Sub(now)
	days := int(d.Hours() / 24)
	if d.Hours() > float64(days*24) {
		days++
	}
	if days < 1 {
		days = 1
	}
	return days
}
