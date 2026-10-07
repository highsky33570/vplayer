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
	trigger := flag.String("trigger", "cli", "sync trigger label")
	flag.Parse()

	cfg := config.Load()
	db, err := store.OpenMySQL(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("mysql: %v", err)
	}
	if dir := store.FindMigrationsDir(); dir != "" {
		if err := db.ApplyMigrationsDir(dir); err != nil {
			log.Fatalf("migrate: %v", err)
		}
	}

	adapter, err := olehdtv.NewAdapter(cfg.OleHdTvSourceMode, cfg.OleHdTvMacCMSDSN, cfg.OleHdTvExportPath, cfg.OleHdTvFixturePath, cfg.OleHdTvHTMLDumpRoot, cfg.OleHdTvPublicBaseURL)
	if err != nil {
		log.Fatalf("adapter: %v", err)
	}

	localStore, err := media.NewLocalStore(cfg.OleHdTvLocalObjectsDir, cfg.OleHdTvMediaPublicBase)
	if err != nil {
		log.Fatalf("object store: %v", err)
	}
	var obj media.ObjectStore = localStore
	if cfg.R2Endpoint != "" && cfg.R2AccessKey != "" {
		obj = media.NewR2Store(cfg.R2Endpoint, cfg.R2Bucket, cfg.R2AccessKey, cfg.R2SecretKey, cfg.R2PublicBase, localStore)
	}
	posters := media.NewPosterSyncer(obj, cfg.OleHdTvFetchPosters)
	svc := catalogsync.NewOleHdTvSyncService(db, adapter, posters)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	sum, err := svc.Run(ctx, *trigger)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(sum)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sync error: %v\n", err)
		os.Exit(1)
	}
}
