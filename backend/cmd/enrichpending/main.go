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
	dryRun := flag.Bool("dry-run", false, "fetch/parse pending play pages but perform zero DB writes")
	limit := flag.Int("limit", catalogsync.EnrichPendingDefaultLim, "max pending episodes to process (1–20)")
	concurrency := flag.Int("concurrency", 1, "HTTP concurrency (1–2; default 1)")
	flag.Parse()

	if err := catalogsync.ValidateEnrichPendingLimit(*limit); err != nil {
		log.Fatal(err)
	}

	cfg := config.Load()
	delayMs, conc, retries := catalogsync.ClampEnrichPendingHTTPSettings(
		cfg.OleHdTvRequestDelayMs, *concurrency, cfg.OleHdTvMaxRetries,
	)
	timeout := time.Duration(cfg.OleHdTvRequestTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	fetcher := maccms.NewHTTPFetcher(timeout, delayMs, retries, "VPlayerEnrichPending/1.0")
	db, err := store.OpenMySQL(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("mysql: %v", err)
	}
	defer db.DB.Close()

	svc := catalogsync.NewEnrichPendingWorker(db, fetcher)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	fmt.Fprintf(os.Stderr, "enrichpending dry_run=%v limit=%d concurrency=%d delay_ms=%d\n",
		*dryRun, *limit, conc, delayMs)
	sum, err := svc.Run(ctx, *limit, *dryRun, conc)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(sum)
	if err != nil {
		fmt.Fprintf(os.Stderr, "enrichpending: %v\n", err)
		os.Exit(1)
	}
	if *dryRun {
		fmt.Fprintf(os.Stderr, "dry-run complete — no DB writes\n")
	}
}
