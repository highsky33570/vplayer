// Command backfillsourcecomments imports public OLEHDTV comments for a bounded
// batch of existing local videos. Nothing starts it: not the API, not catalog
// sync, and not a timer. It does not apply migrations.
//
// After the historical full pass, later operator runs of syncsourcecomments can
// catch up without a scheduler:
//   - frequent --mode incremental for new top-level comments
//   - slower --mode full to repair replies added on older parents
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/olehdtv/commenthtml"
	"github.com/tycdn/vplayer/internal/sourcecomments"
	"github.com/tycdn/vplayer/internal/store"
)

const usageText = `usage: backfillsourcecomments --source-system olehdtv --mode full --limit N --confirm=BACKFILL [--retry-failed] [--dry-run]

Imports anonymous public comments for up to N existing OLEHDTV videos (maximum 100).
Makes no source requests unless --confirm=BACKFILL and --mode full are exact.
Does not migrate the database. Does not start from the API, catalog sync, or a timer.

Later catch-up stays operator-run: syncsourcecomments --mode incremental for new
top-level comments, and --mode full for replies on older parents.`

const publicCommentOrigin = "https://www.olehdtv.com"

type options struct {
	SourceSystem string
	Limit        int
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
	confirm := fs.String("confirm", "", "must be BACKFILL")
	limit := fs.Int("limit", 0, "videos to import this invocation")
	retryFailed := fs.Bool("retry-failed", false, "retry the failed checkpoint, then continue")
	dryRun := fs.Bool("dry-run", false, "print the batch without source requests or writes")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected arguments")
	}
	return validateOptions(*sourceSystem, *mode, *confirm, *limit, *retryFailed, *dryRun)
}

func validateOptions(sourceSystem, mode, confirm string, limit int, retryFailed, dryRun bool) (options, error) {
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
	if confirm != "BACKFILL" {
		return options{}, fmt.Errorf("missing --confirm=BACKFILL; no source requests were made")
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
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()
	_, err = sourcecomments.RunBackfill(ctx, db, db, fetcher, sourcecomments.BackfillOptions{
		SourceSystem: opts.SourceSystem,
		Limit:        opts.Limit,
		RetryFailed:  opts.RetryFailed,
		DryRun:       opts.DryRun,
		Now:          time.Now().UTC(),
		Out:          os.Stdout,
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
