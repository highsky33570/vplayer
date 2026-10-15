package catalogsync

import "sort"

// fairSelectResult is the outcome of round-robin import candidate selection.
type fairSelectResult struct {
	Selected               []leafPlan
	SkippedPerLeafCap      int
	SkippedTotalCap        int
	ImportBudgetExhausted  bool
	RequestBudgetExhausted bool
	PerLeafSelected        map[string]int
}

// selectFairImportCandidates allocates missing candidates fairly across leaves.
// Within each leaf, candidate order is preserved. Across leaves, one candidate
// is taken per eligible leaf per round until total/request/per-leaf limits stop.
func selectFairImportCandidates(
	missing []leafPlan,
	maxNewPerLeaf int,
	maxNewTotal int,
	listingRequests int,
	maxRequests int,
) fairSelectResult {
	out := fairSelectResult{PerLeafSelected: map[string]int{}}
	if len(missing) == 0 {
		return out
	}

	queues := map[string][]leafPlan{}
	var leafOrder []string
	seenLeaf := map[string]bool{}
	for _, p := range missing {
		if !seenLeaf[p.leafSourceID] {
			seenLeaf[p.leafSourceID] = true
			leafOrder = append(leafOrder, p.leafSourceID)
		}
		queues[p.leafSourceID] = append(queues[p.leafSourceID], p)
	}
	sort.Strings(leafOrder)

	for {
		if maxNewTotal > 0 && len(out.Selected) >= maxNewTotal {
			out.ImportBudgetExhausted = true
			break
		}
		remainingSlots := maxRequests - listingRequests - len(out.Selected)
		if maxRequests > 0 && remainingSlots <= 0 {
			out.RequestBudgetExhausted = true
			break
		}

		progressed := false
		for _, leaf := range leafOrder {
			if maxNewTotal > 0 && len(out.Selected) >= maxNewTotal {
				out.ImportBudgetExhausted = true
				break
			}
			if maxRequests > 0 && maxRequests-listingRequests-len(out.Selected) <= 0 {
				out.RequestBudgetExhausted = true
				break
			}
			if maxNewPerLeaf > 0 && out.PerLeafSelected[leaf] >= maxNewPerLeaf {
				continue
			}
			q := queues[leaf]
			if len(q) == 0 {
				continue
			}
			out.Selected = append(out.Selected, q[0])
			queues[leaf] = q[1:]
			out.PerLeafSelected[leaf]++
			progressed = true
		}
		if !progressed {
			break
		}
	}

	// Classify leftovers for reporting.
	for _, leaf := range leafOrder {
		left := len(queues[leaf])
		if left == 0 {
			continue
		}
		if maxNewPerLeaf > 0 && out.PerLeafSelected[leaf] >= maxNewPerLeaf {
			out.SkippedPerLeafCap += left
			continue
		}
		out.SkippedTotalCap += left
	}
	return out
}
