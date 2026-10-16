package auth_test

import (
	"testing"

	"github.com/tycdn/vplayer/internal/auth"
)

func TestGenerateVerificationCode_sixDigits(t *testing.T) {
	for i := 0; i < 20; i++ {
		code, err := auth.GenerateVerificationCode()
		if err != nil {
			t.Fatal(err)
		}
		if !auth.IsSixDigitCode(code) {
			t.Fatalf("code=%q", code)
		}
	}
}

func TestHashVerificationCode_roundTrip(t *testing.T) {
	pepper := "test-pepper"
	code := "042913"
	h := auth.HashVerificationCode(code, pepper)
	if h == code || h == "" {
		t.Fatal("hash must not be plaintext")
	}
	if !auth.VerifyVerificationCode(code, h, pepper) {
		t.Fatal("expected match")
	}
	if auth.VerifyVerificationCode("042914", h, pepper) {
		t.Fatal("wrong code must fail")
	}
	if auth.VerifyVerificationCode(code, h, "other") {
		t.Fatal("wrong pepper must fail")
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got := auth.NormalizeEmail("  Foo@Example.COM "); got != "foo@example.com" {
		t.Fatalf("%q", got)
	}
}
