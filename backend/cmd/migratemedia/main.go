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

	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/media"
	"github.com/tycdn/vplayer/internal/mediamigrate"
	"github.com/tycdn/vplayer/internal/store"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "plan/inspect only; never write R2 or MySQL media fields")
	execute := flag.Bool("execute", false, "required together with confirmation to perform real migration writes")
	confirm := flag.String("confirm", "", "must be MIGRATE to allow --execute")
	limit := flag.Int("limit", 1, "max episodes to process")
	episodeID := flag.Uint64("episode-id", 0, "optional specific episode id")
	trigger := flag.String("trigger", store.TriggerCLI, "operational trigger: cli|timer|admin")
	requestedBy := flag.Uint64("requested-by-user-id", 0, "admin user id when --trigger=admin; must be 0 for cli|timer")
	flag.Parse()

	modeDry := *dryRun
	modeExec := *execute

	if modeDry && modeExec {
		log.Fatal("use either --dry-run or --execute, not both")
	}
	if !modeDry && !modeExec {
		log.Printf("refusing to run: specify --dry-run (safe) or --execute --confirm=MIGRATE")
		os.Exit(2)
	}
	if modeExec && *confirm != "MIGRATE" {
		log.Fatal("--execute requires --confirm=MIGRATE")
	}

	trig, err := store.ValidateTriggerType(*trigger)
	if err != nil {
		log.Fatal(err)
	}
	if err := store.ValidateTriggerRequester(trig, *requestedBy); err != nil {
		log.Fatal(err)
	}

	cfg := config.Load()
	r2cfg := r2ExecuteConfig{
		Endpoint:   cfg.R2Endpoint,
		AccessKey:  cfg.R2AccessKey,
		SecretKey:  cfg.R2SecretKey,
		Bucket:     cfg.R2Bucket,
		Region:     cfg.R2Region,
		PublicBase: cfg.R2PublicBase,
	}

	// Fail-closed BEFORE MySQL, lock, operational_runs, uploads, or migration DB writes.
	if modeExec {
		if err := validateExecuteR2Config(r2cfg); err != nil {
			log.Fatalf("execute refused: %v", err)
		}
	}

	db, err := store.OpenMySQL(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("mysql: %v", err)
	}
	defer db.DB.Close()
	if dir := store.FindMigrationsDir(); dir != "" {
		if err := db.ApplyMigrationsDir(dir); err != nil {
			log.Printf("migrate warning: %v", err)
		}
	}

	cdn := media.OptionsFromEnv(cfg.MediaCDNBaseURL, cfg.CDNBaseURL, cfg.R2Bucket)
	if strings.TrimSpace(cdn.Bucket) == "" {
		log.Fatal("R2_BUCKET is empty")
	}

	var obj media.ObjectStore
	if modeExec {
		// Authoritative remote R2 only — no LocalStore destination/fallback.
		obj = media.NewR2StoreRegion(
			r2cfg.Endpoint, r2cfg.Bucket, r2cfg.AccessKey, r2cfg.SecretKey,
			r2cfg.Region, r2cfg.PublicBase, nil,
		)
	}
	// Dry-run InspectSource does not upload; Store may stay nil.

	mig := &mediamigrate.Migrator{
		Store:    obj,
		Episodes: db,
		CDN:      cdn,
		Log:      log.Default(),
	}
	runner := mediamigrate.NewBatchRunner(db, mig)
	runner.TriggerType = trig
	runner.RequestedByUserID = *requestedBy

	if modeDry {
		fmt.Println("=== media migration dry-run ===")
		if !r2ExecuteConfigReady(r2cfg) {
			fmt.Println("note: R2 execute configuration is unavailable — dry-run inspect/plan only; --execute would be refused")
		}
	} else {
		fmt.Println("=== media migration EXECUTE ===")
		fmt.Println("object_store=r2 (remote authoritative; no local fallback)")
	}
	fmt.Printf("bucket=%s media_cdn_base=%s auth_mode=%s trigger=%s\n",
		cfg.R2Bucket, displayCDNBase(cdn.BaseURL), cfg.CDNURLAuthMode, trig)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	sum, err := runner.Run(ctx, mediamigrate.RunOptions{
		DryRun:    modeDry,
		Execute:   modeExec,
		Limit:     *limit,
		EpisodeID: *episodeID,
	})

	summary := map[string]int{
		"scanned":          sum.Selected,
		"planned":          sum.Planned,
		"skipped":          sum.Skipped,
		"already_migrated": sum.AlreadyMigrated,
		"migrated":         sum.Migrated,
		"failed":           sum.Failed,
	}
	if sum.Selected == 0 && err == nil {
		fmt.Println("no migratable episodes matched")
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(sum)

	fmt.Println("--- summary ---")
	b, _ := json.MarshalIndent(summary, "", "  ")
	fmt.Println(string(b))
	if modeDry {
		fmt.Println("STOP: dry-run only — no R2 uploads and no episode migration field writes.")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "media migration: %v\n", err)
		os.Exit(1)
	}
	os.Exit(exitCodeFromSummary(summary, modeExec))
}

// exitCodeFromSummary returns 0 on success and non-zero when execute migrations failed.
func exitCodeFromSummary(summary map[string]int, modeExec bool) int {
	if modeExec && summary["failed"] > 0 {
		return 1
	}
	return 0
}

func displayCDNBase(base string) string {
	b := strings.TrimSpace(base)
	if b == "" || strings.Contains(strings.ToLower(b), "example.com") {
		return b + "  [PLACEHOLDER — set MEDIA_CDN_BASE_URL]"
	}
	return b
}
