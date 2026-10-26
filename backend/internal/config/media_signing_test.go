package config

import "testing"

func TestMediaURLTTLClamp(t *testing.T) {
	t.Setenv("MEDIA_URL_TTL_SECONDS", "")
	if got := Load().MediaURLTTLSeconds; got != 600 {
		t.Fatalf("default ttl %d", got)
	}
	t.Setenv("MEDIA_URL_TTL_SECONDS", "1")
	if got := Load().MediaURLTTLSeconds; got != 30 {
		t.Fatalf("min ttl %d", got)
	}
	t.Setenv("MEDIA_URL_TTL_SECONDS", "99999999")
	if got := Load().MediaURLTTLSeconds; got != 3600 {
		t.Fatalf("max ttl %d", got)
	}
}

func TestCheckMediaSigning(t *testing.T) {
	t.Setenv("MEDIA_SIGNING_ENABLED", "false")
	t.Setenv("MEDIA_SIGNING_SECRET", "")
	if err := Load().CheckMediaSigning(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEDIA_SIGNING_ENABLED", "true")
	t.Setenv("MEDIA_SIGNING_SECRET", "short")
	if err := Load().CheckMediaSigning(); err == nil {
		t.Fatal("short secret")
	}
	t.Setenv("MEDIA_SIGNING_SECRET", "0123456789abcdef")
	if err := Load().CheckMediaSigning(); err != nil {
		t.Fatal(err)
	}
}
