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

	cfg := config.Load()
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

	var eps []store.MigratableEpisode
	if *episodeID > 0 {
		ep, err := db.GetMigratableEpisodeByID(*episodeID)
		if err != nil {
			log.Fatalf("episode: %v", err)
		}
		if ep == nil {
			log.Fatalf("episode %d not found", *episodeID)
		}
		eps = []store.MigratableEpisode{*ep}
	} else {
		eps, err = db.ListMigratableEpisodes(*limit, 0)
		if err != nil {
			log.Fatalf("list episodes: %v", err)
		}
	}

	localStore, err := media.NewLocalStore(cfg.OleHdTvLocalObjectsDir, cfg.OleHdTvMediaPublicBase)
	if err != nil {
		log.Fatalf("local store: %v", err)
	}
	var obj media.ObjectStore = localStore
	if cfg.R2Endpoint != "" && cfg.R2AccessKey != "" && cfg.R2SecretKey != "" {
		obj = media.NewR2StoreRegion(cfg.R2Endpoint, cfg.R2Bucket, cfg.R2AccessKey, cfg.R2SecretKey, cfg.R2Region, cfg.R2PublicBase, localStore)
	}

	mig := &mediamigrate.Migrator{
		Store:    obj,
		Episodes: db,
		CDN:      cdn,
		Log:      log.Default(),
	}

	summary := map[string]int{
		"scanned": len(eps), "planned": 0, "skipped": 0, "already_migrated": 0, "migrated": 0, "failed": 0,
	}

	if modeDry {
		fmt.Println("=== media migration dry-run ===")
	} else {
		fmt.Println("=== media migration EXECUTE ===")
	}
	fmt.Printf("bucket=%s media_cdn_base=%s auth_mode=%s\n", cfg.R2Bucket, displayCDNBase(cdn.BaseURL), cfg.CDNURLAuthMode)

	if len(eps) == 0 {
		fmt.Println("no migratable episodes matched")
		os.Exit(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	for _, ep := range eps {
		if modeDry {
			plan, err := mig.InspectSource(ctx, ep)
			if err != nil {
				log.Printf("inspect episode=%d: %v", ep.ID, err)
				summary["failed"]++
				continue
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(plan)
			switch {
			case plan.AlreadyMigrated:
				summary["already_migrated"]++
			case plan.SkipReason != "":
				summary["skipped"]++
			default:
				summary["planned"]++
			}
			continue
		}

		// EXECUTE path — still gated by --confirm=MIGRATE above.
		res := mig.MigrateEpisode(ctx, ep)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		switch {
		case res.AlreadyMigrated:
			summary["already_migrated"]++
		case res.Skipped && res.Error == "":
			summary["skipped"]++
		case res.Error != "":
			summary["failed"]++
		default:
			summary["migrated"]++
		}
	}

	fmt.Println("--- summary ---")
	b, _ := json.MarshalIndent(summary, "", "  ")
	fmt.Println(string(b))
	if modeDry {
		fmt.Println("STOP: dry-run only — no R2 uploads and no episode migration field writes.")
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
