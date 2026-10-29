package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/store"
)

type adminPaymentListQuery struct {
	Limit  int
	Offset int
	Query  string
	Status string
}

type AdminPaymentDTO struct {
	ID                     uint64     `json:"id"`
	OrderNo                string     `json:"order_no"`
	UserID                 uint64     `json:"user_id"`
	UserEmail              string     `json:"user_email"`
	UserNickname           string     `json:"user_nickname"`
	PlanSlug               string     `json:"plan_slug"`
	PlanName               string     `json:"plan_name"`
	DurationDays           int        `json:"duration_days"`
	AmountUSD              float64    `json:"amount_usd"`
	AmountUSDT             float64    `json:"amount_usdt"`
	Currency               string     `json:"currency"`
	Status                 string     `json:"status"`
	GatewayProvider        string     `json:"gateway_provider"`
	GatewayTradeID         string     `json:"gateway_trade_id"`
	PayAddress             string     `json:"pay_address"`
	GatewayNetwork         string     `json:"gateway_network"`
	BlockTxID              string     `json:"block_txid"`
	ExpireAt               time.Time  `json:"expire_at"`
	PaidAt                 *time.Time `json:"paid_at,omitempty"`
	MembershipGrantedUntil *time.Time `json:"membership_granted_until,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
}

func adminPaymentDTO(row store.AdminMembershipOrderRow) AdminPaymentDTO {
	o := row.MembershipOrder
	return AdminPaymentDTO{
		ID:                     o.ID,
		OrderNo:                o.OrderNo,
		UserID:                 o.UserID,
		UserEmail:              row.UserEmail,
		UserNickname:           row.UserNickname,
		PlanSlug:               o.PlanSlug,
		PlanName:               o.PlanName,
		DurationDays:           o.DurationDays,
		AmountUSD:              o.AmountUSD,
		AmountUSDT:             o.AmountUSDT,
		Currency:               o.Currency,
		Status:                 o.Status,
		GatewayProvider:        o.GatewayProvider,
		GatewayTradeID:         o.GatewayTradeID,
		PayAddress:             o.PayAddress,
		GatewayNetwork:         o.GatewayNetwork,
		BlockTxID:              o.BlockTxID,
		ExpireAt:               o.ExpireAt,
		PaidAt:                 o.PaidAt,
		MembershipGrantedUntil: o.MembershipGrantedUntil,
		CreatedAt:              o.CreatedAt,
	}
}

func parseAdminPaymentListQuery(c *gin.Context) (adminPaymentListQuery, string, bool) {
	out := adminPaymentListQuery{Limit: 24, Offset: 0}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return out, "invalid limit", false
		}
		out.Limit = n
	}
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return out, "invalid offset", false
		}
		out.Offset = n
	}
	out.Query = strings.TrimSpace(c.Query("q"))
	out.Status = strings.TrimSpace(c.Query("status"))
	switch out.Status {
	case "", store.MembershipOrderPending, store.MembershipOrderPaid, store.MembershipOrderExpired, store.MembershipOrderCancel:
	default:
		return out, "invalid status", false
	}
	return out, "", true
}

// AdminListPayments GET /api/v1/admin/payments
func (a *API) AdminListPayments(c *gin.Context) {
	if a.Store == nil {
		c.JSON(http.StatusOK, gin.H{
			"ok": true, "data": []AdminPaymentDTO{},
			"total": 0, "limit": 24, "offset": 0, "has_more": false,
		})
		return
	}
	q, msg, ok := parseAdminPaymentListQuery(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": msg})
		return
	}
	total, err := a.Store.CountAdminMembershipOrders(q.Status, q.Query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法统计支付记录"})
		return
	}
	rows, err := a.Store.ListAdminMembershipOrders(q.Status, q.Query, q.Limit, q.Offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "无法加载支付记录"})
		return
	}
	out := make([]AdminPaymentDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, adminPaymentDTO(row))
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":       true,
		"data":     out,
		"total":    total,
		"limit":    q.Limit,
		"offset":   q.Offset,
		"has_more": q.Offset+len(out) < total,
	})
}
