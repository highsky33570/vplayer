package olehdtv

import (
	"fmt"
	"strconv"
	"strings"
)

// Verified child type under a canonical OLEHDTV parent (source ids 1–6 only).
type verifiedChild struct {
	SourceID string
	Name     string
}

// verifiedChildrenByParent is the approved live OLEHDTV hierarchy for parents 1–6.
// Kept in the OLEHDTV backend adapter layer (not frontend). Live type-page discovery
// is not used for sync in this phase: ListCategories is home-nav-only and recent
// crawl HTTP budgets are already tight; this map is deterministic, offline-safe,
// and matches the verified inspection order.
var verifiedChildrenByParent = map[string][]verifiedChild{
	"1": { // 电影
		{"101", "动作片"}, {"102", "喜剧片"}, {"103", "爱情片"}, {"104", "科幻片"},
		{"105", "恐怖片"}, {"106", "剧情片"}, {"107", "战争片"}, {"108", "动画片"},
		{"109", "悬疑片"}, {"110", "惊悚片"}, {"111", "纪录片"}, {"112", "奇幻片"},
		{"113", "犯罪片"},
	},
	"2": { // 连续剧
		{"202", "国产剧"}, {"201", "欧美剧"}, {"203", "港台剧"}, {"204", "日韩剧"},
		{"1207", "短剧"},
	},
	"3": { // 综艺
		{"305", "真人秀"}, {"302", "音乐"}, {"304", "搞笑"}, {"301", "家庭"}, {"303", "曲艺"},
	},
	"4": { // 动漫
		{"401", "日本"}, {"402", "国产"}, {"403", "欧美"},
	},
	"5": { // 午夜影院 — never under 电影
		{"506", "AI真人"}, {"505", "AV女优"}, {"502", "日韩"},
		{"501", "欧美"}, {"503", "动漫"}, {"504", "国产"},
	},
	"6": { // VIP蓝光影院 — distinct from 101–113
		{"601", "动作片"}, {"602", "喜剧片"}, {"603", "爱情片"}, {"604", "科幻片"},
		{"605", "恐怖片"}, {"606", "剧情片"}, {"607", "战争片"}, {"608", "动画片"},
		{"609", "悬疑片"}, {"610", "惊悚片"}, {"611", "纪录片"}, {"612", "奇幻片"},
		{"613", "犯罪片"},
	},
}

// ChildCategorySlug returns the stable collision-safe slug for a child source type id.
func ChildCategorySlug(sourceID string) string {
	return "type-" + strings.TrimSpace(sourceID)
}

// IsCanonicalHierarchyParentSourceID reports whether source_id is a Phase-2 hierarchy
// parent (1–6). Parents 7/8/13/14 are excluded from leaf membership sync.
func IsCanonicalHierarchyParentSourceID(sourceID string) bool {
	switch strings.TrimSpace(sourceID) {
	case "1", "2", "3", "4", "5", "6":
		return true
	default:
		return false
	}
}

// MergeVerifiedHierarchy appends verified children for canonical parents 1–6 that
// already appear in tops. Children carry ParentSourceID; tops are unchanged.
// Does not invent parents 7/8/13/14 or attach children when the parent is absent.
func MergeVerifiedHierarchy(tops []SourceCategory) []SourceCategory {
	present := make(map[string]SourceCategory, len(tops))
	out := make([]SourceCategory, 0, len(tops)+64)
	seenChild := map[string]bool{}

	for _, c := range tops {
		sid := strings.TrimSpace(c.SourceID)
		if sid == "" {
			continue
		}
		if strings.TrimSpace(c.ParentSourceID) != "" {
			// Adapter already emitted a child — keep for two-phase sync.
			if !seenChild[sid] {
				out = append(out, c)
				seenChild[sid] = true
			}
			continue
		}
		present[sid] = c
		out = append(out, c)
	}

	for _, parentSrc := range []string{"1", "2", "3", "4", "5", "6"} {
		parent, ok := present[parentSrc]
		if !ok {
			continue
		}
		parentSort := parent.Sort
		if parentSort == 0 {
			if id, err := strconv.Atoi(parentSrc); err == nil {
				parentSort = CategorySortForSourceID(id)
			}
		}
		for i, ch := range verifiedChildrenByParent[parentSrc] {
			if seenChild[ch.SourceID] {
				continue
			}
			seenChild[ch.SourceID] = true
			out = append(out, SourceCategory{
				SourceID:       ch.SourceID,
				Name:           ch.Name,
				Slug:           ChildCategorySlug(ch.SourceID),
				Sort:           parentSort + i + 1,
				ParentSourceID: parentSrc,
			})
		}
	}
	return out
}

// ValidateChildParentRef reports whether a child may be upserted under parentSrc.
// Used by tests and defensive checks (midnight/VIP isolation).
func ValidateChildParentRef(childSourceID, parentSourceID string) error {
	childSourceID = strings.TrimSpace(childSourceID)
	parentSourceID = strings.TrimSpace(parentSourceID)
	if childSourceID == "" || parentSourceID == "" {
		return fmt.Errorf("child/parent source id required")
	}
	children, ok := verifiedChildrenByParent[parentSourceID]
	if !ok {
		return fmt.Errorf("parent source_id %s is not a canonical hierarchy parent", parentSourceID)
	}
	for _, ch := range children {
		if ch.SourceID == childSourceID {
			return nil
		}
	}
	return fmt.Errorf("child source_id %s is not a verified child of parent %s", childSourceID, parentSourceID)
}
