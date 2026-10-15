package catalogsync

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tycdn/vplayer/internal/olehdtv/maccms"
)

type leafDiscovery struct {
	leafSourceID   string
	parentLocalID  uint64
	parentSourceID string
}

// leafScanCounters are filled while scanning leaf listing pages.
type leafScanCounters struct {
	LeavesScanned          int
	PagesScanned           int
	ListingRequests        int
	ListingItems           int
	RequestBudgetExhausted bool
	PaginationLoops        int
	Failed                 int
	PerLeafPages           map[string]int
	PerLeafListingItems    map[string]int
}

// scanLeafListings fetches up to maxPagesPerLeaf listing pages per leaf and
// builds source_video_id → discoveries. Stops early when maxRequests is exhausted.
// Does not fetch detail or play pages.
func scanLeafListings(
	ctx context.Context,
	fetcher PageFetcher,
	baseURL string,
	leaves []leafTarget,
	maxPagesPerLeaf int,
	maxRequests int,
	requestsSoFar int,
) (map[string][]leafDiscovery, leafScanCounters, error) {
	out := map[string][]leafDiscovery{}
	sum := leafScanCounters{
		PerLeafPages:        map[string]int{},
		PerLeafListingItems: map[string]int{},
	}
	if fetcher == nil {
		return out, sum, fmt.Errorf("fetcher required")
	}
	if maxPagesPerLeaf < 1 {
		maxPagesPerLeaf = 1
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")

	for _, leaf := range leaves {
		if err := ctx.Err(); err != nil {
			return out, sum, err
		}
		if maxRequests > 0 && requestsSoFar+sum.ListingRequests >= maxRequests {
			sum.RequestBudgetExhausted = true
			break
		}

		pageURL := maccms.TypeListingURL(baseURL, leaf.SourceID)
		if pageURL == "" {
			sum.Failed++
			continue
		}

		visited := map[string]bool{}
		leafPages := 0
		for page := 1; page <= maxPagesPerLeaf; page++ {
			if maxRequests > 0 && requestsSoFar+sum.ListingRequests >= maxRequests {
				sum.RequestBudgetExhausted = true
				break
			}
			if pageURL == "" {
				break
			}
			if maccms.DetectPaginationLoop(visited, pageURL) {
				sum.PaginationLoops++
				break
			}
			body, ferr := fetcher.Fetch(ctx, pageURL)
			sum.ListingRequests++
			maccms.MarkVisited(visited, pageURL)
			if ferr != nil {
				sum.Failed++
				break
			}
			leafPages++
			sum.PagesScanned++
			items, pag := maccms.ParseCatalogPage(body, pageURL, leaf.SourceID)
			for _, it := range items {
				vod := strings.TrimSpace(it.VodID)
				if vod == "" {
					continue
				}
				sum.ListingItems++
				sum.PerLeafListingItems[leaf.SourceID]++
				out[vod] = append(out[vod], leafDiscovery{
					leafSourceID:   leaf.SourceID,
					parentLocalID:  leaf.ParentLocalID,
					parentSourceID: leaf.ParentSourceID,
				})
			}
			// Detect loops even when NextURL == current (ParseCatalogPage sets HasNext=false then).
			if pag.NextURL != "" && maccms.DetectPaginationLoop(visited, pag.NextURL) {
				sum.PaginationLoops++
				break
			}
			if !pag.HasNext || pag.NextURL == "" || page >= maxPagesPerLeaf {
				break
			}
			pageURL = pag.NextURL
		}
		if leafPages > 0 {
			sum.LeavesScanned++
			sum.PerLeafPages[leaf.SourceID] = leafPages
		}
		if sum.RequestBudgetExhausted {
			break
		}
	}
	return out, sum, nil
}

func uniqueLeafSources(disc []leafDiscovery) []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range disc {
		id := strings.TrimSpace(d.leafSourceID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func uniqueParentSources(disc []leafDiscovery, leafIDs []string) []string {
	want := map[string]bool{}
	for _, id := range leafIDs {
		want[id] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, d := range disc {
		if !want[d.leafSourceID] {
			continue
		}
		p := strings.TrimSpace(d.parentSourceID)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
