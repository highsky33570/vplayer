package store

import (
	"database/sql"
	"strings"
	"time"
)

const (
	NotificationKindCommentReply      = "comment_reply"
	NotificationKindSystem            = "system"
	NotificationKindDirectMessage     = "direct_message"
	NotificationKindDirectMessageSent = "direct_message_sent"
)

// ValidNotificationKind reports whether kind is one of the supported kinds.
func ValidNotificationKind(kind string) bool {
	return kind == NotificationKindCommentReply ||
		kind == NotificationKindSystem ||
		kind == NotificationKindDirectMessage ||
		kind == NotificationKindDirectMessageSent
}

// Notification is a user_notifications row.
type Notification struct {
	ID          uint64
	UserID      uint64
	Kind        string
	Title       string
	Body        string
	VideoID     *uint64
	CommentID   *uint64
	ActorUserID *uint64
	IsRead      bool
	CreatedAt   time.Time
}

// NotificationRepo is the notification surface used by comment replies and /me/notifications.
type NotificationRepo interface {
	// CreateNotification stores n. ID and a zero CreatedAt are filled in.
	// IsRead is honored (sent-message copies are typically stored as already read).
	CreateNotification(n Notification) (Notification, error)
	// ListNotifications lists newest first. An empty kind lists every kind.
	ListNotifications(userID uint64, kind string, limit, offset int) ([]Notification, int, error)
	CountUnread(userID uint64) (int, error)
	// MarkRead marks only the caller's own notifications; foreign or unknown ids are ignored.
	MarkRead(userID uint64, ids []uint64) error
	MarkAllRead(userID uint64) error
}

const (
	notificationDefaultLimit = 20
	notificationMaxLimit     = 50
)

func optionalUint64(p *uint64) any {
	if p == nil {
		return nil
	}
	return *p
}

func (m *MySQL) CreateNotification(n Notification) (Notification, error) {
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now()
	}
	n.CreatedAt = n.CreatedAt.UTC()
	readFlag := 0
	if n.IsRead {
		readFlag = 1
	}
	res, err := m.DB.Exec(`
		INSERT INTO user_notifications (user_id, kind, title, body, video_id, comment_id, actor_user_id, is_read, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.UserID, n.Kind, n.Title, n.Body,
		optionalUint64(n.VideoID), optionalUint64(n.CommentID), optionalUint64(n.ActorUserID),
		readFlag, formatUTCDateTime(n.CreatedAt),
	)
	if err != nil {
		return Notification{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Notification{}, err
	}
	n.ID = uint64(id)
	return n, nil
}

func (m *MySQL) ListNotifications(userID uint64, kind string, limit, offset int) ([]Notification, int, error) {
	limit, offset = clampPage(limit, offset, notificationDefaultLimit, notificationMaxLimit)
	where := `user_id=?`
	args := []any{userID}
	if kind != "" {
		where += ` AND kind=?`
		args = append(args, kind)
	}
	var total int
	if err := m.DB.QueryRow(`SELECT COUNT(*) FROM user_notifications WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := m.DB.Query(`
		SELECT id, user_id, kind, title, body, video_id, comment_id, actor_user_id, is_read,
		       DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:%s')
		FROM user_notifications
		WHERE `+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		var n Notification
		var video, comment, actor sql.NullInt64
		var read int
		var createdRaw string
		if err := rows.Scan(&n.ID, &n.UserID, &n.Kind, &n.Title, &n.Body, &video, &comment, &actor, &read, &createdRaw); err != nil {
			return nil, 0, err
		}
		n.VideoID = nullUint64Ptr(video)
		n.CommentID = nullUint64Ptr(comment)
		n.ActorUserID = nullUint64Ptr(actor)
		n.IsRead = read == 1
		if n.CreatedAt, err = parseUTCDateTime(createdRaw); err != nil {
			return nil, 0, err
		}
		out = append(out, n)
	}
	return out, total, rows.Err()
}

func (m *MySQL) CountUnread(userID uint64) (int, error) {
	var n int
	err := m.DB.QueryRow(`SELECT COUNT(*) FROM user_notifications WHERE user_id=? AND is_read=0`, userID).Scan(&n)
	return n, err
}

func (m *MySQL) MarkRead(userID uint64, ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	args = append(args, userID)
	for _, id := range ids {
		args = append(args, id)
	}
	_, err := m.DB.Exec(`UPDATE user_notifications SET is_read=1 WHERE user_id=? AND is_read=0 AND id IN (`+marks+`)`, args...)
	return err
}

func (m *MySQL) MarkAllRead(userID uint64) error {
	_, err := m.DB.Exec(`UPDATE user_notifications SET is_read=1 WHERE user_id=? AND is_read=0`, userID)
	return err
}
