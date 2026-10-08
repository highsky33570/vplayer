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
	"github.com/tycdn/vplayer/internal/media"
	"github.com/tycdn/vplayer/internal/olehdtv"
	"github.com/tycdn/vplayer/internal/store"
)

func main() {
	rootFlag := flag.String("root", "", "path to import dir (categories.json + videos.jsonl + episodes.jsonl)")
	dryRun := flag.Bool("dry-run", false, "validate only; do not write DB or R2")
	fetchPosters := flag.Bool("fetch-posters", false, "download posters via existing PosterSyncer (off by default)")
	deactivateMissing := flag.Bool("deactivate-missing", false, "deactivate videos missing from this import set")
	flag.Parse()

	root, err := olehdtv.ResolveImportRoot(*rootFlag)
	if err != nil {
		log.Fatalf("import root: %v", err)
	}
	fmt.Fprintf(os.Stderr, "import root: %s\n", root)

	cfg := config.Load()
	var db *store.MySQL
	if !*dryRun {
		db, err = store.OpenMySQL(cfg.MySQLDSN)
		if err != nil {
			log.Fatalf("mysql: %v", err)
		}
		if dir := store.FindMigrationsDir(); dir != "" {
			if err := db.ApplyMigrationsDir(dir); err != nil {
				log.Fatalf("migrate: %v", err)
			}
		}
	} else {
		// Optional connectivity check in dry-run (non-fatal).
		if d, e := store.OpenMySQL(cfg.MySQLDSN); e != nil {
			fmt.Fprintf(os.Stderr, "mysql connectivity (dry-run warn): %v\n", e)
		} else {
			fmt.Fprintf(os.Stderr, "mysql connectivity: ok\n")
			_ = d.DB.Close()
		}
	}

	var posters *media.PosterSyncer
	wantPosters := *fetchPosters || cfg.OleHdTvFetchPosters
	if !*dryRun && wantPosters {
		localStore, err := media.NewLocalStore(cfg.OleHdTvLocalObjectsDir, cfg.OleHdTvMediaPublicBase)
		if err != nil {
			log.Fatalf("object store: %v", err)
		}
		var obj media.ObjectStore = localStore
		if cfg.R2Endpoint != "" && cfg.R2AccessKey != "" {
			obj = media.NewR2Store(cfg.R2Endpoint, cfg.R2Bucket, cfg.R2AccessKey, cfg.R2SecretKey, cfg.R2PublicBase, localStore)
		}
		posters = media.NewPosterSyncer(obj, true)
	}

	imp := catalogsync.NewJSONLImporter(db, posters)
	imp.FetchPosters = wantPosters && !*dryRun
	imp.DeactivateMissing = *deactivateMissing

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()

	sum, err := imp.RunImport(ctx, root, *dryRun)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(sum)
	if err != nil {
		fmt.Fprintf(os.Stderr, "import error: %v\n", err)
		os.Exit(1)
	}
	if *dryRun {
		fmt.Fprintf(os.Stderr, "dry-run complete — no database or R2 writes\n")
	}
}
