package epusdt_test

import (
	"testing"

	"github.com/tycdn/vplayer/internal/epusdt"
)

func TestSignStable(t *testing.T) {
	params := map[string]string{
		"pid":        "1000",
		"order_id":   "TYTEST1",
		"amount":     epusdt.FormatAmountCanonical(5),
		"currency":   "usd",
		"notify_url": "https://example.com/notify",
		"signature":  "ignored",
	}
	a := epusdt.Sign("secret-token", params)
	b := epusdt.Sign("secret-token", params)
	if a == "" || a != b {
		t.Fatalf("unstable signature %q vs %q", a, b)
	}
	if !epusdt.VerifySignature("secret-token", "1000", map[string]string{
		"pid": "1000", "order_id": "TYTEST1", "amount": "5", "currency": "usd",
		"notify_url": "https://example.com/notify", "signature": a,
	}) {
		t.Fatal("verify failed")
	}
	if epusdt.VerifySignature("secret-token", "1000", map[string]string{
		"pid": "1000", "order_id": "TYTEST1", "amount": "5", "currency": "usd",
		"notify_url": "https://example.com/notify", "signature": "deadbeef",
	}) {
		t.Fatal("bad signature accepted")
	}
}

// Vendor vector from GMPay wiki/API.md (also pinned in CDN EpusdtSignatureTest).
func TestSignVendorVector(t *testing.T) {
	params := map[string]string{
		"pid":          "1000",
		"order_id":     "ORD202605230001",
		"currency":     "cny",
		"token":        "usdt",
		"network":      "tron",
		"amount":       epusdt.FormatAmountCanonical(100),
		"notify_url":   "https://merchant.example/notify",
		"redirect_url": "https://merchant.example/return",
		"name":         "VIP",
	}
	got := epusdt.Sign("epusdt_secret_key", params)
	const want = "6f874b1919d95081835e2809b620e354a5866f5a6dbb2e432d1627f1eb10059d"
	if got != want {
		t.Fatalf("vendor vector mismatch\ngot  %s\nwant %s\namount=%q", got, want, params["amount"])
	}
}

func TestFormatAmountCanonical(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{5, "5"},
		{5.1, "5.1"},
		{14, "14"},
		{26.5, "26.5"},
		{5.00, "5"},
	}
	for _, tc := range cases {
		if got := epusdt.FormatAmountCanonical(tc.in); got != tc.want {
			t.Fatalf("FormatAmountCanonical(%v)=%q want %q", tc.in, got, tc.want)
		}
	}
}
