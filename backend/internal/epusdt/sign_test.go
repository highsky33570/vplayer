package epusdt_test

import (
	"testing"

	"github.com/tycdn/vplayer/internal/epusdt"
)

func TestSignStable(t *testing.T) {
	params := map[string]string{
		"pid":        "1000",
		"order_id":   "TYTEST1",
		"amount":     "5.00",
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
		"pid": "1000", "order_id": "TYTEST1", "amount": "5.00", "currency": "usd",
		"notify_url": "https://example.com/notify", "signature": a,
	}) {
		t.Fatal("verify failed")
	}
	if epusdt.VerifySignature("secret-token", "1000", map[string]string{
		"pid": "1000", "order_id": "TYTEST1", "amount": "5.00", "currency": "usd",
		"notify_url": "https://example.com/notify", "signature": "deadbeef",
	}) {
		t.Fatal("bad signature accepted")
	}
}
