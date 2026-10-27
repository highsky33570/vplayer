package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/epusdt"
	"github.com/tycdn/vplayer/internal/store"
)

func (a *API) epusdtClient() *epusdt.Client {
	if a.Epusdt != nil {
		return a.Epusdt
	}
	return epusdt.NewClient(epusdt.Config{
		BaseURL:         a.Cfg.EpusdtBaseURL,
		CreateOrderPath: a.Cfg.EpusdtCreateOrderPath,
		QueryOrderPath:  a.Cfg.EpusdtQueryOrderPath,
		PID:             a.Cfg.EpusdtPID,
		APIToken:        a.Cfg.EpusdtAPIToken,
		NotifyURL:       a.Cfg.EpusdtNotifyURL,
		RedirectURL:     a.Cfg.EpusdtRedirectURL,
		DefaultCurrency: a.Cfg.EpusdtDefaultCurrency,
		DefaultToken:    a.Cfg.EpusdtDefaultToken,
		DefaultNetwork:  a.Cfg.EpusdtDefaultNetwork,
	})
}

// ListMembershipPlans handles GET /api/v1/membership/plans (public).
func (a *API) ListMembershipPlans(c *gin.Context) {
	if a.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "服务暂不可用"})
		return
	}
	plans, err := a.Store.ListActiveMembershipPlans()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "加载套餐失败"})
		return
	}
	out := make([]gin.H, 0, len(plans))
	for _, p := range plans {
		out = append(out, gin.H{
			"slug":          p.Slug,
			"name":          p.Name,
			"price_usd":     p.PriceUSD,
			"duration_days": p.DurationDays,
			"sort":          p.Sort,
		})
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": out})
}

// CreateMembershipOrder handles POST /api/v1/membership/orders (auth).
func (a *API) CreateMembershipOrder(c *gin.Context) {
	uid, ok := authUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "未登录"})
		return
	}
	if a.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "服务暂不可用"})
		return
	}
	client := a.epusdtClient()
	if !client.Enabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "支付通道暂未配置"})
		return
	}
	var body struct {
		PlanSlug string `json:"plan_slug"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.PlanSlug) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请选择套餐"})
		return
	}
	plan, err := a.Store.GetMembershipPlanBySlug(body.PlanSlug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "加载套餐失败"})
		return
	}
	if plan == nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "套餐不存在"})
		return
	}
	orderNo, err := newMembershipOrderNo()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "创建订单失败"})
		return
	}
	ttlMin := a.Cfg.EpusdtOrderTTLMinutes
	if ttlMin <= 0 {
		ttlMin = 30
	}
	now := time.Now()
	expireAt := now.Add(time.Duration(ttlMin) * time.Minute)
	o := store.MembershipOrder{
		OrderNo:         orderNo,
		UserID:          uid,
		PlanSlug:        plan.Slug,
		PlanName:        plan.Name,
		DurationDays:    plan.DurationDays,
		AmountUSD:       plan.PriceUSD,
		Currency:        "usd",
		Status:          store.MembershipOrderPending,
		GatewayProvider: "epusdt",
		ExpireAt:        expireAt,
	}
	if _, err := a.Store.InsertMembershipOrder(o); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "创建订单失败"})
		return
	}
	redirect := strings.TrimSpace(a.Cfg.EpusdtRedirectURL)
	if redirect != "" {
		if strings.Contains(redirect, "?") {
			redirect = redirect + "&order=" + orderNo
		} else {
			redirect = redirect + "?order=" + orderNo
		}
	}
	created, err := client.CreateTransaction(epusdt.CreateParams{
		OrderID:     orderNo,
		AmountUSD:   plan.PriceUSD,
		Currency:    "usd",
		RedirectURL: redirect,
	})
	if err != nil {
		log.Printf("membership order %s epusdt create failed (base=%s): %v", orderNo, a.Cfg.EpusdtBaseURL, err)
		_ = a.Store.MarkMembershipOrderExpired(orderNo)
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "message": "支付网关暂不可用，请稍后重试"})
		return
	}
	_ = a.Store.UpdateMembershipOrderGateway(orderNo, created.TradeID, created.PaymentURL, created.ReceiveAddress,
		created.ActualAmount, created.Token, created.Network, "pending")
	c.JSON(http.StatusCreated, gin.H{
		"ok": true,
		"data": gin.H{
			"order_no":         orderNo,
			"plan_slug":        plan.Slug,
			"plan_name":        plan.Name,
			"amount_usd":       plan.PriceUSD,
			"duration_days":    plan.DurationDays,
			"status":           store.MembershipOrderPending,
			"payment_url":      created.PaymentURL,
			"receive_address":  created.ReceiveAddress,
			"actual_amount":    created.ActualAmount,
			"token":            created.Token,
			"network":          created.Network,
			"expire_at":        expireAt.UTC().Format(time.RFC3339),
			"gateway_provider": "epusdt",
		},
	})
}

func newMembershipOrderNo() (string, error) {
	var b [10]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "VP" + strings.ToUpper(hex.EncodeToString(b[:])), nil
}

// GetMembershipOrder handles GET /api/v1/membership/orders/:orderNo (owner only).
func (a *API) GetMembershipOrder(c *gin.Context) {
	uid, ok := authUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "未登录"})
		return
	}
	if a.Store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "服务暂不可用"})
		return
	}
	orderNo := strings.TrimSpace(c.Param("orderNo"))
	o, err := a.Store.GetMembershipOrderByNo(orderNo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "查询失败"})
		return
	}
	if o == nil || o.UserID != uid {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "订单不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": membershipOrderJSON(o)})
}

func membershipOrderJSON(o *store.MembershipOrder) gin.H {
	out := gin.H{
		"order_no":         o.OrderNo,
		"plan_slug":        o.PlanSlug,
		"plan_name":        o.PlanName,
		"amount_usd":       o.AmountUSD,
		"duration_days":    o.DurationDays,
		"status":           o.Status,
		"payment_url":      o.GatewayPaymentURL,
		"receive_address":  o.PayAddress,
		"actual_amount":    o.AmountUSDT,
		"token":            o.GatewayToken,
		"network":          o.GatewayNetwork,
		"expire_at":        o.ExpireAt.UTC().Format(time.RFC3339),
		"gateway_provider": o.GatewayProvider,
	}
	if o.PaidAt != nil {
		out["paid_at"] = o.PaidAt.UTC().Format(time.RFC3339)
	}
	if o.MembershipGrantedUntil != nil {
		out["membership_granted_until"] = o.MembershipGrantedUntil.UTC().Format(time.RFC3339)
	}
	return out
}

// EpusdtNotify handles POST /api/v1/payments/epusdt/notify (unsigned public, HMAC body).
func (a *API) EpusdtNotify(c *gin.Context) {
	client := a.epusdtClient()
	if !client.Enabled() {
		c.Data(http.StatusServiceUnavailable, "text/plain", []byte("not configured"))
		return
	}
	var raw map[string]any
	if err := c.ShouldBindJSON(&raw); err != nil {
		raw = map[string]any{}
		_ = c.Request.ParseForm()
		for k, vs := range c.Request.Form {
			if len(vs) > 0 {
				raw[k] = vs[0]
			}
		}
	}
	params := map[string]string{}
	for k, v := range raw {
		params[k] = fmt.Sprint(v)
	}
	if !client.VerifyNotify(params) {
		c.Data(http.StatusUnauthorized, "text/plain", []byte("signature error"))
		return
	}
	orderID := strings.TrimSpace(params["order_id"])
	statusRaw := strings.TrimSpace(params["status"])
	status, _ := strconv.Atoi(statusRaw)
	if a.Store == nil {
		c.Data(http.StatusServiceUnavailable, "text/plain", []byte("store unavailable"))
		return
	}
	o, err := a.Store.GetMembershipOrderByNo(orderID)
	if err != nil || o == nil {
		c.Data(http.StatusNotFound, "text/plain", []byte("order not found"))
		return
	}
	if mismatch := epusdtOrderMismatch(o, params); mismatch != "" {
		c.Data(http.StatusUnprocessableEntity, "text/plain", []byte(mismatch))
		return
	}
	switch status {
	case 3:
		_ = a.Store.MarkMembershipOrderExpired(orderID)
		c.Data(http.StatusOK, "text/plain", []byte("ok"))
		return
	case 2:
		actual, _ := epusdt.ParseFloatAmount(params["actual_amount"])
		_, err := a.Store.SettleMembershipOrderPaid(
			orderID,
			strings.TrimSpace(params["trade_id"]),
			strings.TrimSpace(params["receive_address"]),
			strings.TrimSpace(params["token"]),
			strings.TrimSpace(params["block_transaction_id"]),
			actual,
			time.Now(),
		)
		if err != nil {
			c.Data(http.StatusUnprocessableEntity, "text/plain", []byte(err.Error()))
			return
		}
		c.Data(http.StatusOK, "text/plain", []byte("ok"))
		return
	default:
		c.Data(http.StatusOK, "text/plain", []byte("ok"))
	}
}

func epusdtOrderMismatch(o *store.MembershipOrder, params map[string]string) string {
	amount, err := epusdt.ParseFloatAmount(params["amount"])
	if err != nil {
		return "amount mismatch"
	}
	// Allow small float noise; order amount is fiat quote.
	if abs(amount-o.AmountUSD) > 0.05 {
		return "amount mismatch"
	}
	if trade := strings.TrimSpace(params["trade_id"]); o.GatewayTradeID != "" && trade != "" && trade != o.GatewayTradeID {
		return "trade_id mismatch"
	}
	return ""
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
