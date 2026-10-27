package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/tycdn/vplayer/internal/catalogsync"
	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
	"github.com/tycdn/vplayer/internal/store"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "GET and classify only; do not write restriction_reason")
	limit := flag.Int("limit", catalogsync.ClassifyRestrictedDefaultLimit, "max episodes this invocation (1–20)")
	afterID := flag.Uint64("after-id", 0, "resume cursor; only episode ids greater than this value")
	flag.Parse()

	if err := catalogsync.ValidateClassifyRestrictedLimit(*limit); err != nil {
		log.Fatal(err)
	}

	cfg := config.Load()
	delayMs, _, retries := catalogsync.ClampEnrichPendingHTTPSettings(cfg.OleHdTvRequestDelayMs, 1, cfg.OleHdTvMaxRetries)
	timeout := time.Duration(cfg.OleHdTvRequestTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	fetcher := maccms.NewHTTPFetcher(timeout, delayMs, retries, "VPlayerClassifyRestricted/1.0")
	fetcher.SetAuthCookie(cfg.OleHdTvAuthCookie)
	fetcher.TrustPublicBaseURL(cfg.OleHdTvPublicBaseURL)
	authConfigured := cfg.OleHdTvAuthConfigured()
	db, err := store.OpenMySQL(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("mysql: %v", err)
	}
	defer db.DB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	fmt.Fprintf(os.Stderr, "classifyrestricted dry_run=%v limit=%d after_id=%d delay_ms=%d auth=%v\n",
		*dryRun, *limit, *afterID, delayMs, authConfigured)
	sum, err := catalogsync.NewClassifyRestrictedWorker(db, fetcher).Run(ctx, *afterID, *limit, *dryRun)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(sum)
	if err != nil {
		fmt.Fprintf(os.Stderr, "classifyrestricted: %v (next_after_id=%d)\n", err, sum.NextAfterID)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "classifyrestricted selected=%d marked=%d not_membership=%d public_url=%d next_after_id=%d\n",
		sum.Selected, sum.MarkedRestricted, sum.NotMembership, sum.PublicURLPresent, sum.NextAfterID)
}
