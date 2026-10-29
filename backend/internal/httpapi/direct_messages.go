package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/auth"
	"github.com/tycdn/vplayer/internal/store"
)

const (
	directMessageTitlePrefix = "收到私信"
	directMessageMaxRunes    = 500
	directMessageTitleRunes  = 40
)

type publicUserJSON struct {
	ID        uint64 `json:"id"`
	Nickname  string `json:"nickname"`
	AvatarHue int    `json:"avatar_hue"`
}

// LookupPublicUser GET /api/v1/users/:id/public — find a user by platform ID for messaging.
func (a *API) LookupPublicUser(c *gin.Context) {
	if _, ok := libraryUser(c); !ok {
		return
	}
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请输入有效的用户 ID"})
		return
	}
	repo := a.users()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "用户服务暂不可用"})
		return
	}
	u, err := repo.GetUserByID(id)
	if err != nil || u == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "未找到该用户"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok": true,
		"data": publicUserJSON{
			ID:        u.ID,
			Nickname:  auth.DisplayName(u.Nickname, u.Email),
			AvatarHue: u.AvatarHue,
		},
	})
}

type sendDirectMessageBody struct {
	ToUserID uint64 `json:"to_user_id"`
	Title    string `json:"title"`
	Body     string `json:"body"`
}

// SendDirectMessage POST /api/v1/me/messages — send a DM (creates an unread notice for the recipient).
func (a *API) SendDirectMessage(c *gin.Context) {
	fromID, ok := libraryUser(c)
	if !ok {
		return
	}
	var req sendDirectMessageBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请求格式错误"})
		return
	}
	if req.ToUserID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请指定接收用户"})
		return
	}
	if req.ToUserID == fromID {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "不能给自己发私信"})
		return
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请输入私信内容"})
		return
	}
	if utf8.RuneCountInString(body) > directMessageMaxRunes {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "私信内容最多 500 字"})
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = directMessageTitlePrefix
	}
	if utf8.RuneCountInString(title) > directMessageTitleRunes {
		title = truncateRunes(title, directMessageTitleRunes)
	}

	users := a.users()
	notifs := a.notifications()
	if users == nil || notifs == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "私信服务暂不可用"})
		return
	}
	toUser, err := users.GetUserByID(req.ToUserID)
	if err != nil || toUser == nil {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "未找到该用户"})
		return
	}
	fromUser, err := users.GetUserByID(fromID)
	if err != nil || fromUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "未登录"})
		return
	}
	actor := fromID
	display := auth.DisplayName(fromUser.Nickname, fromUser.Email)
	n, err := notifs.CreateNotification(store.Notification{
		UserID:      req.ToUserID,
		Kind:        store.NotificationKindDirectMessage,
		Title:       title,
		Body:        body,
		ActorUserID: &actor,
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "发送失败，请稍后重试"})
		return
	}
	out := a.notificationJSON(n)
	if out.ActorName == "" {
		out.ActorName = display
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": out})
}
