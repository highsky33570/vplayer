package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/auth"
	"github.com/tycdn/vplayer/internal/model"
	"github.com/tycdn/vplayer/internal/store"
)

type authBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
}

func (a *API) users() store.UserRepo {
	if a.Users != nil {
		return a.Users
	}
	if a.Store != nil {
		return a.Store
	}
	return nil
}

func (a *API) authSecret() string {
	if a.Cfg.AuthSecret != "" {
		return a.Cfg.AuthSecret
	}
	return "dev-auth-secret"
}

func (a *API) Login(c *gin.Context) {
	repo := a.users()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "认证服务暂不可用"})
		return
	}
	var body authBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请求格式错误"})
		return
	}
	email := auth.NormalizeEmail(body.Email)
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请输入账号或邮箱"})
		return
	}
	if body.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请输入密码"})
		return
	}
	user, err := repo.Authenticate(email, body.Password)
	if errors.Is(err, store.ErrInvalidCredentials) {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "账号或密码错误"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "登录失败，请稍后重试"})
		return
	}
	token, err := auth.IssueToken(a.authSecret(), user.ID, user.Email, 30*24*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "签发登录凭证失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok": true,
		"data": gin.H{
			"token": token,
			"user":  authUserJSON(user),
		},
	})
}

func (a *API) Me(c *gin.Context) {
	repo := a.users()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "认证服务暂不可用"})
		return
	}
	token := auth.BearerToken(c.GetHeader("Authorization"))
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "未登录"})
		return
	}
	claims, err := auth.ParseToken(a.authSecret(), token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "登录已失效，请重新登录"})
		return
	}
	user, err := repo.GetUserByID(claims.UserID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "用户不存在或已失效"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":   true,
		"data": authUserJSON(user),
	})
}

func authUserJSON(user *model.User) gin.H {
	return gin.H{
		"id":         user.ID,
		"email":      user.Email,
		"nickname":   auth.DisplayName(user.Nickname, user.Email),
		"avatar_hue": user.AvatarHue,
		"is_admin":   user.IsAdmin,
		"is_member":  user.IsMember || user.IsAdmin,
	}
}

const (
	nicknameMaxRunes = 64
	avatarHueMin     = -1
	avatarHueMax     = 359
)

// UpdateMe handles PATCH /api/v1/auth/me (RequireUser). Only nickname and avatar_hue are accepted.
func (a *API) UpdateMe(c *gin.Context) {
	uid, ok := authUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "未登录"})
		return
	}
	repo := a.users()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "认证服务暂不可用"})
		return
	}
	var body struct {
		Nickname  *string `json:"nickname"`
		AvatarHue *int    `json:"avatar_hue"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请求格式错误"})
		return
	}
	if body.Nickname == nil && body.AvatarHue == nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "没有可更新的内容"})
		return
	}
	user, err := repo.GetUserByID(uid)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "用户不存在或已失效"})
		return
	}
	nickname := user.Nickname
	hue := user.AvatarHue
	if body.Nickname != nil {
		nickname = strings.TrimSpace(*body.Nickname)
		if nickname == "" {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "昵称不能为空"})
			return
		}
		if utf8.RuneCountInString(nickname) > nicknameMaxRunes {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "昵称不能超过 64 个字符"})
			return
		}
	}
	if body.AvatarHue != nil {
		if *body.AvatarHue < avatarHueMin || *body.AvatarHue > avatarHueMax {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "头像颜色无效"})
			return
		}
		hue = *body.AvatarHue
	}
	updated, err := repo.UpdateProfile(uid, nickname, hue)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "用户不存在或已失效"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "更新资料失败，请稍后重试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": authUserJSON(updated)})
}

// authUserIDFromContext reads the user id set by RequireUser. It never reads client input.
func authUserIDFromContext(c *gin.Context) (uint64, bool) {
	v, ok := c.Get(ctxAuthUserID)
	if !ok {
		return 0, false
	}
	uid, ok := v.(uint64)
	if !ok || uid == 0 {
		return 0, false
	}
	return uid, true
}

// AdminMe returns the authenticated admin user (RequireUser + RequireAdmin).
func (a *API) AdminMe(c *gin.Context) {
	repo := a.users()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "认证服务暂不可用"})
		return
	}
	uidVal, ok := c.Get(ctxAuthUserID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "未登录"})
		return
	}
	userID, ok := uidVal.(uint64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "未登录"})
		return
	}
	user, err := repo.GetUserByID(userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "用户不存在或已失效"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":   true,
		"data": authUserJSON(user),
	})
}
