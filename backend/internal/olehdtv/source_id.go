package olehdtv

import (
	"fmt"
	"strconv"
	"strings"
)

// ValidateSourceID requires a positive integer OLEHDTV vod id (no leading zeros required).
func ValidateSourceID(sourceID string) error {
	id := strings.TrimSpace(sourceID)
	if id == "" {
		return fmt.Errorf("source-id is required")
	}
	n, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return fmt.Errorf("source-id must be a positive integer")
	}
	if n == 0 {
		return fmt.Errorf("source-id must be > 0")
	}
	return nil
}
