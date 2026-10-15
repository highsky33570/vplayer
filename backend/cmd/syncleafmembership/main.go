package main

import (
	"context"
	"encoding/json"
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
	dryRun := flag.Bool("dry-run", false, "inspect leaf listings and report assignments; zero DB updates")
	limitLeaves := flag.Int("limit-leaves", 0, "max leaf categories to scan (0 = all eligible under parents 1–6; hard-capped)")
	flag.Parse()

	if *limitLeaves < 0 {
		log.Fatal("--limit-leaves must be >= 0")
	}

	cfg := config.Load()
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.OleHdTvPublicBaseURL), "/")
	if baseURL == "" {
		log.Fatal("OLEHDTV_PUBLIC_BASE_URL is required for leaf membership sync")
	}

	delayMs, _, retries := maccms.ClampRecentHTTPSettings(cfg.OleHdTvRequestDelayMs, 1, cfg.OleHdTvMaxRetries)
	timeout := time.Duration(cfg.OleHdTvRequestTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	// Concurrency fixed at 1 for this phase (safe bound).
	fetcher := maccms.NewHTTPFetcher(timeout, delayMs, retries, "VPlayerLeafMembership/1.0")

	db, err := store.OpenMySQL(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("mysql: %v", err)
	}
	defer db.DB.Close()

	svc := catalogsync.NewLeafMembershipSync(db, fetcher, baseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	fmt.Fprintf(os.Stderr, "syncleafmembership dry_run=%v limit_leaves=%d delay_ms=%d concurrency=1 base=%s\n",
		*dryRun, *limitLeaves, delayMs, baseURL)

	sum, err := svc.Run(ctx, *dryRun, *limitLeaves)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(sum)
	if err != nil {
		fmt.Fprintf(os.Stderr, "syncleafmembership: %v\n", err)
		os.Exit(1)
	}
	if sum.NeedsArchitectureReview {
		fmt.Fprintf(os.Stderr, "syncleafmembership: needs architecture review (multi-leaf membership)\n")
		os.Exit(2)
	}
	if *dryRun {
		fmt.Fprintf(os.Stderr, "dry-run complete — no DB writes\n")
	}
}
