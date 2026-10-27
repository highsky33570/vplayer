package olehdtv

import "strings"

// IsVIPSourceCategoryID reports OLEHDTV VIP蓝光影院 parent "6" or verified
// children 601–613. Incremental sync must not ingest these into the public
// VIP channel; that channel is reserved for admin-local titles.
func IsVIPSourceCategoryID(sourceCategoryID string) bool {
	switch strings.TrimSpace(sourceCategoryID) {
	case "6",
		"601", "602", "603", "604", "605", "606", "607",
		"608", "609", "610", "611", "612", "613":
		return true
	default:
		return false
	}
}
