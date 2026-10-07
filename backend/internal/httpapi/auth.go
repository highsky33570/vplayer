package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/auth"
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
	if a.Cfg.CDNSignSecret != "" {
		return a.Cfg.CDNSignSecret
	}
	return "dev-auth-secret"
}

func (a *API) Register(c *gin.Context) {
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
	email := strings.TrimSpace(body.Email)
	pass := body.Password
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请输入账号或邮箱"})
		return
	}
	if len(pass) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "密码至少 6 位"})
		return
	}
	user, err := repo.CreateUser(email, pass, body.Nickname)
	if errors.Is(err, store.ErrUserExists) {
		c.JSON(http.StatusConflict, gin.H{"ok": false, "message": "该邮箱已注册"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": err.Error()})
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
			"user": gin.H{
				"id":       user.ID,
				"email":    user.Email,
				"nickname": auth.DisplayName(user.Nickname, user.Email),
			},
		},
	})
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
	email := strings.TrimSpace(body.Email)
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
			"user": gin.H{
				"id":       user.ID,
				"email":    user.Email,
				"nickname": auth.DisplayName(user.Nickname, user.Email),
			},
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
		"ok": true,
		"data": gin.H{
			"id":       user.ID,
			"email":    user.Email,
			"nickname": auth.DisplayName(user.Nickname, user.Email),
		},
	})
}
