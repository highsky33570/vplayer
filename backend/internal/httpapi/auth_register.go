package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/auth"
	"github.com/tycdn/vplayer/internal/mail"
	"github.com/tycdn/vplayer/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const (
	registerStartIPLimit      = 20
	registerStartIPWindow     = time.Hour
	registerVerifyRequiredSec = int(store.PendingCodeTTL / time.Second)
	registerResendAfterSec    = int(store.PendingSendCooldown / time.Second)
)

type registerIPLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newRegisterIPLimiter() *registerIPLimiter {
	return &registerIPLimiter{hits: map[string][]time.Time{}}
}

func (l *registerIPLimiter) allow(ip string, now time.Time) bool {
	if l == nil || ip == "" {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := now.Add(-registerStartIPWindow)
	arr := l.hits[ip]
	n := 0
	for _, t := range arr {
		if t.After(cut) {
			arr[n] = t
			n++
		}
	}
	arr = arr[:n]
	if len(arr) >= registerStartIPLimit {
		l.hits[ip] = arr
		return false
	}
	l.hits[ip] = append(arr, now)
	return true
}

func (a *API) pendingStore() store.PendingRegistrationStore {
	if a.PendingRegs != nil {
		return a.PendingRegs
	}
	if a.Store != nil {
		return a.Store
	}
	return nil
}

func (a *API) registrationMailReady() bool {
	if a.Mailer == nil {
		return false
	}
	return strings.TrimSpace(a.Cfg.EmailCodePepper) != ""
}

func (a *API) registerStartMeta() gin.H {
	return gin.H{
		"verification_required": true,
		"expires_in_sec":        registerVerifyRequiredSec,
		"resend_after_sec":      registerResendAfterSec,
	}
}

// RegistrationOptions exposes only whether email verification is required.
// It must not include SMTP settings or secrets.
func (a *API) RegistrationOptions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"ok": true,
		"data": gin.H{
			"email_verification_enabled": a.Cfg.EmailVerificationEnabled,
			"auth_required":              a.Cfg.AuthRequired,
		},
	})
}

// Register creates a user immediately when email verification is disabled.
// When EMAIL_VERIFICATION_ENABLED=true it refuses to create a user or token;
// clients must use register/start and register/verify.
func (a *API) Register(c *gin.Context) {
	if a.Cfg.EmailVerificationEnabled {
		c.JSON(http.StatusBadRequest, gin.H{
			"ok":      false,
			"message": "请使用邮箱验证注册：先完成 /api/v1/auth/register/start，再调用 /api/v1/auth/register/verify",
		})
		return
	}

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
			"user":  authUserJSON(user),
		},
	})
}

func (a *API) RegisterStart(c *gin.Context) {
	pending := a.pendingStore()
	if pending == nil || a.users() == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "认证服务暂不可用"})
		return
	}
	if !a.registrationMailReady() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "注册邮件服务暂不可用，请稍后重试"})
		return
	}

	var body authBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请求格式错误"})
		return
	}
	email := auth.NormalizeEmail(body.Email)
	pass := body.Password
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请输入账号或邮箱"})
		return
	}
	if len(pass) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "密码至少 6 位"})
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

	_ = pending.DeleteExpiredPendingRegistrations()

	exists, err := pending.UserEmailExists(email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "注册失败，请稍后重试"})
		return
	}
	if exists {
		// Anti-enumeration: same UX metadata; no pending row / no email with code.
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": a.registerStartMeta()})
		return
	}

	// Read only. SMTP failure must not mutate this row.
	// Activation happens only after a successful send, and only if code_hash is unchanged.
	var prior *store.PendingRegistration
	if existing, gerr := pending.GetPendingRegistration(email); gerr == nil {
		prior = existing
		if now.Before(existing.ExpiresAt.UTC()) {
			if now.Sub(existing.LastSentAt.UTC()) < store.PendingSendCooldown {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"ok":      false,
					"message": "发送过于频繁，请稍后再试",
					"data": gin.H{
						"resend_after_sec": remainingSec(existing.LastSentAt.UTC(), store.PendingSendCooldown, now),
					},
				})
				return
			}
			if existing.ResendCount+1 >= store.PendingMaxSendsPerRow {
				c.JSON(http.StatusTooManyRequests, gin.H{"ok": false, "message": "验证码发送次数已达上限，请稍后再试"})
				return
			}
		}
	} else if !errors.Is(gerr, store.ErrPendingNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "注册失败，请稍后重试"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "注册失败，请稍后重试"})
		return
	}
	code, err := auth.GenerateVerificationCode()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "注册失败，请稍后重试"})
		return
	}
	codeHash := auth.HashVerificationCode(code, a.Cfg.EmailCodePepper)

	// Send before any write. A failed send leaves the previous pending row untouched.
	if err := a.Mailer.Send(c.Request.Context(), mail.VerificationEmail(email, code)); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "验证码发送失败，请稍后重试"})
		return
	}

	now = time.Now().UTC()
	if prior == nil {
		err = pending.InsertPendingIfAbsent(email, string(hash), body.Nickname, codeHash, now)
	} else {
		// Replace only the exact version we read. A newer concurrent row is left alone.
		// If this row was expired, the same CAS still refuses to overwrite a newer hash.
		err = pending.ReplacePendingIfCodeHash(email, prior.CodeHash, string(hash), body.Nickname, codeHash, now)
	}
	if errors.Is(err, store.ErrPendingConflict) {
		c.JSON(http.StatusConflict, gin.H{"ok": false, "message": "注册状态已变化，请重新获取验证码"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "注册失败，请稍后重试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": a.registerStartMeta()})
}

func (a *API) RegisterVerify(c *gin.Context) {
	pending := a.pendingStore()
	if pending == nil || a.users() == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "认证服务暂不可用"})
		return
	}
	if strings.TrimSpace(a.Cfg.EmailCodePepper) == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "注册服务暂不可用，请稍后重试"})
		return
	}

	var body struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请求格式错误"})
		return
	}
	email := auth.NormalizeEmail(body.Email)
	code := strings.TrimSpace(body.Code)
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请输入账号或邮箱"})
		return
	}
	if !auth.IsSixDigitCode(code) {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "请输入 6 位数字验证码"})
		return
	}

	_ = pending.DeleteExpiredPendingRegistrations()

	p, err := pending.GetPendingRegistration(email)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "验证码无效或已过期，请重新获取"})
		return
	}
	if time.Now().UTC().After(p.ExpiresAt.UTC()) {
		_ = pending.DeletePendingIfCodeHash(email, p.CodeHash)
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "验证码已过期，请重新获取"})
		return
	}
	if p.AttemptCount >= store.PendingMaxVerifyAttempts {
		c.JSON(http.StatusTooManyRequests, gin.H{"ok": false, "message": "验证次数过多，请重新获取验证码"})
		return
	}
	if !auth.VerifyVerificationCode(code, p.CodeHash, a.Cfg.EmailCodePepper) {
		attempts, ierr := pending.IncrementPendingVerifyAttempts(email)
		if ierr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "验证码错误"})
			return
		}
		if attempts >= store.PendingMaxVerifyAttempts {
			c.JSON(http.StatusTooManyRequests, gin.H{"ok": false, "message": "验证次数过多，请重新获取验证码"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "验证码错误"})
		return
	}

	user, err := pending.ConsumePendingAndCreateUser(email, p.CodeHash)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrUserExists):
			c.JSON(http.StatusConflict, gin.H{"ok": false, "message": "该邮箱已注册"})
		case errors.Is(err, store.ErrPendingExpired):
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "验证码已过期，请重新获取"})
		case errors.Is(err, store.ErrPendingTooManyTries):
			c.JSON(http.StatusTooManyRequests, gin.H{"ok": false, "message": "验证次数过多，请重新获取验证码"})
		case errors.Is(err, store.ErrPendingNotFound), errors.Is(err, store.ErrInvalidCredentials):
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "验证码无效或已过期，请重新获取"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "注册失败，请稍后重试"})
		}
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

func (a *API) RegisterResend(c *gin.Context) {
	pending := a.pendingStore()
	if pending == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "认证服务暂不可用"})
		return
	}
	if !a.registrationMailReady() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "注册邮件服务暂不可用，请稍后重试"})
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

	_ = pending.DeleteExpiredPendingRegistrations()

	exists, err := pending.UserEmailExists(email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "发送失败，请稍后重试"})
		return
	}
	if exists {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": a.registerStartMeta()})
		return
	}

	p, err := pending.GetPendingRegistration(email)
	if err != nil {
		// Do not leak whether a pending registration exists.
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": a.registerStartMeta()})
		return
	}
	now := time.Now().UTC()
	if now.After(p.ExpiresAt.UTC()) {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "验证码已过期，请重新注册"})
		return
	}
	if now.Sub(p.LastSentAt.UTC()) < store.PendingSendCooldown {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"ok":      false,
			"message": "发送过于频繁，请稍后再试",
			"data": gin.H{
				"resend_after_sec": remainingSec(p.LastSentAt.UTC(), store.PendingSendCooldown, now),
			},
		})
		return
	}
	if p.ResendCount+1 >= store.PendingMaxSendsPerRow {
		c.JSON(http.StatusTooManyRequests, gin.H{"ok": false, "message": "验证码发送次数已达上限，请稍后再试"})
		return
	}

	code, err := auth.GenerateVerificationCode()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "发送失败，请稍后重试"})
		return
	}
	codeHash := auth.HashVerificationCode(code, a.Cfg.EmailCodePepper)
	// Failed send must not change the existing code, expiry, or attempt counters.
	if err := a.Mailer.Send(c.Request.Context(), mail.VerificationEmail(email, code)); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "message": "验证码发送失败，请稍后重试"})
		return
	}
	if err := pending.RotatePendingCodeIfHash(email, p.CodeHash, codeHash, time.Now().UTC()); err != nil {
		if errors.Is(err, store.ErrPendingConflict) {
			c.JSON(http.StatusConflict, gin.H{"ok": false, "message": "验证码已更新，请使用最新邮件中的验证码或稍后重试"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "message": "发送失败，请稍后重试"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": a.registerStartMeta()})
}

func remainingSec(last time.Time, cooldown time.Duration, now time.Time) int {
	left := int(last.Add(cooldown).Sub(now).Seconds())
	if left < 1 {
		return 1
	}
	return left
}
