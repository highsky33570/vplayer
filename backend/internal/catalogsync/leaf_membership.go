package catalogsync

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/olehdtv"
)

// Hard bounds for leaf membership enrichment (membership-only mode).
const (
	LeafMembershipMaxPagesPerLeaf = 1
	LeafMembershipConcurrency     = 1
	// LeafMembershipMaxRequests caps listing GETs per run (~45 verified children).
	LeafMembershipMaxRequests = 64
)

// LeafMembershipStore is the DB surface for membership enrichment (mockable).
type LeafMembershipStore interface {
	ListCategories() ([]model.Category, error)
	GetVideoBySource(system, sourceID string) (*model.Video, error)
	SetVideoLeafSourceID(videoID uint64, leafSourceID string) error
}

// LeafMembershipSummary is the machine-readable report for syncleafmembership.
type LeafMembershipSummary struct {
	DryRun                  bool   `json:"dry_run"`
	BaseURL                 string `json:"base_url,omitempty"`
	LeavesSelected          int    `json:"leaves_selected"`
	LeavesScanned           int    `json:"leaves_scanned"`
	Requests                int    `json:"requests"`
	DetailRequests          int    `json:"detail_requests"`
	ListingItems            int    `json:"listing_items"`
	LocalMatches            int    `json:"local_matches"`
	Assigned                int    `json:"assigned"`
	Unchanged               int    `json:"unchanged"`
	MissingLocal            int    `json:"missing_local"`
	OwnershipMismatch       int    `json:"ownership_mismatch"`
	MembershipConflicts     int    `json:"membership_conflicts"`
	Failed                  int    `json:"failed"`
	NeedsArchitectureReview bool   `json:"needs_architecture_review"`
	LimitLeaves             int    `json:"limit_leaves,omitempty"`
	Error                   string `json:"error,omitempty"`
}

// leafTarget is a persisted child category eligible for membership sync.
type leafTarget struct {
	LocalID        uint64
	SourceID       string
	Name           string
	ParentLocalID  uint64
	ParentSourceID string
}

// LeafMembershipSync enriches videos.leaf_source_id from bounded leaf listings.
type LeafMembershipSync struct {
	Store   LeafMembershipStore
	Fetcher PageFetcher
	BaseURL string
	Log     *slog.Logger
}

func NewLeafMembershipSync(st LeafMembershipStore, fetcher PageFetcher, baseURL string) *LeafMembershipSync {
	return &LeafMembershipSync{
		Store:   st,
		Fetcher: fetcher,
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Log:     slog.Default().With("component", "leaf-membership"),
	}
}

// SelectLeafTargets returns persisted children under canonical parents 1–6 only.
// Unknown/noncanonical parents are omitted (fail closed). Sorted by source_id.
func SelectLeafTargets(cats []model.Category) []leafTarget {
	byID := make(map[uint64]model.Category, len(cats))
	for _, c := range cats {
		byID[c.ID] = c
	}
	var out []leafTarget
	for _, c := range cats {
		if c.ParentID == nil || *c.ParentID == 0 {
			continue
		}
		parent, ok := byID[*c.ParentID]
		if !ok {
			continue
		}
		parentSrc := strings.TrimSpace(parent.SourceID)
		childSrc := strings.TrimSpace(c.SourceID)
		if childSrc == "" || !olehdtv.IsCanonicalHierarchyParentSourceID(parentSrc) {
			continue
		}
		out = append(out, leafTarget{
			LocalID:        c.ID,
			SourceID:       childSrc,
			Name:           c.Name,
			ParentLocalID:  parent.ID,
			ParentSourceID: parentSrc,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ParentSourceID != out[j].ParentSourceID {
			return out[i].ParentSourceID < out[j].ParentSourceID
		}
		return out[i].SourceID < out[j].SourceID
	})
	return out
}

// Run performs bounded leaf listing inspection and optional membership writes.
// limitLeaves <= 0 means all selected leaves (still capped by MaxRequests).
// Page-1 only; no imports; no detail requests.
func (s *LeafMembershipSync) Run(ctx context.Context, dryRun bool, limitLeaves int) (LeafMembershipSummary, error) {
	sum := LeafMembershipSummary{DryRun: dryRun, BaseURL: s.BaseURL, DetailRequests: 0}
	if limitLeaves > 0 {
		sum.LimitLeaves = limitLeaves
	}
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
	if limitLeaves > 0 && len(leaves) > limitLeaves {
		leaves = leaves[:limitLeaves]
	}
	if len(leaves) > LeafMembershipMaxRequests {
		leaves = leaves[:LeafMembershipMaxRequests]
	}
	sum.LeavesSelected = len(leaves)

	vodLeaves, scan, err := scanLeafListings(ctx, s.Fetcher, s.BaseURL, leaves, LeafMembershipMaxPagesPerLeaf, LeafMembershipMaxRequests, 0)
	if err != nil {
		sum.Error = err.Error()
		sum.Failed++
		return sum, err
	}
	sum.LeavesScanned = scan.LeavesScanned
	sum.Requests = scan.ListingRequests
	sum.ListingItems = scan.ListingItems
	sum.Failed += scan.Failed

	// Detect multi-leaf ambiguity before any write (no last-writer-wins).
	type plan struct {
		vodID         string
		leafSourceID  string
		parentLocalID uint64
	}
	var plans []plan
	for vod, disc := range vodLeaves {
		uniq := uniqueLeafSources(disc)
		if len(uniq) > 1 {
			sum.MembershipConflicts++
			sum.NeedsArchitectureReview = true
			s.Log.Error("multi-leaf membership observed; skipping assignment",
				"source_video_id", vod, "leaf_source_ids", uniq)
			continue
		}
		if len(uniq) == 0 {
			continue
		}
		var parentLocal uint64
		for _, d := range disc {
			if d.leafSourceID == uniq[0] {
				parentLocal = d.parentLocalID
				break
			}
		}
		plans = append(plans, plan{vodID: vod, leafSourceID: uniq[0], parentLocalID: parentLocal})
	}
	sort.Slice(plans, func(i, j int) bool {
		if plans[i].leafSourceID != plans[j].leafSourceID {
			return plans[i].leafSourceID < plans[j].leafSourceID
		}
		return plans[i].vodID < plans[j].vodID
	})

	for _, p := range plans {
		if err := ctx.Err(); err != nil {
			sum.Error = err.Error()
			sum.Failed++
			return sum, err
		}
		v, gerr := s.Store.GetVideoBySource(olehdtv.SourceSystem, p.vodID)
		if gerr != nil {
			sum.Failed++
			s.Log.Error("GetVideoBySource failed", "source_video_id", p.vodID, "err", gerr)
			continue
		}
		if v == nil {
			sum.MissingLocal++
			continue
		}
		sum.LocalMatches++
		if v.CategoryID != p.parentLocalID {
			sum.OwnershipMismatch++
			s.Log.Info("ownership mismatch; skip leaf membership",
				"source_video_id", p.vodID,
				"leaf_source_id", p.leafSourceID,
				"local_category_id", v.CategoryID,
				"expected_parent_id", p.parentLocalID,
			)
			continue
		}
		cur := strings.TrimSpace(v.LeafSourceID)
		if cur == p.leafSourceID {
			sum.Unchanged++
			continue
		}
		if cur != "" && cur != p.leafSourceID {
			sum.MembershipConflicts++
			s.Log.Error("existing leaf_source_id conflict; not overwriting",
				"source_video_id", p.vodID,
				"existing", cur,
				"discovered", p.leafSourceID,
			)
			continue
		}
		if dryRun {
			sum.Assigned++
			continue
		}
		if err := s.Store.SetVideoLeafSourceID(v.ID, p.leafSourceID); err != nil {
			sum.Failed++
			s.Log.Error("SetVideoLeafSourceID failed", "video_id", v.ID, "err", err)
			continue
		}
		sum.Assigned++
	}

	if sum.NeedsArchitectureReview {
		if sum.Error == "" {
			sum.Error = "multi-leaf membership observed; architecture review required (single leaf_source_id)"
		}
		return sum, fmt.Errorf("%s", sum.Error)
	}
	return sum, nil
}
