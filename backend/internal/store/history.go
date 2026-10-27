package store

import (
	"time"

	"github.com/tycdn/vplayer/internal/model"
)

// HistoryVideo is a watch-history row joined with its video.
type HistoryVideo struct {
	Video         model.Video
	LastWatchedAt time.Time
	ProgressSec   int
}

// HistoryRepo is the watch-history surface used by the /me/history APIs.
type HistoryRepo interface {
	// HistoryVideoExists reports whether the video row exists (any status).
	HistoryVideoExists(videoID uint64) (bool, error)
	// UpsertHistory refreshes last_watched_at. progressSec < 0 leaves the stored progress unchanged
	// (a new row starts at 0).
	UpsertHistory(userID, videoID uint64, progressSec int, now time.Time) error
	ListHistory(userID uint64, limit, offset int, includeMembership bool) ([]HistoryVideo, int, error)
	DeleteHistory(userID, videoID uint64) error
	ClearHistory(userID uint64) error
	CountHistory(userID uint64) (int, error)
}

const (
	historyDefaultLimit = 24
	historyMaxLimit     = 100
)

func (m *MySQL) HistoryVideoExists(videoID uint64) (bool, error) {
	return m.VideoExists(videoID)
}

func (m *MySQL) UpsertHistory(userID, videoID uint64, progressSec int, now time.Time) error {
	stamp := formatUTCDateTime(now.UTC())
	initial := progressSec
	if initial < 0 {
		initial = 0
	}
	_, err := m.DB.Exec(`
		INSERT INTO user_watch_history (user_id, video_id, last_watched_at, progress_sec)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		  last_watched_at=VALUES(last_watched_at),
		  progress_sec=IF(?>=0, ?, progress_sec)`,
		userID, videoID, stamp, initial, progressSec, progressSec,
	)
	return err
}

func (m *MySQL) ListHistory(userID uint64, limit, offset int, includeMembership bool) ([]HistoryVideo, int, error) {
	limit, offset = clampPage(limit, offset, historyDefaultLimit, historyMaxLimit)
	where := `h.user_id=? AND ` + libraryVideoVisible + libraryMembershipVisible(includeMembership)
	var total int
	if err := m.DB.QueryRow(`SELECT COUNT(*) FROM user_watch_history h JOIN videos v ON v.id = h.video_id WHERE `+where, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := m.DB.Query(`SELECT `+libraryVideoColumns+`,
		       DATE_FORMAT(h.last_watched_at, '%Y-%m-%d %H:%i:%s'), h.progress_sec
		FROM user_watch_history h
		JOIN videos v ON v.id = h.video_id
		WHERE `+where+`
		ORDER BY h.last_watched_at DESC, h.id DESC
		LIMIT ? OFFSET ?`, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []HistoryVideo{}
	for rows.Next() {
		var watchedRaw string
		var progress int
		v, err := scanVideoWithExtra(rows, &watchedRaw, &progress)
		if err != nil {
			return nil, 0, err
		}
		at, err := parseUTCDateTime(watchedRaw)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, HistoryVideo{Video: *v, LastWatchedAt: at, ProgressSec: progress})
	}
	return out, total, rows.Err()
}

func (m *MySQL) DeleteHistory(userID, videoID uint64) error {
	_, err := m.DB.Exec(`DELETE FROM user_watch_history WHERE user_id=? AND video_id=?`, userID, videoID)
	return err
}

func (m *MySQL) ClearHistory(userID uint64) error {
	_, err := m.DB.Exec(`DELETE FROM user_watch_history WHERE user_id=?`, userID)
	return err
}

func (m *MySQL) CountHistory(userID uint64) (int, error) {
	var n int
	err := m.DB.QueryRow(`
		SELECT COUNT(*) FROM user_watch_history h
		JOIN videos v ON v.id = h.video_id
		WHERE h.user_id=? AND `+libraryVideoVisible, userID).Scan(&n)
	return n, err
}
