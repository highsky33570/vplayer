package httpapi

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/auth"
	"github.com/tycdn/vplayer/internal/mail"
	"github.com/tycdn/vplayer/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const (
	passwordForgotIPLimit  = 20
	passwordForgotIPWindow = time.Hour
	passwordResetExpireSec = int(store.PasswordResetCodeTTL / time.Second)
	passwordResetResendSec = int(store.PasswordResetSendCooldown / time.Second)
)

func (a *API) passwordResetStore() store.PasswordResetStore {
	if a.PasswordResets != nil {
		return a.PasswordResets
	}
	if a.Store != nil {
		return a.Store
	}
	return nil
}

func (a *API) passwordResetMailReady() bool {
	return a.registrationMailReady()
}

func (a *API) passwordResetMeta() gin.H {
	return gin.H{
		"expires_in_sec":   passwordResetExpireSec,
		"resend_after_sec": passwordResetResendSec,
	}
}

// PasswordForgot POST /api/v1/auth/password/forgot
// Always returns a generic success when the email is well-formed so callers
// cannot probe whether an account exists. Sends a code only when the user exists.
func (a *API) PasswordForgot(c *gin.Context) {
	resets := a.passwordResetStore()
	users := a.users()
	if resets == nil || users == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "认证服务暂不可用"})
		return
	}
	if !a.passwordResetMailReady() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "邮件服务暂不可用，请稍后重试"})
		return
	}

	var body struct {
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请求格式错误"})
		return
	}
	email := auth.NormalizeEmail(body.Email)
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请输入账号或邮箱"})
		return
	}

	now := time.Now().UTC()
	if a.registerIP == nil {
		a.registerIP = newRegisterIPLimiter()
	}
	if !a.registerIP.allow(c.ClientIP(), now) {
		c.JSON(http.StatusTooManyRequests, gin.H{"ok": false, "message": "请求过于频繁，请稍后再试"})
		return
	}

	_ = resets.DeleteExpiredPasswordResets()

	exists, err := resets.UserEmailExists(email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "发送失败，请稍后重试"})
		return
	}
	// Do not reveal whether the account exists.
	if !exists {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": a.passwordResetMeta()})
		return
	}

	if existing, gerr := resets.GetPasswordReset(email); gerr == nil {
		if now.Sub(existing.LastSentAt.UTC()) < store.PasswordResetSendCooldown {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"ok":      false,
				"message": "发送过于频繁，请稍后再试",
				"data": gin.H{
					"resend_after_sec": remainingSec(existing.LastSentAt.UTC(), store.PasswordResetSendCooldown, now),
				},
			})
			return
		}
		if existing.ResendCount+1 >= store.PasswordResetMaxSendsPerRow && now.Before(existing.ExpiresAt.UTC()) {
			c.JSON(http.StatusTooManyRequests, gin.H{"ok": false, "message": "验证码发送次数已达上限，请稍后再试"})
			return
		}
	}

	code, err := auth.GenerateVerificationCode()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "发送失败，请稍后重试"})
		return
	}
	codeHash := auth.HashVerificationCode(code, a.Cfg.EmailCodePepper)

	if err := a.Mailer.Send(c.Request.Context(), mail.PasswordResetEmail(email, code)); err != nil {
		log.Printf("password/forgot smtp send failed host=%s port=%d: %v", a.Cfg.SMTPHost, a.Cfg.SMTPPort, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "验证码发送失败，请稍后重试"})
		return
	}

	now = time.Now().UTC()
	if err := resets.UpsertPasswordReset(email, codeHash, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "发送失败，请稍后重试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": a.passwordResetMeta()})
}

// PasswordReset POST /api/v1/auth/password/reset
// Verifies the email code and sets a new password. Returns a login token on success.
func (a *API) PasswordReset(c *gin.Context) {
	resets := a.passwordResetStore()
	users := a.users()
	if resets == nil || users == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "认证服务暂不可用"})
		return
	}
	if strings.TrimSpace(a.Cfg.EmailCodePepper) == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "认证服务暂不可用，请稍后重试"})
		return
	}

	var body struct {
		Email    string `json:"email"`
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请求格式错误"})
		return
	}
	email := auth.NormalizeEmail(body.Email)
	code := strings.TrimSpace(body.Code)
	pass := body.Password
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请输入账号或邮箱"})
		return
	}
	if !auth.IsSixDigitCode(code) {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请输入 6 位数字验证码"})
		return
	}
	if len(pass) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "密码至少 6 位"})
		return
	}

	_ = resets.DeleteExpiredPasswordResets()

	p, err := resets.GetPasswordReset(email)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "验证码无效或已过期，请重新获取"})
		return
	}
	if time.Now().UTC().After(p.ExpiresAt.UTC()) {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "验证码已过期，请重新获取"})
		return
	}
	if p.AttemptCount >= store.PasswordResetMaxVerifyAttempts {
		c.JSON(http.StatusTooManyRequests, gin.H{"ok": false, "message": "验证次数过多，请重新获取验证码"})
		return
	}
	if !auth.VerifyVerificationCode(code, p.CodeHash, a.Cfg.EmailCodePepper) {
		attempts, ierr := resets.IncrementPasswordResetAttempts(email)
		if ierr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "验证码错误"})
			return
		}
		if attempts >= store.PasswordResetMaxVerifyAttempts {
			c.JSON(http.StatusTooManyRequests, gin.H{"ok": false, "message": "验证次数过多，请重新获取验证码"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "验证码错误"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "重置失败，请稍后重试"})
		return
	}
	if err := resets.ConsumePasswordResetAndSetPassword(email, p.CodeHash, string(hash)); err != nil {
		switch {
		case errors.Is(err, store.ErrPasswordResetExpired):
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "验证码已过期，请重新获取"})
		case errors.Is(err, store.ErrPasswordResetTooMany):
			c.JSON(http.StatusTooManyRequests, gin.H{"ok": false, "message": "验证次数过多，请重新获取验证码"})
		case errors.Is(err, store.ErrPasswordResetNotFound), errors.Is(err, store.ErrPasswordResetConflict):
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "验证码无效或已过期，请重新获取"})
		case errors.Is(err, store.ErrUserNotFound):
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "账号不存在"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "重置失败，请稍后重试"})
		}
		return
	}

	user, err := users.Authenticate(email, pass)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"reset": true}})
		return
	}
	token, err := auth.IssueToken(a.authSecret(), user.ID, user.Email, 30*24*time.Hour)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"reset": true}})
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
