package httpapi

import (
	"log/slog"
	"strings"
	"time"

	"github.com/tycdn/vplayer/internal/epusdt"
	"github.com/tycdn/vplayer/internal/store"
)

// StartMembershipReconcile periodically expires stale pending orders and, when
// EPUSDT_QUERY_ORDER_PATH is set, polls the gateway for missed paid notifies.
func (a *API) StartMembershipReconcile() {
	if a == nil || a.Store == nil {
		return
	}
	client := a.epusdtClient()
	if !client.Enabled() {
		return
	}
	every := time.Duration(a.Cfg.EpusdtReconcileMinutes) * time.Minute
	if every < time.Minute {
		every = 5 * time.Minute
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for range t.C {
			a.reconcileMembershipOrders(client)
		}
	}()
	slog.Info("membership payment reconcile enabled", "every_min", int(every.Minutes()))
}

func (a *API) reconcileMembershipOrders(client *epusdt.Client) {
	now := time.Now()
	if _, err := a.Store.ExpireStaleMembershipOrders(now); err != nil {
		slog.Warn("membership expire stale", "err", err)
	}
	if strings.TrimSpace(a.Cfg.EpusdtQueryOrderPath) == "" {
		return
	}
	rows, err := a.Store.ListPendingMembershipOrdersForReconcile(24*time.Hour, 40)
	if err != nil {
		slog.Warn("membership list pending", "err", err)
		return
	}
	for _, o := range rows {
		res, err := client.QueryTransaction(o.OrderNo)
		if err != nil {
			continue
		}
		// Query responses mirror create; treat presence of actual_amount + trade as paid hint
		// only when gateway_status style fields exist in raw.
		status := 0
		if raw := res.Raw; raw != nil {
			switch v := raw["status"].(type) {
			case float64:
				status = int(v)
			case string:
				if v == "2" || strings.EqualFold(v, "paid") {
					status = 2
				}
			}
		}
		if status != 2 && res.ActualAmount <= 0 {
			continue
		}
		if status == 2 || res.ActualAmount > 0 {
			_, _ = a.Store.SettleMembershipOrderPaid(
				o.OrderNo, res.TradeID, res.ReceiveAddress, res.Token, "", res.ActualAmount, now,
			)
		}
	}
}

// Ensure store import used when only types referenced via methods.
var _ = store.MembershipOrderPending
