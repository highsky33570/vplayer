package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/tycdn/vplayer/internal/catalogsync"
	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
	"github.com/tycdn/vplayer/internal/store"
)

func main() {
	defaults := catalogsync.DefaultLeafImportOptions()
	dryRun := flag.Bool("dry-run", false, "scan + plan only; zero DB writes and zero detail GETs")
	limitLeaves := flag.Int("limit-leaves", 0, "max leaf categories to scan (0 = all eligible under parents 1–6; hard-capped)")
	maxPages := flag.Int("max-pages-per-leaf", defaults.MaxPagesPerLeaf, "max listing pages per leaf")
	maxNewPerLeaf := flag.Int("max-new-per-leaf", defaults.MaxNewPerLeaf, "max new video imports per leaf")
	maxNewTotal := flag.Int("max-new-total", defaults.MaxNewTotal, "max new video imports per run")
	maxRequests := flag.Int("max-requests", defaults.MaxRequests,
		"LOGICAL max listing+detail Fetch calls per run (not a hard network-attempt ceiling; HTTP retries may multiply wire GETs)")
	flag.Parse()

	opt := catalogsync.LeafImportOptions{
		DryRun:          *dryRun,
		LimitLeaves:     *limitLeaves,
		MaxPagesPerLeaf: *maxPages,
		MaxNewPerLeaf:   *maxNewPerLeaf,
		MaxNewTotal:     *maxNewTotal,
		MaxRequests:     *maxRequests,
	}
	if err := catalogsync.ValidateLeafImportOptions(&opt); err != nil {
		log.Fatal(err)
	}

	cfg := config.Load()
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.OleHdTvPublicBaseURL), "/")
	if baseURL == "" {
		log.Fatal("OLEHDTV_PUBLIC_BASE_URL is required for leaf import")
	}

	delayMs, _, retries := maccms.ClampRecentHTTPSettings(cfg.OleHdTvRequestDelayMs, 1, cfg.OleHdTvMaxRetries)
	timeout := time.Duration(cfg.OleHdTvRequestTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	// Concurrency fixed at 1 for listing and detail fetches.
	fetcher := maccms.NewHTTPFetcher(timeout, delayMs, retries, "VPlayerLeafImport/1.0")

	db, err := store.OpenMySQL(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("mysql: %v", err)
	}
	defer db.DB.Close()

	svc := catalogsync.NewLeafImportSync(db, fetcher, baseURL)
	svc.HTTPRetries = retries
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	fmt.Fprintf(os.Stderr, "syncleafimport dry_run=%v limit_leaves=%d max_pages=%d max_new_per_leaf=%d max_new_total=%d max_requests=%d (logical Fetch budget; wire attempts may be up to max_requests*%d with retries) delay_ms=%d http_retries=%d concurrency=1 base=%s\n",
		opt.DryRun, opt.LimitLeaves, opt.MaxPagesPerLeaf, opt.MaxNewPerLeaf, opt.MaxNewTotal, opt.MaxRequests, retries, delayMs, retries, baseURL)

	sum, runErr := svc.Run(ctx, opt)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(sum)

	if errors.Is(runErr, catalogsync.ErrLeafArchitectureReview) || sum.NeedsArchitectureReview {
		fmt.Fprintf(os.Stderr, "syncleafimport: needs architecture review (multi-leaf membership) — not safe for expanded rollout\n")
		os.Exit(2)
	}
	if runErr != nil {
		fmt.Fprintf(os.Stderr, "syncleafimport: %v\n", runErr)
		os.Exit(1)
	}
	if opt.DryRun {
		fmt.Fprintf(os.Stderr, "dry-run complete — no DB writes, no detail requests\n")
	}
}
