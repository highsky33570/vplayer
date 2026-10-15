package store

import (
	"fmt"
	"strings"
)

// SetVideoLeafSourceID sets only videos.leaf_source_id for Phase 4 membership sync.
// It does not modify category_id, source_category_id, playback, migration, or catalog fields.
// Empty leafSourceID clears membership (SQL NULL).
func (m *MySQL) SetVideoLeafSourceID(videoID uint64, leafSourceID string) error {
	if videoID == 0 {
		return fmt.Errorf("set leaf_source_id: video id required")
	}
	_, err := m.DB.Exec(
		`UPDATE videos SET leaf_source_id=? WHERE id=?`,
		leafSourceSQL(leafSourceID), videoID,
	)
	return err
}

func leafSourceSQL(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

// setVideoLeafSourceIDQuery is the exact SQL used by SetVideoLeafSourceID (tests).
func setVideoLeafSourceIDQuery() string {
	return `UPDATE videos SET leaf_source_id=? WHERE id=?`
}
