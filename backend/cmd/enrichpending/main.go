package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tycdn/vplayer/internal/catalogsync"
	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
	"github.com/tycdn/vplayer/internal/store"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "fetch/parse pending play pages but perform zero DB writes")
	catchUp := flag.Bool("catch-up", false, "manual backlog catch-up: allow --limit up to 90")
	drain := flag.Bool("drain", false, "with --catch-up: repeat bounded batches until empty or safety limits (manual only; not for timers)")
	limit := flag.Int("limit", catalogsync.EnrichPendingDefaultLim, "max pending episodes per batch (1–20; or 1–90 with --catch-up)")
	concurrency := flag.Int("concurrency", 1, "HTTP concurrency (1–2; default 1)")
	maxBatches := flag.Int("max-batches", catalogsync.EnrichPendingDrainMaxBatchesDefault, "drain only: max batches (1–100)")
	maxRuntime := flag.Duration("max-runtime", catalogsync.EnrichPendingDrainMaxRuntimeDefault, "drain only: overall runtime budget (hard max 3h)")
	trigger := flag.String("trigger", store.TriggerCLI, "operational trigger: cli|timer|admin")
	requestedBy := flag.Uint64("requested-by-user-id", 0, "admin user id when --trigger=admin; must be 0 for cli|timer")
	flag.Parse()

	if err := catalogsync.ValidateEnrichPendingDrainFlags(*catchUp, *drain, *dryRun); err != nil {
		log.Fatal(err)
	}
	if *drain {
		if err := catalogsync.ValidateEnrichPendingDrainOptions(*limit, *maxBatches, *maxRuntime); err != nil {
			log.Fatal(err)
		}
	} else if err := catalogsync.ValidateEnrichPendingLimitMode(*limit, *catchUp); err != nil {
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

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	if *drain {
		// Signal + max-runtime budget for the whole drain session.
		sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		ctx, cancel := context.WithTimeout(sigCtx, *maxRuntime)
		defer cancel()

		fmt.Fprintf(os.Stderr, "enrichpending drain=true catch_up=true limit=%d concurrency=%d max_batches=%d max_runtime=%s delay_ms=%d trigger=%s\n",
			*limit, conc, *maxBatches, maxRuntime.String(), delayMs, trig)
		sum, runErr := svc.RunDrain(ctx, catalogsync.EnrichPendingDrainOptions{
			Limit:       *limit,
			Concurrency: conc,
			MaxBatches:  *maxBatches,
			MaxRuntime:  *maxRuntime,
		})
		_ = enc.Encode(sum)
		if runErr != nil {
			fmt.Fprintf(os.Stderr, "enrichpending drain: %v (stop_reason=%s)\n", runErr, sum.StopReason)
		} else {
			fmt.Fprintf(os.Stderr, "enrichpending drain complete stop_reason=%s batches=%d enriched=%d unavailable=%d\n",
				sum.StopReason, sum.BatchesRun, sum.TotalEnriched, sum.TotalUnavailable)
		}
		os.Exit(catalogsync.DrainExitCode(sum, runErr))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	fmt.Fprintf(os.Stderr, "enrichpending dry_run=%v catch_up=%v limit=%d concurrency=%d delay_ms=%d trigger=%s\n",
		*dryRun, *catchUp, *limit, conc, delayMs, trig)
	sum, err := svc.Run(ctx, *limit, *dryRun, conc, *catchUp)
	_ = enc.Encode(sum)
	if err != nil {
		fmt.Fprintf(os.Stderr, "enrichpending: %v\n", err)
		os.Exit(1)
	}
	if *dryRun {
		fmt.Fprintf(os.Stderr, "dry-run complete — no DB writes\n")
	}
}
