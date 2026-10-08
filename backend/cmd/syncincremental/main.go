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
	"github.com/tycdn/vplayer/internal/olehdtv"
	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
	"github.com/tycdn/vplayer/internal/store"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "discover and compare only; zero DB/R2 mutations (advisory lock still used)")
	limit := flag.Int("limit", 10, "max recent source videos to process (required small bound; never full catalog)")
	flag.Parse()

	if *limit <= 0 {
		log.Fatal("--limit must be > 0")
	}

	cfg := config.Load()
	mode := strings.TrimSpace(cfg.OleHdTvSourceMode)
	if mode == "maccms_recent" {
		if err := maccms.ValidateRecentLimit(*limit); err != nil {
			log.Fatal(err)
		}
	} else if *limit > 100 {
		log.Fatal("--limit too large for incremental sync foundation (max 100); use a small value like 5–20")
	}

	src, err := olehdtv.NewRecentSourceOpts(olehdtv.RecentSourceOpts{
		Mode:              mode,
		DSN:               cfg.OleHdTvMacCMSDSN,
		ExportPath:        cfg.OleHdTvExportPath,
		FixturePath:       cfg.OleHdTvFixturePath,
		HTMLDumpRoot:      cfg.OleHdTvHTMLDumpRoot,
		BaseURL:           cfg.OleHdTvPublicBaseURL,
		RequestDelayMs:    cfg.OleHdTvRequestDelayMs,
		RequestTimeoutSec: cfg.OleHdTvRequestTimeoutSec,
		MaxConcurrency:    cfg.OleHdTvMaxConcurrency,
		MaxRetries:        cfg.OleHdTvMaxRetries,
	})
	if err != nil {
		log.Fatalf("recent source: %v\n\nConfigure an authorized bounded source:\n  - OLEHDTV_SOURCE_MODE=maccms_recent + OLEHDTV_PUBLIC_BASE_URL\n  - OLEHDTV_SOURCE_MODE=fixture\n  - OLEHDTV_SOURCE_MODE=export_file + OLEHDTV_EXPORT_PATH\n  - OLEHDTV_SOURCE_MODE=maccms_db + OLEHDTV_MACCMS_DSN\nhtml_dump is not supported (full crawl).", err)
	}

	db, err := store.OpenMySQL(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("mysql: %v", err)
	}
	defer db.DB.Close()

	svc := catalogsync.NewIncrementalSync(db, src)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	fmt.Fprintf(os.Stderr, "incremental sync adapter=%s dry_run=%v limit=%d\n", src.Name(), *dryRun, *limit)
	sum, err := svc.Run(ctx, *limit, *dryRun)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(sum)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incremental sync: %v\n", err)
		os.Exit(1)
	}
	if *dryRun {
		fmt.Fprintf(os.Stderr, "dry-run complete — no catalog/R2 writes\n")
	}
}
