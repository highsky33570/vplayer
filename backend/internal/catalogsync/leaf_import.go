package catalogsync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/olehdtv"
	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
	"github.com/tycdn/vplayer/internal/store"
)

// ErrLeafArchitectureReview is returned when multi-leaf conflicts require review.
var ErrLeafArchitectureReview = errors.New("multi-leaf membership observed; architecture review required (single leaf_source_id)")

// Ensure *store.MySQL satisfies LeafImportStore.
var _ LeafImportStore = (*store.MySQL)(nil)

// Hard caps prevent accidental unlimited leaf import crawls.
const (
	LeafImportHardMaxPagesPerLeaf = 10
	LeafImportHardMaxNewPerLeaf   = 50
	LeafImportHardMaxNewTotal     = 500
	LeafImportHardMaxRequests     = 2000
	LeafImportHardMaxLeaves       = 64

	// Defaults match the approved first full dry-run / bounded real run.
	LeafImportDefaultMaxPagesPerLeaf = 3
	LeafImportDefaultMaxNewPerLeaf   = 8
	LeafImportDefaultMaxNewTotal     = 150
	LeafImportDefaultMaxRequests     = 400
)

// LeafImportOptions configures a bounded leaf scan + optional import run.
type LeafImportOptions struct {
	DryRun          bool
	LimitLeaves     int // 0 = all eligible (still hard-capped)
	MaxPagesPerLeaf int
	MaxNewPerLeaf   int
	MaxNewTotal     int
	MaxRequests     int
}

// DefaultLeafImportOptions returns conservative defaults for import-capable mode.
func DefaultLeafImportOptions() LeafImportOptions {
	return LeafImportOptions{
		MaxPagesPerLeaf: LeafImportDefaultMaxPagesPerLeaf,
		MaxNewPerLeaf:   LeafImportDefaultMaxNewPerLeaf,
		MaxNewTotal:     LeafImportDefaultMaxNewTotal,
		MaxRequests:     LeafImportDefaultMaxRequests,
	}
}

// ValidateLeafImportOptions clamps/rejects unsafe bounds.
func ValidateLeafImportOptions(o *LeafImportOptions) error {
	if o == nil {
		return fmt.Errorf("options required")
	}
	if o.LimitLeaves < 0 {
		return fmt.Errorf("--limit-leaves must be >= 0")
	}
	if o.MaxPagesPerLeaf <= 0 {
		o.MaxPagesPerLeaf = LeafImportDefaultMaxPagesPerLeaf
	}
	if o.MaxNewPerLeaf < 0 {
		return fmt.Errorf("--max-new-per-leaf must be >= 0")
	}
	if o.MaxNewTotal < 0 {
		return fmt.Errorf("--max-new-total must be >= 0")
	}
	if o.MaxRequests <= 0 {
		o.MaxRequests = LeafImportDefaultMaxRequests
	}
	if o.MaxPagesPerLeaf > LeafImportHardMaxPagesPerLeaf {
		return fmt.Errorf("--max-pages-per-leaf exceeds hard cap %d", LeafImportHardMaxPagesPerLeaf)
	}
	if o.MaxNewPerLeaf > LeafImportHardMaxNewPerLeaf {
		return fmt.Errorf("--max-new-per-leaf exceeds hard cap %d", LeafImportHardMaxNewPerLeaf)
	}
	if o.MaxNewTotal > LeafImportHardMaxNewTotal {
		return fmt.Errorf("--max-new-total exceeds hard cap %d", LeafImportHardMaxNewTotal)
	}
	if o.MaxRequests > LeafImportHardMaxRequests {
		return fmt.Errorf("--max-requests exceeds hard cap %d", LeafImportHardMaxRequests)
	}
	if o.LimitLeaves > LeafImportHardMaxLeaves {
		return fmt.Errorf("--limit-leaves exceeds hard cap %d", LeafImportHardMaxLeaves)
	}
	return nil
}

// LeafImportLeafStat is per-leaf reporting.
type LeafImportLeafStat struct {
	LeafSourceID       string `json:"leaf_source_id"`
	ParentSourceID     string `json:"parent_source_id"`
	PagesScanned       int    `json:"pages_scanned"`
	ListingItems       int    `json:"listing_items"`
	MembershipAssign   int    `json:"membership_assign"`
	Unchanged          int    `json:"unchanged"`
	MissingCandidates  int    `json:"missing_candidates"`
	ImportsSelected    int    `json:"imports_selected"`
	OwnershipMismatch  int    `json:"ownership_mismatch"`
	MembershipConflict int    `json:"membership_conflict"`
}

// LeafImportSummary is the machine-readable report for syncleafimport.
type LeafImportSummary struct {
	DryRun                        bool   `json:"dry_run"`
	BaseURL                       string `json:"base_url,omitempty"`
	LeavesSelected                int    `json:"leaves_selected"`
	LeavesScanned                 int    `json:"leaves_scanned"`
	PagesScanned                  int    `json:"pages_scanned"`
	ListingRequests               int    `json:"listing_requests"`
	ListingItems                  int    `json:"listing_items"`
	DetailRequests                int    `json:"detail_requests"`
	PlayRequests                  int    `json:"play_requests"`
	DistinctVideosDiscovered      int    `json:"distinct_videos_discovered"`
	ExistingMatches               int    `json:"existing_matches"`
	MembershipAssigned            int    `json:"membership_assigned"`
	MembershipWouldAssign         int    `json:"membership_would_assign"`
	Unchanged                     int    `json:"unchanged"`
	MissingCandidates             int    `json:"missing_candidates"`
	ImportsSelected               int    `json:"imports_selected"`
	ImportsWouldSelect            int    `json:"imports_would_select"`
	ImportsSucceeded              int    `json:"imports_succeeded"`
	ImportsFailed                 int    `json:"imports_failed"`
	ImportsSkippedPerLeafCap      int    `json:"imports_skipped_per_leaf_cap"`
	ImportsSkippedTotalCap        int    `json:"imports_skipped_total_cap"`
	OwnershipMismatch             int    `json:"ownership_mismatch"`
	MembershipConflicts           int    `json:"membership_conflicts"` // aggregate: multi-leaf + existing-leaf
	MultiLeafConflicts            int    `json:"multi_leaf_conflicts"`
	SameParentMultiLeafConflicts  int    `json:"same_parent_multi_leaf_conflicts"`
	CrossParentMultiLeafConflicts int    `json:"cross_parent_multi_leaf_conflicts"`
	ExistingLeafConflicts         int    `json:"existing_leaf_conflicts"`
	PaginationLoops               int    `json:"pagination_loops"`
	RequestBudgetExhausted        bool   `json:"request_budget_exhausted"`
	ImportBudgetExhausted         bool   `json:"import_budget_exhausted"`
	NeedsArchitectureReview       bool   `json:"needs_architecture_review"`
	Failed                        int    `json:"failed"`
	LimitLeaves                   int    `json:"limit_leaves,omitempty"`
	MaxPagesPerLeaf               int    `json:"max_pages_per_leaf"`
	MaxNewPerLeaf                 int    `json:"max_new_per_leaf"`
	MaxNewTotal                   int    `json:"max_new_total"`
	MaxRequests                   int    `json:"max_requests"`
	// ConfiguredHTTPRetries is the HTTPFetcher retry count (attempts per logical Fetch).
	// max_requests is a LOGICAL fetch budget; wire attempts may be up to max_requests * retries.
	ConfiguredHTTPRetries int                  `json:"configured_http_retries,omitempty"`
	PerLeaf               []LeafImportLeafStat `json:"per_leaf,omitempty"`
	Error                 string               `json:"error,omitempty"`
}

// LeafImportStore is the DB surface for leaf import (mockable).
type LeafImportStore interface {
	CatalogWriter
	ListCategories() ([]model.Category, error)
	SetVideoLeafSourceID(videoID uint64, leafSourceID string) error
}

// LeafImportSync performs bounded leaf listing scan, membership assign, and optional import.
type LeafImportSync struct {
	Store   LeafImportStore
	Fetcher PageFetcher
	BaseURL string
	Log     *slog.Logger
	// HTTPRetries is informational (configured fetcher retry count). Not used for control flow.
	HTTPRetries int
}

func NewLeafImportSync(st LeafImportStore, fetcher PageFetcher, baseURL string) *LeafImportSync {
	return &LeafImportSync{
		Store:   st,
		Fetcher: fetcher,
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Log:     slog.Default().With("component", "leaf-import"),
	}
}

type leafPlan struct {
	vodID          string
	leafSourceID   string
	parentLocalID  uint64
	parentSourceID string
}

// Run executes discovery/classification first, then (unless dry-run) membership + imports.
func (s *LeafImportSync) Run(ctx context.Context, opt LeafImportOptions) (LeafImportSummary, error) {
	sum := LeafImportSummary{
		DryRun:                opt.DryRun,
		BaseURL:               s.BaseURL,
		PlayRequests:          0,
		MaxPagesPerLeaf:       opt.MaxPagesPerLeaf,
		MaxNewPerLeaf:         opt.MaxNewPerLeaf,
		MaxNewTotal:           opt.MaxNewTotal,
		MaxRequests:           opt.MaxRequests,
		ConfiguredHTTPRetries: s.HTTPRetries,
	}
	if opt.LimitLeaves > 0 {
		sum.LimitLeaves = opt.LimitLeaves
	}
	if err := ValidateLeafImportOptions(&opt); err != nil {
		sum.Error = err.Error()
		sum.Failed++
		return sum, err
	}
	sum.MaxPagesPerLeaf = opt.MaxPagesPerLeaf
	sum.MaxNewPerLeaf = opt.MaxNewPerLeaf
	sum.MaxNewTotal = opt.MaxNewTotal
	sum.MaxRequests = opt.MaxRequests

	if s.Store == nil || s.Fetcher == nil {
		sum.Error = "store and fetcher required"
		sum.Failed++
		return sum, fmt.Errorf("%s", sum.Error)
	}
	if s.BaseURL == "" {
		sum.Error = "OLEHDTV public base URL required"
		sum.Failed++
		return sum, fmt.Errorf("%s", sum.Error)
	}

	cats, err := s.Store.ListCategories()
	if err != nil {
		sum.Error = "list categories: " + err.Error()
		sum.Failed++
		return sum, err
	}
	leaves := SelectLeafTargets(cats)
	if opt.LimitLeaves > 0 && len(leaves) > opt.LimitLeaves {
		leaves = leaves[:opt.LimitLeaves]
	}
	if len(leaves) > LeafImportHardMaxLeaves {
		leaves = leaves[:LeafImportHardMaxLeaves]
	}
	sum.LeavesSelected = len(leaves)

	perLeaf := map[string]*LeafImportLeafStat{}
	for _, leaf := range leaves {
		perLeaf[leaf.SourceID] = &LeafImportLeafStat{
			LeafSourceID:   leaf.SourceID,
			ParentSourceID: leaf.ParentSourceID,
		}
	}

	// ——— Phase 1: scan all listing pages (no detail, no writes) ———
	vodLeaves, scan, err := scanLeafListings(ctx, s.Fetcher, s.BaseURL, leaves, opt.MaxPagesPerLeaf, opt.MaxRequests, 0)
	if err != nil {
		sum.Error = err.Error()
		sum.Failed++
		return sum, err
	}
	sum.LeavesScanned = scan.LeavesScanned
	sum.PagesScanned = scan.PagesScanned
	sum.ListingRequests = scan.ListingRequests
	sum.ListingItems = scan.ListingItems
	sum.RequestBudgetExhausted = scan.RequestBudgetExhausted
	sum.PaginationLoops = scan.PaginationLoops
	sum.Failed += scan.Failed
	sum.DistinctVideosDiscovered = len(vodLeaves)
	for sid, n := range scan.PerLeafPages {
		if st := perLeaf[sid]; st != nil {
			st.PagesScanned = n
		}
	}
	for sid, n := range scan.PerLeafListingItems {
		if st := perLeaf[sid]; st != nil {
			st.ListingItems = n
		}
	}

	// ——— Phase 2: multi-leaf detection before any mutation ———
	var plans []leafPlan
	for vod, disc := range vodLeaves {
		uniq := uniqueLeafSources(disc)
		if len(uniq) > 1 {
			sum.MembershipConflicts++
			sum.MultiLeafConflicts++
			sum.NeedsArchitectureReview = true
			parents := uniqueParentSources(disc, uniq)
			if len(parents) > 1 {
				sum.CrossParentMultiLeafConflicts++
			} else {
				sum.SameParentMultiLeafConflicts++
			}
			for _, id := range uniq {
				if st := perLeaf[id]; st != nil {
					st.MembershipConflict++
				}
			}
			s.Log.Error("multi-leaf membership observed; skipping assign and import",
				"source_video_id", vod, "leaf_source_ids", uniq, "parent_source_ids", parents)
			continue
		}
		if len(uniq) == 0 {
			continue
		}
		var parentLocal uint64
		var parentSrc, leafSrc string
		for _, d := range disc {
			if d.leafSourceID == uniq[0] {
				parentLocal = d.parentLocalID
				parentSrc = d.parentSourceID
				leafSrc = d.leafSourceID
				break
			}
		}
		plans = append(plans, leafPlan{
			vodID:          vod,
			leafSourceID:   leafSrc,
			parentLocalID:  parentLocal,
			parentSourceID: parentSrc,
		})
	}
	sort.Slice(plans, func(i, j int) bool {
		if plans[i].leafSourceID != plans[j].leafSourceID {
			return plans[i].leafSourceID < plans[j].leafSourceID
		}
		return plans[i].vodID < plans[j].vodID
	})

	// ——— Phase 3: classify (read-only DB) ———
	type membershipItem struct {
		plan    leafPlan
		videoID uint64
	}
	var memberships []membershipItem
	var missing []leafPlan

	for _, p := range plans {
		if err := ctx.Err(); err != nil {
			sum.Error = err.Error()
			sum.Failed++
			sum.PerLeaf = leafStatsSlice(perLeaf)
			return sum, err
		}
		v, gerr := s.Store.GetVideoBySource(olehdtv.SourceSystem, p.vodID)
		if gerr != nil {
			sum.Failed++
			s.Log.Error("GetVideoBySource failed", "source_video_id", p.vodID, "err", gerr)
			continue
		}
		st := perLeaf[p.leafSourceID]
		if v == nil {
			sum.MissingCandidates++
			if st != nil {
				st.MissingCandidates++
			}
			missing = append(missing, p)
			continue
		}
		sum.ExistingMatches++
		if v.CategoryID != p.parentLocalID {
			sum.OwnershipMismatch++
			if st != nil {
				st.OwnershipMismatch++
			}
			continue
		}
		cur := strings.TrimSpace(v.LeafSourceID)
		if cur == p.leafSourceID {
			sum.Unchanged++
			if st != nil {
				st.Unchanged++
			}
			continue
		}
		if cur != "" && cur != p.leafSourceID {
			sum.MembershipConflicts++
			sum.ExistingLeafConflicts++
			if st != nil {
				st.MembershipConflict++
			}
			continue
		}
		memberships = append(memberships, membershipItem{plan: p, videoID: v.ID})
		if st != nil {
			st.MembershipAssign++
		}
	}

	// ——— Phase 4: fair round-robin import selection (no detail yet) ———
	fair := selectFairImportCandidates(missing, opt.MaxNewPerLeaf, opt.MaxNewTotal, sum.ListingRequests, opt.MaxRequests)
	selected := fair.Selected
	sum.ImportsSkippedPerLeafCap = fair.SkippedPerLeafCap
	sum.ImportsSkippedTotalCap = fair.SkippedTotalCap
	if fair.ImportBudgetExhausted {
		sum.ImportBudgetExhausted = true
	}
	if fair.RequestBudgetExhausted {
		sum.RequestBudgetExhausted = true
	}
	for leaf, n := range fair.PerLeafSelected {
		if st := perLeaf[leaf]; st != nil {
			st.ImportsSelected = n
		}
	}
	if opt.DryRun {
		sum.MembershipWouldAssign = len(memberships)
		sum.ImportsWouldSelect = len(selected)
	} else {
		sum.ImportsSelected = len(selected)
	}

	// ——— Phase 5: mutations (real mode only) ———
	if opt.DryRun {
		sum.PerLeaf = leafStatsSlice(perLeaf)
		if sum.NeedsArchitectureReview {
			sum.Error = ErrLeafArchitectureReview.Error()
			return sum, ErrLeafArchitectureReview
		}
		return sum, nil
	}

	for _, m := range memberships {
		if err := ctx.Err(); err != nil {
			sum.Error = err.Error()
			sum.Failed++
			sum.PerLeaf = leafStatsSlice(perLeaf)
			return sum, err
		}
		if err := s.Store.SetVideoLeafSourceID(m.videoID, m.plan.leafSourceID); err != nil {
			sum.Failed++
			s.Log.Error("SetVideoLeafSourceID failed", "video_id", m.videoID, "err", err)
			continue
		}
		sum.MembershipAssigned++
	}

	catMap := parentCatMap(cats)
	for _, p := range selected {
		if err := ctx.Err(); err != nil {
			sum.Error = err.Error()
			sum.Failed++
			sum.PerLeaf = leafStatsSlice(perLeaf)
			return sum, err
		}
		if sum.ListingRequests+sum.DetailRequests >= opt.MaxRequests {
			sum.RequestBudgetExhausted = true
			break
		}
		if err := s.importOne(ctx, &sum, catMap, p); err != nil {
			sum.ImportsFailed++
			sum.Failed++
			s.Log.Error("leaf import failed", "source_video_id", p.vodID, "err", err)
			continue
		}
		sum.ImportsSucceeded++
	}

	sum.PerLeaf = leafStatsSlice(perLeaf)
	if sum.NeedsArchitectureReview {
		sum.Error = ErrLeafArchitectureReview.Error()
		return sum, ErrLeafArchitectureReview
	}
	if sum.ImportsFailed > 0 || sum.Failed > 0 {
		sum.Error = fmt.Sprintf("leaf import completed with failures (imports_failed=%d failed=%d)", sum.ImportsFailed, sum.Failed)
		return sum, fmt.Errorf("%s", sum.Error)
	}
	return sum, nil
}

func (s *LeafImportSync) importOne(
	ctx context.Context,
	sum *LeafImportSummary,
	catMap map[string]uint64,
	p leafPlan,
) error {
	sv, err := s.fetchDetailNoPlay(ctx, p.vodID)
	sum.DetailRequests++
	if err != nil {
		return err
	}
	// Force canonical parent ownership — never trust detail TypeID.
	sv.SourceCategoryID = p.parentSourceID
	sv.SourceID = p.vodID

	parentLocal := catMap[p.parentSourceID]
	if parentLocal == 0 || parentLocal != p.parentLocalID {
		return fmt.Errorf("parent category unresolved: source=%s local=%d want=%d", p.parentSourceID, parentLocal, p.parentLocalID)
	}

	res, err := ApplySourceVideoCatalog(s.Store, catMap, sv, false)
	if err != nil {
		return err
	}
	if res.VideoID == 0 {
		return fmt.Errorf("import produced no video id for %s", p.vodID)
	}
	// Verify ownership was not redirected to a child category id.
	got, gerr := s.Store.GetVideoBySource(olehdtv.SourceSystem, p.vodID)
	if gerr != nil {
		return gerr
	}
	if got == nil {
		return fmt.Errorf("video missing after import %s", p.vodID)
	}
	if got.CategoryID != p.parentLocalID {
		return fmt.Errorf("refusing leaf assign: category_id=%d want parent %d (child ownership forbidden)", got.CategoryID, p.parentLocalID)
	}
	if strings.TrimSpace(got.SourceCategoryID) != p.parentSourceID {
		return fmt.Errorf("refusing leaf assign: source_category_id=%q want %q", got.SourceCategoryID, p.parentSourceID)
	}
	// Assign leaf only after successful catalog write + ownership check.
	if err := s.Store.SetVideoLeafSourceID(got.ID, p.leafSourceID); err != nil {
		return fmt.Errorf("catalog ok but leaf assign failed: %w", err)
	}
	return nil
}

func (s *LeafImportSync) fetchDetailNoPlay(ctx context.Context, sourceID string) (olehdtv.SourceVideo, error) {
	sourceID = strings.TrimSpace(sourceID)
	if err := olehdtv.ValidateSourceID(sourceID); err != nil {
		return olehdtv.SourceVideo{}, err
	}
	durl := fmt.Sprintf("%s/index.php/vod/detail/id/%s.html", s.BaseURL, sourceID)
	body, err := s.Fetcher.Fetch(ctx, durl)
	if err != nil {
		return olehdtv.SourceVideo{}, err
	}
	card := maccms.CatalogItem{VodID: sourceID, DetailURL: durl}
	detail := maccms.MergeDetailWithCatalog(card, maccms.ParseDetailPage(body, durl))
	// No play enrichment — episodes stay pending_enrichment for enrichpending.
	for i := range detail.Episodes {
		detail.Episodes[i].StreamURL = ""
		detail.Episodes[i].EnrichmentSkipped = true
	}
	return olehdtv.SourceVideoFromDetailMeta(s.BaseURL, detail), nil
}

func parentCatMap(cats []model.Category) map[string]uint64 {
	m := make(map[string]uint64, len(cats))
	for _, c := range cats {
		if c.ParentID != nil && *c.ParentID != 0 {
			continue // never map child source ids for ownership
		}
		sid := strings.TrimSpace(c.SourceID)
		if sid == "" {
			continue
		}
		m[sid] = c.ID
	}
	return m
}

func leafStatsSlice(perLeaf map[string]*LeafImportLeafStat) []LeafImportLeafStat {
	out := make([]LeafImportLeafStat, 0, len(perLeaf))
	for _, st := range perLeaf {
		if st != nil {
			out = append(out, *st)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ParentSourceID != out[j].ParentSourceID {
			return out[i].ParentSourceID < out[j].ParentSourceID
		}
		return out[i].LeafSourceID < out[j].LeafSourceID
	})
	return out
}
