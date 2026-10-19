// Command maintainsourcecomments runs one OLEHDTV source-comment maintenance cycle.
// Nothing starts it by itself: deploy installs the unit files and does not enable the timers.
// It does not apply migrations and it does not read historical source_comment_backfill rows.
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
  maintainsourcecomments --source-system olehdtv --mode incremental|full --confirm=MAINTENANCE [--retry-failed]
  maintainsourcecomments --source-system olehdtv --mode incremental|full --status

Incremental maintenance stops once known top-level comments are reached.
Full maintenance walks every public comment page.
A completed historical backfill does not skip either mode.
--status prints the latest cycle and makes no source requests.`

const publicCommentOrigin = "https://www.olehdtv.com"

type options struct {
	SourceSystem string
	Mode         string
	RetryFailed  bool
	StatusOnly   bool
}

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n\n%s\n", err, usageText)
		os.Exit(1)
	}
	if err := run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "maintainsourcecomments: %s\n", err)
		os.Exit(1)
	}
}

func parseArgs(args []string) (options, error) {
	fs := flag.NewFlagSet("maintainsourcecomments", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	sourceSystem := fs.String("source-system", "", "source system; only olehdtv")
	mode := fs.String("mode", "", "incremental or full")
	confirm := fs.String("confirm", "", "must be MAINTENANCE to run a cycle")
	retryFailed := fs.Bool("retry-failed", false, "retry the failed video, then continue the same cycle")
	statusOnly := fs.Bool("status", false, "print the latest cycle without source requests or writes")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected arguments")
	}
	return validateOptions(*sourceSystem, *mode, *confirm, *retryFailed, *statusOnly)
}

func validateOptions(sourceSystem, mode, confirm string, retryFailed, statusOnly bool) (options, error) {
	sourceSystem = strings.TrimSpace(sourceSystem)
	if sourceSystem == "" {
		return options{}, fmt.Errorf("missing --source-system")
	}
	if sourceSystem != commenthtml.SourceSystem {
		return options{}, fmt.Errorf("unsupported --source-system %q", sourceSystem)
	}
	switch mode {
	case store.MaintenanceModeIncremental, store.MaintenanceModeFull:
	case "":
		return options{}, fmt.Errorf("missing --mode")
	default:
		return options{}, fmt.Errorf("invalid --mode %q", mode)
	}
	if statusOnly {
		if retryFailed {
			return options{}, fmt.Errorf("--status does not accept --retry-failed")
		}
		if confirm != "" {
			return options{}, fmt.Errorf("--status does not accept --confirm")
		}
		return options{SourceSystem: sourceSystem, Mode: mode, StatusOnly: true}, nil
	}
	if confirm != "MAINTENANCE" {
		return options{}, fmt.Errorf("maintenance requires --confirm=MAINTENANCE; no source requests were made")
	}
	return options{SourceSystem: sourceSystem, Mode: mode, RetryFailed: retryFailed}, nil
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
		  AND table_name IN (
		    'video_source_comments',
		    'source_comment_maintenance',
		    'source_comment_maintenance_cycles'
		  )`).Scan(&tables); err != nil {
		return err
	}
	if tables != 3 {
		return fmt.Errorf("comment maintenance tables are missing; start the API so migration 015 can apply")
	}
	if opts.StatusOnly {
		return sourcecomments.ReportMaintenanceStatus(db, sourcecomments.MaintenanceOptions{
			SourceSystem: opts.SourceSystem,
			Mode:         opts.Mode,
			StatusOnly:   true,
			Out:          os.Stdout,
		})
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	_, err = sourcecomments.RunMaintenance(ctx, db, db, readOnlyFetcher{client: client}, sourcecomments.MaintenanceOptions{
		SourceSystem: opts.SourceSystem,
		Mode:         opts.Mode,
		RetryFailed:  opts.RetryFailed,
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
