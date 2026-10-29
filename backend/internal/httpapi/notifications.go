package httpapi

import (
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/store"
)

const (
	notificationListDefault = 20
	notificationListMax     = 50
	notificationMarkMaxIDs  = 100
	notificationBodyRunes   = 100
	commentReplyTitle       = "有人回复了你的评论"
)

func (a *API) notifications() store.NotificationRepo {
	if a.Notifications != nil {
		return a.Notifications
	}
	if a.Store != nil {
		return a.Store
	}
	return nil
}

func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max]) + "…"
}

// notifyCommentReply tells the parent author about a reply. Self-replies are skipped and a
// notification failure never fails the comment itself.
func (a *API) notifyCommentReply(parent, reply store.Comment, actorID uint64, now time.Time) {
	if parent.UserID == 0 || parent.UserID == actorID {
		return
	}
	repo := a.notifications()
	if repo == nil {
		return
	}
	videoID, commentID, actor := reply.VideoID, reply.ID, actorID
	_, err := repo.CreateNotification(store.Notification{
		UserID:      parent.UserID,
		Kind:        store.NotificationKindCommentReply,
		Title:       commentReplyTitle,
		Body:        truncateRunes(reply.Body, notificationBodyRunes),
		VideoID:     &videoID,
		CommentID:   &commentID,
		ActorUserID: &actor,
		CreatedAt:   now,
	})
	if err != nil {
		log.Printf("comment reply notification: %v", err)
	}
}

type notificationJSON struct {
	ID          uint64  `json:"id"`
	Kind        string  `json:"kind"`
	Title       string  `json:"title"`
	Body        string  `json:"body"`
	VideoID     *uint64 `json:"video_id,omitempty"`
	CommentID   *uint64 `json:"comment_id,omitempty"`
	ActorUserID *uint64 `json:"actor_user_id,omitempty"`
	ActorName   string  `json:"actor_name,omitempty"`
	IsRead      bool    `json:"is_read"`
	CreatedAt   string  `json:"created_at"`
}

func (a *API) notificationJSON(n store.Notification) notificationJSON {
	out := notificationJSON{
		ID:        n.ID,
		Kind:      n.Kind,
		Title:     n.Title,
		Body:      n.Body,
		VideoID:   n.VideoID,
		CommentID: n.CommentID,
		IsRead:    n.IsRead,
		CreatedAt: n.CreatedAt.UTC().Format(time.RFC3339),
	}
	// Expose actor id for DMs (received + sent) so the UI can reply / show the other party.
	if n.Kind == store.NotificationKindDirectMessage || n.Kind == store.NotificationKindDirectMessageSent {
		out.ActorUserID = n.ActorUserID
	}
	if n.ActorUserID != nil {
		if users := a.users(); users != nil {
			if u, err := users.GetUserByID(*n.ActorUserID); err == nil && u != nil {
				out.ActorName = PublicCommentDisplayName(u.Nickname)
			}
		}
		if out.ActorName == "" {
			out.ActorName = PublicCommentDisplayName("")
		}
	}
	return out
}

const (
	welcomeTitle = "欢迎加入天涯欧乐！"
	welcomeBody  = "恭喜你成功注册！海量影视内容已为你准备好。点击本条消息可随时查看系统通知；也可在「查找用户」中按用户 ID 发送私信。祝你观影愉快！"
)

// notifyWelcomeSignup creates an unread celebration system notice for a new account.
func (a *API) notifyWelcomeSignup(userID uint64) {
	if userID == 0 {
		return
	}
	repo := a.notifications()
	if repo == nil {
		return
	}
	_, err := repo.CreateNotification(store.Notification{
		UserID:    userID,
		Kind:      store.NotificationKindSystem,
		Title:     welcomeTitle,
		Body:      welcomeBody,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		log.Printf("welcome notification: %v", err)
	}
}

// ListNotifications handles GET /api/v1/me/notifications?kind=&limit=&offset=.
func (a *API) ListNotifications(c *gin.Context) {
	uid, ok := libraryUser(c)
	if !ok {
		return
	}
	repo := a.notifications()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "通知暂不可用"})
		return
	}
	kind := strings.TrimSpace(c.Query("kind"))
	if kind != "" && !store.ValidNotificationKind(kind) {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid kind"})
		return
	}
	limit, offset := pageQuery(c, notificationListDefault, notificationListMax)
	rows, total, err := repo.ListNotifications(uid, kind, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	data := make([]notificationJSON, 0, len(rows))
	for _, row := range rows {
		data = append(data, a.notificationJSON(row))
	}
	c.JSON(http.StatusOK, pageJSON(data, len(data), total, limit, offset))
}

// UnreadNotificationCount handles GET /api/v1/me/notifications/unread-count.
func (a *API) UnreadNotificationCount(c *gin.Context) {
	uid, ok := libraryUser(c)
	if !ok {
		return
	}
	repo := a.notifications()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "通知暂不可用"})
		return
	}
	n, err := repo.CountUnread(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"unread_count": n}})
}

// MarkNotificationsRead handles POST /api/v1/me/notifications/read with {"ids":[...]} or {"all":true}.
// Ids that belong to someone else are ignored.
func (a *API) MarkNotificationsRead(c *gin.Context) {
	uid, ok := libraryUser(c)
	if !ok {
		return
	}
	var body struct {
		IDs []uint64 `json:"ids"`
		All bool     `json:"all"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请求格式错误"})
		return
	}
	if !body.All && len(body.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请指定要标记已读的通知"})
		return
	}
	if len(body.IDs) > notificationMarkMaxIDs {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "一次最多标记 100 条通知"})
		return
	}
	repo := a.notifications()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "通知暂不可用"})
		return
	}
	var err error
	if body.All {
		err = repo.MarkAllRead(uid)
	} else {
		err = repo.MarkRead(uid, body.IDs)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	unread, err := repo.CountUnread(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"unread_count": unread}})
}
