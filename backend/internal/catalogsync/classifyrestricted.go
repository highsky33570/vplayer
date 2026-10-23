package catalogsync

import (
	"context"
	"strings"

	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
	"github.com/tycdn/vplayer/internal/store"
)

const (
	ClassifyRestrictedDefaultLimit = 20
	ClassifyRestrictedMaxLimit     = 20
)

// ClassifyRestrictedStore is the narrow DB surface for historical membership labeling.
type ClassifyRestrictedStore interface {
	ListUnclassifiedUnavailable(afterID uint64, limit int) ([]store.UnclassifiedUnavailableEpisode, error)
	MarkMembershipRestriction(id uint64) (bool, error)
}

// ClassifyRestrictedSummary is the bounded result of one classification batch.
// NextAfterID is the resume cursor: the last episode id fully classified.
// A fetch error stops the batch before that row so a rerun retries it.
type ClassifyRestrictedSummary struct {
	DryRun           bool   `json:"dry_run"`
	AfterID          uint64 `json:"after_id"`
	NextAfterID      uint64 `json:"next_after_id"`
	Selected         int    `json:"selected"`
	MarkedRestricted int    `json:"marked_restricted"`
	NotMembership    int    `json:"not_membership"`
	PublicURLPresent int    `json:"public_url_present"`
	FetchFailed      int    `json:"fetch_failed"`
	StoppedOnError   bool   `json:"stopped_on_error"`
	Error            string `json:"error,omitempty"`
}

// ClassifyRestrictedWorker rechecks historical unavailable play pages.
// It uses the same GET-only ClassifyPlayPage markers as enrichment.
// It never writes playback_url, playback_status, or is_active.
type ClassifyRestrictedWorker struct {
	Store   ClassifyRestrictedStore
	Fetcher PageFetcher
}

func NewClassifyRestrictedWorker(st ClassifyRestrictedStore, fetcher PageFetcher) *ClassifyRestrictedWorker {
	return &ClassifyRestrictedWorker{Store: st, Fetcher: fetcher}
}

func ValidateClassifyRestrictedLimit(limit int) error {
	if limit < 1 || limit > ClassifyRestrictedMaxLimit {
		return errClassifyLimit
	}
	return nil
}

type classifyLimitError struct{}

func (classifyLimitError) Error() string {
	return "classifyrestricted --limit must be 1..20"
}

var errClassifyLimit = classifyLimitError{}

// Run classifies at most limit candidates with id greater than afterID.
// dryRun fetches and classifies but does not write restriction_reason.
func (w *ClassifyRestrictedWorker) Run(ctx context.Context, afterID uint64, limit int, dryRun bool) (ClassifyRestrictedSummary, error) {
	sum := ClassifyRestrictedSummary{DryRun: dryRun, AfterID: afterID, NextAfterID: afterID}
	if err := ValidateClassifyRestrictedLimit(limit); err != nil {
		sum.Error = err.Error()
		return sum, err
	}
	if w.Store == nil || w.Fetcher == nil {
		sum.Error = "store and fetcher are required"
		return sum, errClassifyLimitRequired
	}
	rows, err := w.Store.ListUnclassifiedUnavailable(afterID, limit)
	if err != nil {
		sum.Error = err.Error()
		sum.StoppedOnError = true
		return sum, err
	}
	sum.Selected = len(rows)
	for _, ep := range rows {
		if ctx.Err() != nil {
			sum.StoppedOnError = true
			sum.Error = ctx.Err().Error()
			return sum, ctx.Err()
		}
		body, ferr := w.Fetcher.Fetch(ctx, strings.TrimSpace(ep.PlayPageURL))
		if ferr != nil {
			sum.FetchFailed++
			sum.StoppedOnError = true
			sum.Error = "fetch failed"
			return sum, ferr
		}
		switch maccms.ClassifyPlayPage(body) {
		case maccms.PlayPageSourceRestricted:
			if !dryRun {
				ok, uerr := w.Store.MarkMembershipRestriction(ep.ID)
				if uerr != nil {
					sum.StoppedOnError = true
					sum.Error = uerr.Error()
					return sum, uerr
				}
				if ok {
					sum.MarkedRestricted++
				}
			} else {
				sum.MarkedRestricted++
			}
		case maccms.PlayPageOK:
			if pageHasPlaybackURL(body) {
				sum.PublicURLPresent++
			}
			sum.NotMembership++
		default:
			sum.NotMembership++
		}
		sum.NextAfterID = ep.ID
	}
	return sum, nil
}

func pageHasPlaybackURL(body string) bool {
	p, ok := maccms.ParsePlayerAAAA(body)
	if !ok || p == nil {
		return false
	}
	url := strings.TrimSpace(p.URL)
	return strings.HasPrefix(strings.ToLower(url), "http://") || strings.HasPrefix(strings.ToLower(url), "https://")
}

type classifyRequiredError struct{}

func (classifyRequiredError) Error() string { return "store and fetcher are required" }

var errClassifyLimitRequired = classifyRequiredError{}
