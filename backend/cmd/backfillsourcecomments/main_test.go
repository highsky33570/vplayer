package main

import (
	"context"
	"strings"
	"testing"
)

func TestParseArgsRefusesWithoutConfirmationOrLimit(t *testing.T) {
	cases := [][]string{
		nil,
		{"--source-system", "olehdtv", "--mode", "full", "--limit", "10"},
		{"--source-system", "olehdtv", "--mode", "full", "--confirm", "BACKFILL"},
		{"--source-system", "olehdtv", "--mode", "full", "--confirm", "backfill", "--limit", "10"},
		{"--source-system", "olehdtv", "--mode", "full", "--confirm", "BACKFILL", "--limit", "0"},
		{"--source-system", "olehdtv", "--mode", "full", "--confirm", "BACKFILL", "--limit", "-3"},
		{"--source-system", "olehdtv", "--mode", "full", "--confirm", "BACKFILL", "--limit", "101"},
		{"--source-system", "other", "--mode", "full", "--confirm", "BACKFILL", "--limit", "1"},
		{"--source-system", "olehdtv", "--mode", "incremental", "--confirm", "BACKFILL", "--limit", "1"},
		{"--all"},
	}
	for _, args := range cases {
		if _, err := parseArgs(args); err == nil {
			t.Fatalf("accepted %q", args)
		}
	}
}

func TestParseArgsAcceptsBoundedFullBackfill(t *testing.T) {
	opts, err := parseArgs([]string{
		"--source-system", "olehdtv",
		"--mode", "full",
		"--limit", "10",
		"--confirm", "BACKFILL",
		"--dry-run",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Limit != 10 || !opts.DryRun || opts.RetryFailed || opts.SourceSystem != "olehdtv" {
		t.Fatalf("%+v", opts)
	}
	retry, err := parseArgs([]string{
		"--source-system", "olehdtv",
		"--mode", "full",
		"--limit", "100",
		"--confirm", "BACKFILL",
		"--retry-failed",
	})
	if err != nil || !retry.RetryFailed || retry.Limit != 100 {
		t.Fatalf("%+v %v", retry, err)
	}
}

func TestReadOnlyFetcherRefusesBeforeAnyRequest(t *testing.T) {
	_, err := (readOnlyFetcher{}).FetchCommentPage(context.Background(), "../savedata", 1)
	if err == nil || !strings.Contains(err.Error(), "invalid source video id") {
		t.Fatalf("err=%v", err)
	}
}
