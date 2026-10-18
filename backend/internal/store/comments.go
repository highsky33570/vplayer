package store

import (
	"database/sql"
	"errors"
	"time"
)

// ErrCommentNotFound is returned when a comment is missing or not owned by the caller.
// Callers must not distinguish those cases.
var ErrCommentNotFound = errors.New("comment not found")

// Comment is an internal comment row. Nickname comes from a user join and is not a comment column.
type Comment struct {
	ID        uint64
	VideoID   uint64
	UserID    uint64
	Body      string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
	Nickname  string
}

// CommentRepo is the comment surface used by the public and authenticated comment APIs.
type CommentRepo interface {
	VideoExists(id uint64) (bool, error)
	ListVisibleComments(videoID uint64, limit, offset int) ([]Comment, int, error)
	CountUserCommentsSince(userID uint64, since time.Time) (int, error)
	CreateComment(videoID, userID uint64, body string, now time.Time) (Comment, error)
	SoftDeleteComment(commentID, userID uint64, now time.Time) error
}

const (
	// listVisibleCommentsSQL selects nickname only. Email, password_hash, and is_admin stay out.
	// DATE_FORMAT keeps the UTC wall clock. parseTime=true would otherwise turn DATETIME
	// into a local time.Time, and scanning that into a string is not the UTC layout we store.
	listVisibleCommentsSQL = `
		SELECT c.id, c.video_id, c.user_id, c.body, c.status,
		       DATE_FORMAT(c.created_at, '%Y-%m-%d %H:%i:%s'),
		       DATE_FORMAT(c.updated_at, '%Y-%m-%d %H:%i:%s'),
		       COALESCE(u.nickname,'')
		FROM video_comments c
		LEFT JOIN users u ON u.id = c.user_id
		WHERE c.video_id=? AND c.status='visible'
		ORDER BY c.created_at DESC, c.id DESC
		LIMIT ? OFFSET ?`
	countVisibleCommentsSQL = `
		SELECT COUNT(*) FROM video_comments WHERE video_id=? AND status='visible'`
	// countUserCommentsSinceSQL counts every status so a soft delete does not refill the quota.
	countUserCommentsSinceSQL = `
		SELECT COUNT(*) FROM video_comments WHERE user_id=? AND created_at>=?`
	insertCommentSQL = `
		INSERT INTO video_comments (video_id, user_id, body, status, created_at, updated_at)
		VALUES (?, ?, ?, 'visible', ?, ?)`
	softDeleteCommentSQL = `
		UPDATE video_comments SET status='deleted', updated_at=?
		WHERE id=? AND user_id=? AND status='visible'`
)

func (m *MySQL) VideoExists(id uint64) (bool, error) {
	var one int
	err := m.DB.QueryRow(`SELECT 1 FROM videos WHERE id=? LIMIT 1`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (m *MySQL) ListVisibleComments(videoID uint64, limit, offset int) ([]Comment, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var total int
	if err := m.DB.QueryRow(countVisibleCommentsSQL, videoID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := m.DB.Query(listVisibleCommentsSQL, videoID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Comment
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

func (m *MySQL) CountUserCommentsSince(userID uint64, since time.Time) (int, error) {
	var n int
	err := m.DB.QueryRow(countUserCommentsSinceSQL, userID, formatUTCDateTime(since)).Scan(&n)
	return n, err
}

func (m *MySQL) CreateComment(videoID, userID uint64, body string, now time.Time) (Comment, error) {
	now = now.UTC()
	stamp := formatUTCDateTime(now)
	res, err := m.DB.Exec(insertCommentSQL, videoID, userID, body, stamp, stamp)
	if err != nil {
		return Comment{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Comment{}, err
	}
	c := Comment{
		ID:        uint64(id),
		VideoID:   videoID,
		UserID:    userID,
		Body:      body,
		Status:    "visible",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if u, err := m.GetUserByID(userID); err == nil && u != nil {
		c.Nickname = u.Nickname
	}
	return c, nil
}

func (m *MySQL) SoftDeleteComment(commentID, userID uint64, now time.Time) error {
	res, err := m.DB.Exec(softDeleteCommentSQL, formatUTCDateTime(now.UTC()), commentID, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrCommentNotFound
	}
	return nil
}

func scanComment(row scannable) (Comment, error) {
	var c Comment
	var createdRaw, updatedRaw string
	err := row.Scan(&c.ID, &c.VideoID, &c.UserID, &c.Body, &c.Status, &createdRaw, &updatedRaw, &c.Nickname)
	if err != nil {
		return Comment{}, err
	}
	created, err := parseUTCDateTime(createdRaw)
	if err != nil {
		return Comment{}, err
	}
	updated, err := parseUTCDateTime(updatedRaw)
	if err != nil {
		return Comment{}, err
	}
	c.CreatedAt = created
	c.UpdatedAt = updated
	return c, nil
}
