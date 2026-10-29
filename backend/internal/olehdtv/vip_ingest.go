package olehdtv

import "strings"

// VIPSourceCategoryIDs is parent 6 (VIP蓝光影院) plus verified leaves 601–613.
// The public VIP channel is admin-local only — never ingest these from OLEHDTV.
func VIPSourceCategoryIDs() []string {
	return []string{
		"6",
		"601", "602", "603", "604", "605", "606", "607",
		"608", "609", "610", "611", "612", "613",
	}
}

// IsVIPSourceCategoryID reports OLEHDTV VIP蓝光影院 parent "6" or verified
// children 601–613. Incremental sync must not ingest these into the public
// VIP channel; that channel is reserved for admin-local titles.
func IsVIPSourceCategoryID(sourceCategoryID string) bool {
	id := strings.TrimSpace(sourceCategoryID)
	for _, want := range VIPSourceCategoryIDs() {
		if id == want {
			return true
		}
	}
	return false
}
