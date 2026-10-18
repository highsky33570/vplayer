package store

import (
	"sort"
	"strings"

	"github.com/tycdn/vplayer/internal/model"
)

// RelatedVideoLimit is the public maximum for GET /videos/:id/related.
const RelatedVideoLimit = 10

type relatedScored struct {
	v    model.Video
	rank int
}

// RankRelatedVideos orders public candidates and is the behavior the SQL in
// ListRelatedVideos must match:
//  1. same category and the same non-empty leaf_source_id
//  2. same category
//  3. other public videos, newest created_at then id
// The current video, inactive rows, non-ready rows, and duplicate ids are omitted.
// limit is clamped to 1..RelatedVideoLimit.
func RankRelatedVideos(current model.Video, candidates []model.Video, limit int) []model.Video {
	if limit <= 0 || limit > RelatedVideoLimit {
		limit = RelatedVideoLimit
	}
	leaf := strings.TrimSpace(current.LeafSourceID)
	seen := map[uint64]bool{current.ID: true}
	rows := make([]relatedScored, 0, len(candidates))
	for _, v := range candidates {
		if v.ID == 0 || seen[v.ID] || !publicCatalogVideo(v) {
			continue
		}
		seen[v.ID] = true
		rank := 2
		if v.CategoryID == current.CategoryID {
			rank = 1
			if leaf != "" && strings.TrimSpace(v.LeafSourceID) == leaf {
				rank = 0
			}
		}
		rows = append(rows, relatedScored{v: v, rank: rank})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].rank != rows[j].rank {
			return rows[i].rank < rows[j].rank
		}
		if !rows[i].v.CreatedAt.Equal(rows[j].v.CreatedAt) {
			return rows[i].v.CreatedAt.After(rows[j].v.CreatedAt)
		}
		return rows[i].v.ID > rows[j].v.ID
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]model.Video, len(rows))
	for i, row := range rows {
		out[i] = row.v
	}
	return out
}

func publicCatalogVideo(v model.Video) bool {
	if !v.IsActive {
		return false
	}
	return v.Status == "ready" || v.Status == "demo"
}
