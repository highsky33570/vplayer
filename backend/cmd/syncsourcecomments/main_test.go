package main

import (
	"strings"
	"testing"

	"github.com/tycdn/vplayer/internal/sourcecomments"
)

func TestParseArgsSelectsNoVideoByDefault(t *testing.T) {
	_, err := parseArgs(nil)
	if err == nil || !strings.Contains(err.Error(), "--video-id") {
		t.Fatalf("got %v", err)
	}
}

func TestFullModeWithoutConfirmRefuses(t *testing.T) {
	_, err := parseArgs([]string{
		"--video-id", "42",
		"--source-system", "olehdtv",
		"--source-video-id", "82861",
		"--mode", "full",
	})
	if err == nil || !strings.Contains(err.Error(), "--confirm=FULL") {
		t.Fatalf("got %v", err)
	}
}

func TestFullModeConfirmIsExact(t *testing.T) {
	_, err := parseArgs([]string{
		"--video-id", "42",
		"--source-system", "olehdtv",
		"--source-video-id", "82861",
		"--mode", "full",
		"--confirm", "full",
	})
	if err == nil {
		t.Fatal("lowercase confirm was accepted")
	}
	opts, err := parseArgs([]string{
		"--video-id", "42",
		"--source-system", "olehdtv",
		"--source-video-id", "82861",
		"--mode", "full",
		"--confirm", "FULL",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.VideoID != 42 || opts.SourceVideoID != "82861" || opts.Mode != sourcecomments.ModeFull {
		t.Fatalf("%+v", opts)
	}
}

func TestIncrementalIsOneExplicitVideo(t *testing.T) {
	opts, err := parseArgs([]string{
		"--video-id", "7",
		"--source-system", "olehdtv",
		"--source-video-id", "82861",
		"--mode", "incremental",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Mode != sourcecomments.ModeIncremental || opts.SourceSystem != "olehdtv" {
		t.Fatalf("%+v", opts)
	}
}

func TestRejectsCatalogWideArguments(t *testing.T) {
	cases := [][]string{
		{"--all"},
		{"--video-id", "1", "--source-system", "olehdtv", "--source-video-id", "1,2", "--mode", "incremental"},
		{"--video-id", "1", "--source-system", "other", "--source-video-id", "1", "--mode", "incremental"},
		{"--video-id", "1", "--source-system", "olehdtv", "--source-video-id", "1", "--mode", "incremental", "extra"},
		{"--video-id", "0", "--source-system", "olehdtv", "--source-video-id", "1", "--mode", "incremental"},
	}
	for _, args := range cases {
		if _, err := parseArgs(args); err == nil {
			t.Fatalf("accepted %q", args)
		}
	}
}
