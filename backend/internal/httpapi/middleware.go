package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/auth"
)

const (
	ctxAuthUserID = "auth_user_id"
	ctxAuthEmail  = "auth_email"
)

// RequireUser rejects requests without a valid user Bearer token (HTTP 401).
func (a *API) RequireUser(c *gin.Context) {
	token := auth.BearerToken(c.GetHeader("Authorization"))
	if token == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "未登录"})
		return
	}
	claims, err := auth.ParseToken(a.authSecret(), token)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "登录已失效，请重新登录"})
		return
	}
	c.Set(ctxAuthUserID, claims.UserID)
	c.Set(ctxAuthEmail, claims.Email)
	c.Next()
}

// playbackRequiresUser is true only when account auth and playback auth are both on.
// PLAYBACK_AUTH_REQUIRED=false opens play while favorites/history/account stay gated.
// AUTH_REQUIRED=false (dormant guest mode) also leaves play public.
func (a *API) playbackRequiresUser() bool {
	return a.Cfg.AuthRequired && a.Cfg.PlaybackAuthRequired
}

// RequireUserWhenPlaybackAuthRequired guards play routes only. Admin routes do not use this.
func (a *API) RequireUserWhenPlaybackAuthRequired(c *gin.Context) {
	if !a.playbackRequiresUser() {
		c.Next()
		return
	}
	a.RequireUser(c)
}

// RequireAdmin must run after RequireUser. It loads the current user from the
// database and allows only users with is_admin=true (HTTP 403 otherwise).
// Admin status is never taken from the client or token claims.
func (a *API) RequireAdmin(c *gin.Context) {
	uidVal, ok := c.Get(ctxAuthUserID)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "未登录"})
		return
	}
	userID, ok := uidVal.(uint64)
	if !ok || userID == 0 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "未登录"})
		return
	}
	repo := a.users()
	if repo == nil {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "认证服务暂不可用"})
		return
	}
	user, err := repo.GetUserByID(userID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"ok": false, "message": "用户不存在或已失效"})
		return
	}
	if !user.IsAdmin {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"ok": false, "message": "需要管理员权限"})
		return
	}
	c.Set(ctxAuthEmail, user.Email)
	c.Next()
}
