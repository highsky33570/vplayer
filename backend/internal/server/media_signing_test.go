package server

import (
	"testing"

	"github.com/tycdn/vplayer/internal/config"
)

func TestNewRejectsSigningWithoutSecret(t *testing.T) {
	_, err := New(config.Config{MediaSigningEnabled: true, MediaSigningSecret: "short", MediaURLTTLSeconds: 600})
	if err == nil {
		t.Fatal("expected startup failure")
	}
}
