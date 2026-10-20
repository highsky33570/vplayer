package httpapi

import (
	"errors"
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
	commentListDefault   = 20
	commentListMax       = 50
	commentBodyMaxRunes  = 1000
	commentMaxPerMinute  = 5
	commentMaxPerHour    = 30
	commentRateLimitText = "评论过于频繁，请稍后再试"
)

// PublicCommentDisplayName is the only public identity for a comment author.
// Empty nicknames become "用户". Email is never an input.
func PublicCommentDisplayName(nickname string) string {
	if n := strings.TrimSpace(nickname); n != "" {
		return n
	}
	return "用户"
}

type commentAuthorJSON struct {
	DisplayName string `json:"display_name"`
}

type commentJSON struct {
	ID        uint64            `json:"id"`
	VideoID   uint64            `json:"video_id"`
	ParentID  *uint64           `json:"parent_id,omitempty"`
	Body      string            `json:"body"`
	CreatedAt string            `json:"created_at"`
	Author    commentAuthorJSON `json:"author"`
	Mine      bool              `json:"mine"`
	Origin    string            `json:"origin"`
}

type sourceCommentJSON struct {
	Origin           string              `json:"origin"`
	Mine             bool                `json:"mine"`
	Body             string              `json:"body"`
	SourceCreatedRaw string              `json:"source_created_raw"`
	DiggUp           int                 `json:"digg_up"`
	DiggDown         int                 `json:"digg_down"`
	Author           commentAuthorJSON   `json:"author"`
	Replies          []sourceCommentJSON `json:"replies,omitempty"`
}

type sourceCommentPageJSON struct {
	Data    []sourceCommentJSON `json:"data"`
	Total   int                 `json:"total"`
	Limit   int                 `json:"limit"`
	Offset  int                 `json:"offset"`
	HasMore bool                `json:"has_more"`
	Error   string              `json:"error,omitempty"`
}

func (a *API) comments() store.CommentRepo {
	if a.Comments != nil {
		return a.Comments
	}
	if a.Store != nil {
		return a.Store
	}
	return nil
}

func (a *API) sourceComments() store.SourceCommentRepo {
	if a.SourceComments != nil {
		return a.SourceComments
	}
	if a.Store != nil {
		return a.Store
	}
	return nil
}

// PublicSourceAuthor is the public name for an imported comment.
// Empty source names become "游客". Email is never an input.
func PublicSourceAuthor(name string) string {
	if n := strings.TrimSpace(name); n != "" {
		return n
	}
	return "游客"
}

func (a *API) nowUTC() time.Time {
	if a.Clock != nil {
		return a.Clock().UTC()
	}
	return time.Now().UTC()
}

func (a *API) optionalUserID(c *gin.Context) uint64 {
	token := auth.BearerToken(c.GetHeader("Authorization"))
	if token == "" {
		return 0
	}
	claims, err := auth.ParseToken(a.authSecret(), token)
	if err != nil {
		return 0
	}
	return claims.UserID
}

func (a *API) commentJSON(c store.Comment, viewer uint64) commentJSON {
	nick := c.Nickname
	if users := a.users(); users != nil {
		if u, err := users.GetUserByID(c.UserID); err == nil && u != nil {
			nick = u.Nickname
		}
	}
	return commentJSON{
		ID:        c.ID,
		VideoID:   c.VideoID,
		ParentID:  c.ParentID,
		Body:      c.Body,
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
		Author:    commentAuthorJSON{DisplayName: PublicCommentDisplayName(nick)},
		Mine:      viewer != 0 && viewer == c.UserID,
		Origin:    "vplayer",
	}
}

func sourceThreadJSON(row store.SourceThread) sourceCommentJSON {
	out := sourceCommentJSON{
		Origin:           "source",
		Mine:             false,
		Body:             row.Body,
		SourceCreatedRaw: row.SourceCreatedRaw,
		DiggUp:           row.DiggUp,
		DiggDown:         row.DiggDown,
		Author:           commentAuthorJSON{DisplayName: PublicSourceAuthor(row.AuthorName)},
	}
	for _, reply := range row.Replies {
		out.Replies = append(out.Replies, sourceThreadJSON(reply))
	}
	return out
}

func (a *API) sourceCommentPage(videoID uint64, limit, offset int) sourceCommentPageJSON {
	page := sourceCommentPageJSON{
		Data:   []sourceCommentJSON{},
		Limit:  limit,
		Offset: offset,
	}
	repo := a.sourceComments()
	if repo == nil {
		return page
	}
	threads, parentTotal, rowTotal, err := repo.ListSourceCommentThreads(videoID, limit, offset)
	if err != nil {
		page.Error = "原站评论加载失败"
		return page
	}
	page.Total = rowTotal
	page.Data = make([]sourceCommentJSON, 0, len(threads))
	for _, thread := range threads {
		page.Data = append(page.Data, sourceThreadJSON(thread))
	}
	page.HasMore = offset+len(threads) < parentTotal
	return page
}

func (a *API) ListComments(c *gin.Context) {
	videoID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || videoID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	repo := a.comments()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "评论暂不可用"})
		return
	}
	ok, err := repo.VideoExists(videoID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "视频不存在"})
		return
	}
	limit := commentListDefault
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			limit = commentListDefault
		} else if n > commentListMax {
			limit = commentListMax
		} else {
			limit = n
		}
	}
	offset := 0
	if raw := c.Query("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err == nil && n > 0 {
			offset = n
		}
	}
	rows, total, err := repo.ListVisibleComments(videoID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	viewer := a.optionalUserID(c)
	data := make([]commentJSON, 0, len(rows))
	for _, row := range rows {
		data = append(data, a.commentJSON(row, viewer))
	}
	sourceLimit, sourceOffset := sourcePageQuery(c)
	c.JSON(http.StatusOK, gin.H{
		"ok":       true,
		"data":     data,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": offset+len(data) < total,
		"source":   a.sourceCommentPage(videoID, sourceLimit, sourceOffset),
	})
}

func sourcePageQuery(c *gin.Context) (int, int) {
	limit := commentListDefault
	if raw := c.Query("source_limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			limit = commentListDefault
		} else if n > commentListMax {
			limit = commentListMax
		} else {
			limit = n
		}
	}
	offset := 0
	if raw := c.Query("source_offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err == nil && n > 0 {
			offset = n
		}
	}
	return limit, offset
}

func (a *API) CreateComment(c *gin.Context) {
	videoID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || videoID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	uidVal, ok := c.Get(ctxAuthUserID)
	uid, uidOK := uidVal.(uint64)
	if !ok || !uidOK || uid == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "未登录"})
		return
	}
	var body struct {
		Body     string  `json:"body"`
		ParentID *uint64 `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "评论内容无效"})
		return
	}
	text := strings.TrimSpace(body.Body)
	if text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "评论不能为空"})
		return
	}
	if utf8.RuneCountInString(text) > commentBodyMaxRunes {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "评论不能超过 1000 字"})
		return
	}
	repo := a.comments()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "评论暂不可用"})
		return
	}
	exists, err := repo.VideoExists(videoID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "视频不存在"})
		return
	}
	var parent *store.Comment
	if body.ParentID != nil {
		if *body.ParentID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "回复的评论无效"})
			return
		}
		p, err := repo.GetVisibleComment(*body.ParentID)
		if errors.Is(err, store.ErrCommentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "评论不存在"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
			return
		}
		if p.VideoID != videoID {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "不能回复其他视频的评论"})
			return
		}
		parent = &p
	}
	now := a.nowUTC()
	minuteCount, err := repo.CountUserCommentsSince(uid, now.Add(-time.Minute))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	if minuteCount >= commentMaxPerMinute {
		c.JSON(http.StatusTooManyRequests, gin.H{"ok": false, "message": commentRateLimitText})
		return
	}
	hourCount, err := repo.CountUserCommentsSince(uid, now.Add(-time.Hour))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	if hourCount >= commentMaxPerHour {
		c.JSON(http.StatusTooManyRequests, gin.H{"ok": false, "message": commentRateLimitText})
		return
	}
	var parentID *uint64
	if parent != nil {
		id := parent.ID
		parentID = &id
	}
	created, err := repo.CreateComment(videoID, uid, text, now, parentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	if parent != nil {
		a.notifyCommentReply(*parent, created, uid, now)
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true, "data": a.commentJSON(created, uid)})
}

func (a *API) DeleteComment(c *gin.Context) {
	commentID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || commentID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid id"})
		return
	}
	uidVal, ok := c.Get(ctxAuthUserID)
	uid, uidOK := uidVal.(uint64)
	if !ok || !uidOK || uid == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "未登录"})
		return
	}
	repo := a.comments()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "评论暂不可用"})
		return
	}
	if err := repo.SoftDeleteComment(commentID, uid, a.nowUTC()); err != nil {
		if errors.Is(err, store.ErrCommentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "评论不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
