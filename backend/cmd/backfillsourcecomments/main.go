// Command backfillsourcecomments imports public OLEHDTV comments for a bounded
// batch of existing local videos. Nothing starts it: not the API, not catalog
// sync, and not a timer. It does not apply migrations.
//
// Historical backfill is not scheduled. Ongoing maintenance is maintainsourcecomments.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/olehdtv/commenthtml"
	"github.com/tycdn/vplayer/internal/sourcecomments"
	"github.com/tycdn/vplayer/internal/store"
)

const usageText = `usage:
  backfillsourcecomments --source-system olehdtv --mode full --limit N --confirm=BACKFILL [--retry-failed] [--dry-run]
  backfillsourcecomments --source-system olehdtv --mode full --drain --max-videos N --confirm=BACKFILL-DRAIN [--retry-failed] [--dry-run]

Bounded mode imports at most 100 videos. Drain mode repeats batches of 100 until the
backlog is done, a video fails, the process is interrupted, or --max-videos is reached.
--confirm=BACKFILL does not authorize --drain. Nothing starts this command automatically.

Historical drain is a full import. Later catch-up stays a separate operator command.`

const publicCommentOrigin = "https://www.olehdtv.com"

type options struct {
	SourceSystem string
	Limit        int
	MaxVideos    int
	Drain        bool
	RetryFailed  bool
	DryRun       bool
}

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n\n%s\n", err, usageText)
		os.Exit(1)
	}
	if err := run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "backfillsourcecomments: %s\n", err)
		os.Exit(1)
	}
}

func parseArgs(args []string) (options, error) {
	fs := flag.NewFlagSet("backfillsourcecomments", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	sourceSystem := fs.String("source-system", "", "source system; only olehdtv")
	mode := fs.String("mode", "", "must be full")
	confirm := fs.String("confirm", "", "BACKFILL, or BACKFILL-DRAIN with --drain")
	limit := fs.Int("limit", 0, "videos to import in bounded mode")
	maxVideos := fs.Int("max-videos", 0, "safety ceiling for --drain")
	drain := fs.Bool("drain", false, "keep importing until the ceiling, a failure, or interrupt")
	retryFailed := fs.Bool("retry-failed", false, "retry the failed checkpoint, then continue")
	dryRun := fs.Bool("dry-run", false, "print the batch without source requests or writes")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected arguments")
	}
	return validateOptions(*sourceSystem, *mode, *confirm, *limit, *maxVideos, *drain, *retryFailed, *dryRun)
}

func validateOptions(sourceSystem, mode, confirm string, limit, maxVideos int, drain, retryFailed, dryRun bool) (options, error) {
	sourceSystem = strings.TrimSpace(sourceSystem)
	if sourceSystem == "" {
		return options{}, fmt.Errorf("missing --source-system")
	}
	if sourceSystem != commenthtml.SourceSystem {
		return options{}, fmt.Errorf("unsupported --source-system %q", sourceSystem)
	}
	if mode == "" {
		return options{}, fmt.Errorf("missing --mode")
	}
	if mode != "full" {
		return options{}, fmt.Errorf("historical backfill only accepts --mode full")
	}
	if drain {
		if confirm != "BACKFILL-DRAIN" {
			return options{}, fmt.Errorf("drain mode requires --confirm=BACKFILL-DRAIN; no source requests were made")
		}
		if limit != 0 {
			return options{}, fmt.Errorf("drain mode does not use --limit")
		}
		if maxVideos <= 0 {
			return options{}, fmt.Errorf("missing --max-videos; no videos were selected")
		}
		if maxVideos > sourcecomments.MaxDrainVideos {
			return options{}, fmt.Errorf("--max-videos %d exceeds %d", maxVideos, sourcecomments.MaxDrainVideos)
		}
		return options{
			SourceSystem: sourceSystem,
			MaxVideos:    maxVideos,
			Drain:        true,
			RetryFailed:  retryFailed,
			DryRun:       dryRun,
		}, nil
	}
	if confirm != "BACKFILL" {
		return options{}, fmt.Errorf("missing --confirm=BACKFILL; no source requests were made")
	}
	if maxVideos != 0 {
		return options{}, fmt.Errorf("--max-videos is only valid with --drain")
	}
	if limit <= 0 {
		return options{}, fmt.Errorf("missing --limit; no videos were selected")
	}
	if limit > sourcecomments.MaxBackfillLimit {
		return options{}, fmt.Errorf("--limit %d exceeds %d", limit, sourcecomments.MaxBackfillLimit)
	}
	return options{
		SourceSystem: sourceSystem,
		Limit:        limit,
		RetryFailed:  retryFailed,
		DryRun:       dryRun,
	}, nil
}

func run(opts options) error {
	cfg := config.Load()
	db, err := store.OpenMySQL(cfg.MySQLDSN)
	if err != nil {
		return fmt.Errorf("mysql: %w", err)
	}
	defer db.DB.Close()
	var tables int
	if err := db.DB.QueryRow(`
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = DATABASE()
		  AND table_name IN ('video_source_comments', 'source_comment_backfill')`).Scan(&tables); err != nil {
		return err
	}
	if tables != 2 {
		return fmt.Errorf("comment backfill tables are missing; start the API so migrations 012 and 013 can apply")
	}

	var fetcher sourcecomments.PageFetcher
	if !opts.DryRun {
		delay := cfg.OleHdTvRequestDelayMs
		if delay < 1000 {
			delay = 1000
		}
		timeout := cfg.OleHdTvRequestTimeoutSec
		if timeout < 1 {
			timeout = 20
		}
		client := commenthtml.NewClient(publicCommentOrigin, time.Duration(timeout)*time.Second, delay, cfg.OleHdTvMaxRetries)
		fetcher = readOnlyFetcher{client: client}
	}
	if opts.Drain {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		_, err = sourcecomments.RunDrain(ctx, db, db, fetcher, sourcecomments.BackfillOptions{
			SourceSystem: opts.SourceSystem,
			MaxVideos:    opts.MaxVideos,
			RetryFailed:  opts.RetryFailed,
			DryRun:       opts.DryRun,
			Now:          time.Now().UTC(),
			Out:          os.Stdout,
			Locker:       sourcecomments.MySQLCommentLocker(db),
		})
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()
	_, err = sourcecomments.RunBackfill(ctx, db, db, fetcher, sourcecomments.BackfillOptions{
		SourceSystem: opts.SourceSystem,
		Limit:        opts.Limit,
		RetryFailed:  opts.RetryFailed,
		DryRun:       opts.DryRun,
		Now:          time.Now().UTC(),
		Out:          os.Stdout,
		Locker:       sourcecomments.MySQLCommentLocker(db),
	})
	return err
}

// readOnlyFetcher allows anonymous GET comment pages and refuses any other comment route.
type readOnlyFetcher struct {
	client *commenthtml.Client
}

func (f readOnlyFetcher) FetchCommentPage(ctx context.Context, sourceVideoID string, page int) (string, error) {
	if _, err := commenthtml.ListURL(publicCommentOrigin, sourceVideoID, page); err != nil {
		return "", err
	}
	return f.client.FetchCommentPage(ctx, sourceVideoID, page)
}
