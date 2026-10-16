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
	catchUp := flag.Bool("catch-up", false, "manual backlog catch-up: allow --limit up to 90 (do not use on a drain loop)")
	limit := flag.Int("limit", catalogsync.EnrichPendingDefaultLim, "max pending episodes to process (1–20; or 1–90 with --catch-up)")
	concurrency := flag.Int("concurrency", 1, "HTTP concurrency (1–2; default 1)")
	trigger := flag.String("trigger", store.TriggerCLI, "operational trigger: cli|timer|admin")
	requestedBy := flag.Uint64("requested-by-user-id", 0, "admin user id when --trigger=admin; must be 0 for cli|timer")
	flag.Parse()

	if err := catalogsync.ValidateEnrichPendingLimitMode(*limit, *catchUp); err != nil {
		log.Fatal(err)
	}
	trig, err := store.ValidateTriggerType(*trigger)
	if err != nil {
		log.Fatal(err)
	}
	if err := store.ValidateTriggerRequester(trig, *requestedBy); err != nil {
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
	svc.TriggerType = trig
	svc.RequestedByUserID = *requestedBy
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	fmt.Fprintf(os.Stderr, "enrichpending dry_run=%v catch_up=%v limit=%d concurrency=%d delay_ms=%d trigger=%s\n",
		*dryRun, *catchUp, *limit, conc, delayMs, trig)
	sum, err := svc.Run(ctx, *limit, *dryRun, conc, *catchUp)
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
