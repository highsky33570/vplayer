package httpapi

import (
	"errors"
	"net/http"
	"time"

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
		"id":       user.ID,
		"email":    user.Email,
		"nickname": auth.DisplayName(user.Nickname, user.Email),
		"is_admin": user.IsAdmin,
	}
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
