package httpapi

import (
	"testing"
	"time"

	"github.com/tycdn/vplayer/internal/store"
)

func TestPaymentDeliveryStatus(t *testing.T) {
	until := time.Now().Add(24 * time.Hour)
	cases := []struct {
		name string
		o    store.MembershipOrder
		want string
	}{
		{
			name: "paid_granted",
			o:    store.MembershipOrder{Status: store.MembershipOrderPaid, MembershipGrantedUntil: &until},
			want: "success",
		},
		{
			name: "paid_no_grant",
			o:    store.MembershipOrder{Status: store.MembershipOrderPaid},
			want: "fail",
		},
		{
			name: "pending",
			o:    store.MembershipOrder{Status: store.MembershipOrderPending},
			want: "pending",
		},
		{
			name: "expired",
			o:    store.MembershipOrder{Status: store.MembershipOrderExpired},
			want: "none",
		},
		{
			name: "cancelled",
			o:    store.MembershipOrder{Status: store.MembershipOrderCancel},
			want: "none",
		},
	}
	for _, tc := range cases {
		if got := paymentDeliveryStatus(tc.o); got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}
