package config

import "testing"

func TestEmailVerificationEnabledDefaultsOff(t *testing.T) {
	t.Setenv("EMAIL_VERIFICATION_ENABLED", "")
	cfg := Load()
	if cfg.EmailVerificationEnabled {
		t.Fatal("unset EMAIL_VERIFICATION_ENABLED must be false")
	}
}

func TestEmailVerificationEnabledTrue(t *testing.T) {
	t.Setenv("EMAIL_VERIFICATION_ENABLED", "true")
	cfg := Load()
	if !cfg.EmailVerificationEnabled {
		t.Fatal("EMAIL_VERIFICATION_ENABLED=true must enable verification")
	}
}
