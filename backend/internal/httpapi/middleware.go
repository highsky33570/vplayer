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
