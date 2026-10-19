// Command syncsourcecomments imports public OLEHDTV comments for exactly one video.
// Nothing starts it: not the API, not the scheduler, and not catalog sync.
// It does not apply migrations and it does not walk the catalog.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/olehdtv/commenthtml"
	"github.com/tycdn/vplayer/internal/sourcecomments"
	"github.com/tycdn/vplayer/internal/store"
)

const usageText = `usage: syncsourcecomments --video-id ID --source-system olehdtv --source-video-id RID --mode full|incremental [--confirm=FULL]

Imports anonymous public comments for exactly one local video.
Full mode makes no source requests unless --confirm=FULL.
This command does not migrate the database and does not import other videos.`

const publicCommentOrigin = "https://www.olehdtv.com"

type options struct {
	VideoID       uint64
	SourceSystem  string
	SourceVideoID string
	Mode          sourcecomments.Mode
}

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n\n%s\n", err, usageText)
		os.Exit(1)
	}
	if err := run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "syncsourcecomments: %s\n", err)
		os.Exit(1)
	}
}

func parseArgs(args []string) (options, error) {
	fs := flag.NewFlagSet("syncsourcecomments", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	videoID := fs.Uint64("video-id", 0, "local videos.id")
	sourceSystem := fs.String("source-system", "", "source system; only olehdtv")
	sourceVideoID := fs.String("source-video-id", "", "source video id")
	mode := fs.String("mode", "", "full or incremental")
	confirm := fs.String("confirm", "", "must be FULL for --mode full")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected arguments")
	}
	return validateOptions(*videoID, *sourceSystem, *sourceVideoID, *mode, *confirm)
}

func validateOptions(videoID uint64, sourceSystem, sourceVideoID, mode, confirm string) (options, error) {
	if videoID == 0 {
		return options{}, fmt.Errorf("missing --video-id; no video was selected")
	}
	sourceSystem = strings.TrimSpace(sourceSystem)
	if sourceSystem == "" {
		return options{}, fmt.Errorf("missing --source-system")
	}
	if sourceSystem != commenthtml.SourceSystem {
		return options{}, fmt.Errorf("unsupported --source-system %q", sourceSystem)
	}
	sourceVideoID = strings.TrimSpace(sourceVideoID)
	if sourceVideoID == "" {
		return options{}, fmt.Errorf("missing --source-video-id")
	}
	if _, err := strconv.ParseUint(sourceVideoID, 10, 64); err != nil || len(sourceVideoID) > 20 {
		return options{}, fmt.Errorf("invalid --source-video-id")
	}
	var syncMode sourcecomments.Mode
	switch mode {
	case "full":
		if confirm != "FULL" {
			return options{}, fmt.Errorf("full mode requires --confirm=FULL; no source requests were made")
		}
		syncMode = sourcecomments.ModeFull
	case "incremental":
		syncMode = sourcecomments.ModeIncremental
	case "":
		return options{}, fmt.Errorf("missing --mode")
	default:
		return options{}, fmt.Errorf("invalid --mode %q", mode)
	}
	return options{
		VideoID:       videoID,
		SourceSystem:  sourceSystem,
		SourceVideoID: sourceVideoID,
		Mode:          syncMode,
	}, nil
}

func run(opts options) error {
	cfg := config.Load()
	db, err := store.OpenMySQL(cfg.MySQLDSN)
	if err != nil {
		return fmt.Errorf("mysql: %w", err)
	}
	defer db.DB.Close()

	var title, system, sourceID string
	err = db.DB.QueryRow(`
		SELECT title, source_system, source_id
		FROM videos WHERE id=?`, opts.VideoID).Scan(&title, &system, &sourceID)
	if err != nil {
		return fmt.Errorf("video %d: %w", opts.VideoID, err)
	}
	if system != opts.SourceSystem || sourceID != opts.SourceVideoID {
		return fmt.Errorf("video %d is %s/%s, not %s/%s", opts.VideoID, system, sourceID, opts.SourceSystem, opts.SourceVideoID)
	}
	var tableCount int
	if err := db.DB.QueryRow(`
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'video_source_comments'`).Scan(&tableCount); err != nil {
		return err
	}
	if tableCount != 1 {
		return fmt.Errorf("video_source_comments is missing; start the API so migration 012 can apply")
	}

	delay := cfg.OleHdTvRequestDelayMs
	if delay < 1000 {
		delay = 1000
	}
	timeout := cfg.OleHdTvRequestTimeoutSec
	if timeout < 1 {
		timeout = 20
	}
	client := commenthtml.NewClient(publicCommentOrigin, time.Duration(timeout)*time.Second, delay, cfg.OleHdTvMaxRetries)
	fetcher := oneVideoFetcher{client: client, sourceVideoID: opts.SourceVideoID}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	result, err := sourcecomments.Sync(ctx, db, fetcher, opts.VideoID, opts.SourceSystem, opts.SourceVideoID, opts.Mode, time.Now().UTC())
	fmt.Printf("video_id=%d title=%s source_system=%s source_video_id=%s pages=%d discovered=%d inserted=%d updated=%d skipped=%d errors=%d\n",
		opts.VideoID, title, opts.SourceSystem, opts.SourceVideoID, result.PagesFetched, result.Discovered, result.Inserted, result.Updated, result.Skipped, len(result.Errors))
	for _, item := range result.Errors {
		fmt.Fprintf(os.Stderr, "error: %s\n", item)
	}
	return err
}

// oneVideoFetcher allows anonymous GET comment pages for the single requested source id.
type oneVideoFetcher struct {
	client        *commenthtml.Client
	sourceVideoID string
}

func (f oneVideoFetcher) FetchCommentPage(ctx context.Context, sourceVideoID string, page int) (string, error) {
	if sourceVideoID != f.sourceVideoID {
		return "", fmt.Errorf("refusing source video %s", sourceVideoID)
	}
	if _, err := commenthtml.ListURL(publicCommentOrigin, sourceVideoID, page); err != nil {
		return "", err
	}
	return f.client.FetchCommentPage(ctx, sourceVideoID, page)
}
