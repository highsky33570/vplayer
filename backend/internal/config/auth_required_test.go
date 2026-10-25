package config

import "testing"

func TestLoadAuthRequired(t *testing.T) {
	t.Setenv("AUTH_REQUIRED", "")
	if !Load().AuthRequired {
		t.Fatal("AUTH_REQUIRED defaults to true")
	}
	t.Setenv("AUTH_REQUIRED", "false")
	if Load().AuthRequired {
		t.Fatal("AUTH_REQUIRED=false")
	}
	t.Setenv("AUTH_REQUIRED", "true")
	if !Load().AuthRequired {
		t.Fatal("AUTH_REQUIRED=true")
	}
}

func TestLoadPlaybackAuthRequired(t *testing.T) {
	t.Setenv("PLAYBACK_AUTH_REQUIRED", "")
	if !Load().PlaybackAuthRequired {
		t.Fatal("PLAYBACK_AUTH_REQUIRED defaults to true")
	}
	t.Setenv("PLAYBACK_AUTH_REQUIRED", "false")
	if Load().PlaybackAuthRequired {
		t.Fatal("PLAYBACK_AUTH_REQUIRED=false")
	}
	t.Setenv("AUTH_REQUIRED", "true")
	cfg := Load()
	if !cfg.AuthRequired || cfg.PlaybackAuthRequired {
		t.Fatalf("production target auth=%v playback=%v", cfg.AuthRequired, cfg.PlaybackAuthRequired)
	}
}
