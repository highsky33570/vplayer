package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/auth"
	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/mail"
	"github.com/tycdn/vplayer/internal/store"
)

type fakeMailer struct {
	mu     sync.Mutex
	msgs   []mail.Message
	err    error
	failN  int // fail first N sends
	sends  int
	onSend func() error
}

func (f *fakeMailer) Send(ctx context.Context, msg mail.Message) error {
	f.mu.Lock()
	f.sends++
	failN := f.failN
	if failN > 0 {
		f.failN--
	}
	err := f.err
	hook := f.onSend
	f.mu.Unlock()
	if failN > 0 {
		if err != nil {
			return err
		}
		return errors.New("smtp failed")
	}
	if hook != nil {
		if herr := hook(); herr != nil {
			return herr
		}
	}
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.msgs = append(f.msgs, msg)
	f.mu.Unlock()
	return nil
}

func (f *fakeMailer) lastCode(t *testing.T) string {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.msgs) == 0 {
		t.Fatal("no mail sent")
	}
	body := f.msgs[len(f.msgs)-1].TextBody
	// "您的验证码是：123456"
	idx := strings.Index(body, "：")
	if idx < 0 {
		t.Fatalf("no code marker in %q", body)
	}
	rest := strings.TrimSpace(body[idx+len("："):])
	code := rest
	if i := strings.IndexAny(rest, "\r\n"); i >= 0 {
		code = rest[:i]
	}
	code = strings.TrimSpace(code)
	if !auth.IsSixDigitCode(code) {
		t.Fatalf("extracted %q from %q", code, body)
	}
	return code
}

func testRegisterRouter(mem *store.MemoryAuth, m mail.Mailer) (*gin.Engine, *API) {
	gin.SetMode(gin.TestMode)
	api := &API{
		Cfg: config.Config{
			AuthSecret:               "test-auth-secret",
			EmailCodePepper:          "test-email-pepper",
			EmailVerificationEnabled: true,
		},
		Users:       mem.MemoryUsers,
		PendingRegs: mem.MemoryPendingRegistrations,
		Mailer:      m,
	}
	r := gin.New()
	v1 := r.Group("/api/v1")
	{
		v1.GET("/auth/registration", api.RegistrationOptions)
		v1.POST("/auth/register", api.Register)
		v1.POST("/auth/register/start", api.RegisterStart)
		v1.POST("/auth/register/verify", api.RegisterVerify)
		v1.POST("/auth/register/resend", api.RegisterResend)
		v1.POST("/auth/login", api.Login)
		v1.GET("/auth/me", api.Me)
		v1.GET("/admin/me", api.RequireUser, api.RequireAdmin, api.AdminMe)
	}
	return r, api
}

func postJSON(r http.Handler, path string, body any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRegisterDirectWhenVerificationDisabled(t *testing.T) {
	mem := store.NewMemoryAuth()
	fm := &fakeMailer{err: errors.New("smtp must not be called")}
	gin.SetMode(gin.TestMode)
	api := &API{
		Cfg:         config.Config{AuthSecret: "test-auth-secret"},
		Users:       mem.MemoryUsers,
		PendingRegs: mem.MemoryPendingRegistrations,
		Mailer:      fm,
	}
	if api.Cfg.EmailVerificationEnabled {
		t.Fatal("unset flag must be false")
	}
	r := gin.New()
	v1 := r.Group("/api/v1")
	v1.GET("/auth/registration", api.RegistrationOptions)
	v1.POST("/auth/register", api.Register)
	v1.POST("/auth/login", api.Login)
	v1.GET("/auth/me", api.Me)
	v1.GET("/admin/me", api.RequireUser, api.RequireAdmin, api.AdminMe)

	opt := httptest.NewRecorder()
	r.ServeHTTP(opt, httptest.NewRequest(http.MethodGet, "/api/v1/auth/registration", nil))
	if opt.Code != http.StatusOK || !strings.Contains(opt.Body.String(), `"email_verification_enabled":false`) {
		t.Fatalf("options status=%d body=%s", opt.Code, opt.Body.String())
	}
	if strings.Contains(strings.ToLower(opt.Body.String()), "smtp") || strings.Contains(opt.Body.String(), "pepper") {
		t.Fatal("registration options must not expose mail config")
	}

	w := postJSON(r, "/api/v1/auth/register", map[string]string{
		"email": " Direct@Example.com ", "password": "password1", "nickname": "Dee", "is_admin": "true",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Token string `json:"token"`
			User  struct {
				Email   string `json:"email"`
				IsAdmin bool   `json:"is_admin"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Token == "" {
		t.Fatal("token missing")
	}
	if resp.Data.User.Email != "direct@example.com" || resp.Data.User.IsAdmin {
		t.Fatalf("user=%+v", resp.Data.User)
	}
	if fm.sends != 0 {
		t.Fatalf("smtp sends=%d", fm.sends)
	}
	if _, err := mem.GetPendingRegistration("direct@example.com"); !errors.Is(err, store.ErrPendingNotFound) {
		t.Fatalf("pending err=%v", err)
	}
	user, err := mem.Authenticate("direct@example.com", "password1")
	if err != nil || user.IsAdmin {
		t.Fatalf("authenticate err=%v admin=%v", err, user != nil && user.IsAdmin)
	}
	reqMe := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	reqMe.Header.Set("Authorization", "Bearer "+resp.Data.Token)
	wMe := httptest.NewRecorder()
	r.ServeHTTP(wMe, reqMe)
	if wMe.Code != http.StatusOK {
		t.Fatalf("me status=%d body=%s", wMe.Code, wMe.Body.String())
	}
	reqAdmin := httptest.NewRequest(http.MethodGet, "/api/v1/admin/me", nil)
	reqAdmin.Header.Set("Authorization", "Bearer "+resp.Data.Token)
	wAdmin := httptest.NewRecorder()
	r.ServeHTTP(wAdmin, reqAdmin)
	if wAdmin.Code != http.StatusForbidden {
		t.Fatalf("admin/me status=%d", wAdmin.Code)
	}

	again := postJSON(r, "/api/v1/auth/register", map[string]string{
		"email": "direct@example.com", "password": "password1",
	})
	if again.Code != http.StatusConflict {
		t.Fatalf("duplicate status=%d body=%s", again.Code, again.Body.String())
	}
}

func TestRegisterExplicitlyDisabledMatchesUnset(t *testing.T) {
	mem := store.NewMemoryAuth()
	gin.SetMode(gin.TestMode)
	api := &API{
		Cfg:   config.Config{AuthSecret: "test-auth-secret", EmailVerificationEnabled: false},
		Users: mem.MemoryUsers,
	}
	r := gin.New()
	r.POST("/api/v1/auth/register", api.Register)
	w := postJSON(r, "/api/v1/auth/register", map[string]string{
		"email": "off@example.com", "password": "password1",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if _, err := mem.Authenticate("off@example.com", "password1"); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterLegacyCannotBypassVerification(t *testing.T) {
	mem := store.NewMemoryAuth()
	r, _ := testRegisterRouter(mem, &fakeMailer{})
	w := postJSON(r, "/api/v1/auth/register", map[string]string{
		"email": "bypass@example.com", "password": "password1", "nickname": "X",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	exists, _ := mem.UserEmailExists("bypass@example.com")
	if exists {
		t.Fatal("legacy register must not create users")
	}
}

func TestRegisterStartDoesNotCreateUser(t *testing.T) {
	mem := store.NewMemoryAuth()
	fm := &fakeMailer{}
	r, _ := testRegisterRouter(mem, fm)
	w := postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "New@Example.com", "password": "password1", "nickname": "Nick",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if fm.sends != 1 {
		t.Fatalf("sends=%d", fm.sends)
	}
	exists, _ := mem.UserEmailExists("new@example.com")
	if exists {
		t.Fatal("start must not create users")
	}
	p, err := mem.GetPendingRegistration("new@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if p.PasswordHash == "password1" || strings.Contains(p.PasswordHash, "password") {
		t.Fatal("pending must store bcrypt hash only")
	}
	if auth.IsSixDigitCode(p.CodeHash) || p.CodeHash == fm.lastCode(t) {
		t.Fatal("code must not be stored plaintext")
	}
}

func TestRegisterVerifySuccessAndLogin(t *testing.T) {
	mem := store.NewMemoryAuth()
	fm := &fakeMailer{}
	r, _ := testRegisterRouter(mem, fm)
	if w := postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "ok@example.com", "password": "password1",
	}); w.Code != http.StatusOK {
		t.Fatalf("start: %s", w.Body.String())
	}
	code := fm.lastCode(t)
	w := postJSON(r, "/api/v1/auth/register/verify", map[string]string{
		"email": "ok@example.com", "code": code,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("verify status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Token string `json:"token"`
			User  struct {
				Email   string `json:"email"`
				IsAdmin bool   `json:"is_admin"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Token == "" || resp.Data.User.Email != "ok@example.com" || resp.Data.User.IsAdmin {
		t.Fatalf("%+v", resp)
	}
	_, err := mem.GetPendingRegistration("ok@example.com")
	if !errors.Is(err, store.ErrPendingNotFound) {
		t.Fatal("pending must be consumed")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+resp.Data.Token)
	wMe := httptest.NewRecorder()
	r.ServeHTTP(wMe, req)
	if wMe.Code != http.StatusOK {
		t.Fatalf("me=%d %s", wMe.Code, wMe.Body.String())
	}
	// reused code
	w2 := postJSON(r, "/api/v1/auth/register/verify", map[string]string{
		"email": "ok@example.com", "code": code,
	})
	if w2.Code == http.StatusOK {
		t.Fatal("reused code must fail")
	}
	// login still works
	wLogin := postJSON(r, "/api/v1/auth/login", map[string]string{
		"email": "ok@example.com", "password": "password1",
	})
	if wLogin.Code != http.StatusOK {
		t.Fatalf("login=%d %s", wLogin.Code, wLogin.Body.String())
	}
}

func TestRegisterWrongCodeAndAttemptLimit(t *testing.T) {
	mem := store.NewMemoryAuth()
	fm := &fakeMailer{}
	r, _ := testRegisterRouter(mem, fm)
	_ = postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "tries@example.com", "password": "password1",
	})
	for i := 0; i < store.PendingMaxVerifyAttempts-1; i++ {
		w := postJSON(r, "/api/v1/auth/register/verify", map[string]string{
			"email": "tries@example.com", "code": "000000",
		})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("i=%d status=%d", i, w.Code)
		}
	}
	w := postJSON(r, "/api/v1/auth/register/verify", map[string]string{
		"email": "tries@example.com", "code": "000000",
	})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	// correct code also blocked after max attempts
	wOK := postJSON(r, "/api/v1/auth/register/verify", map[string]string{
		"email": "tries@example.com", "code": fm.lastCode(t),
	})
	if wOK.Code == http.StatusOK {
		t.Fatal("must not verify after attempt limit")
	}
}

func TestRegisterExpiredCode(t *testing.T) {
	mem := store.NewMemoryAuth()
	fm := &fakeMailer{}
	r, _ := testRegisterRouter(mem, fm)
	_ = postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "exp@example.com", "password": "password1",
	})
	mem.TestingForceExpiresAt("exp@example.com", time.Now().Add(-time.Minute))
	w := postJSON(r, "/api/v1/auth/register/verify", map[string]string{
		"email": "exp@example.com", "code": fm.lastCode(t),
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestRegisterResendCooldownAndInvalidatesOldCode(t *testing.T) {
	mem := store.NewMemoryAuth()
	fm := &fakeMailer{}
	r, _ := testRegisterRouter(mem, fm)
	_ = postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "rs@example.com", "password": "password1",
	})
	old := fm.lastCode(t)
	wCool := postJSON(r, "/api/v1/auth/register/resend", map[string]string{"email": "rs@example.com"})
	if wCool.Code != http.StatusTooManyRequests {
		t.Fatalf("cooldown status=%d body=%s", wCool.Code, wCool.Body.String())
	}
	mem.TestingForceLastSentAt("rs@example.com", time.Now().Add(-2*time.Minute))
	w := postJSON(r, "/api/v1/auth/register/resend", map[string]string{"email": "rs@example.com"})
	if w.Code != http.StatusOK {
		t.Fatalf("resend=%d %s", w.Code, w.Body.String())
	}
	newCode := fm.lastCode(t)
	if newCode == old {
		t.Fatal("expected different code")
	}
	wOld := postJSON(r, "/api/v1/auth/register/verify", map[string]string{
		"email": "rs@example.com", "code": old,
	})
	if wOld.Code == http.StatusOK {
		t.Fatal("old code must be invalid")
	}
	wNew := postJSON(r, "/api/v1/auth/register/verify", map[string]string{
		"email": "rs@example.com", "code": newCode,
	})
	if wNew.Code != http.StatusOK {
		t.Fatalf("new code verify=%d %s", wNew.Code, wNew.Body.String())
	}
}

func TestRegisterDuplicateExistingUser(t *testing.T) {
	mem := store.NewMemoryAuth()
	fm := &fakeMailer{}
	r, _ := testRegisterRouter(mem, fm)
	if _, err := mem.CreateUser("dup@example.com", "password1", ""); err != nil {
		t.Fatal(err)
	}
	w := postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "dup@example.com", "password": "password1",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	if fm.sends != 0 {
		t.Fatal("must not email code for existing user")
	}
	_, err := mem.GetPendingRegistration("dup@example.com")
	if !errors.Is(err, store.ErrPendingNotFound) {
		t.Fatal("must not create pending for existing user")
	}
}

func TestRegisterSMTPStartFailure(t *testing.T) {
	mem := store.NewMemoryAuth()
	fm := &fakeMailer{err: errors.New("smtp down")}
	r, _ := testRegisterRouter(mem, fm)
	w := postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "fail@example.com", "password": "password1",
	})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	_, err := mem.GetPendingRegistration("fail@example.com")
	if !errors.Is(err, store.ErrPendingNotFound) {
		t.Fatal("pending must not remain after SMTP failure")
	}
}

func TestRegisterSMTPResendFailurePreservesOldCode(t *testing.T) {
	mem := store.NewMemoryAuth()
	fm := &fakeMailer{}
	r, _ := testRegisterRouter(mem, fm)
	_ = postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "rfail@example.com", "password": "password1",
	})
	oldCode := fm.lastCode(t)
	before, err := mem.GetPendingRegistration("rfail@example.com")
	if err != nil {
		t.Fatal(err)
	}
	mem.TestingForceLastSentAt("rfail@example.com", time.Now().UTC().Add(-2*time.Minute))
	before, _ = mem.GetPendingRegistration("rfail@example.com")
	fm.err = errors.New("smtp down")
	w := postJSON(r, "/api/v1/auth/register/resend", map[string]string{"email": "rfail@example.com"})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	after, err := mem.GetPendingRegistration("rfail@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if after.CodeHash != before.CodeHash || after.AttemptCount != before.AttemptCount || after.ResendCount != before.ResendCount {
		t.Fatalf("pending mutated: before=%+v after=%+v", before, after)
	}
	if !after.ExpiresAt.Equal(before.ExpiresAt) {
		t.Fatalf("expiry changed: %v vs %v", before.ExpiresAt, after.ExpiresAt)
	}
	fm.err = nil
	wOK := postJSON(r, "/api/v1/auth/register/verify", map[string]string{
		"email": "rfail@example.com", "code": oldCode,
	})
	if wOK.Code != http.StatusOK {
		t.Fatalf("old code must remain valid: %d %s", wOK.Code, wOK.Body.String())
	}
}

func TestRegisterMalformedCode(t *testing.T) {
	mem := store.NewMemoryAuth()
	fm := &fakeMailer{}
	r, _ := testRegisterRouter(mem, fm)
	_ = postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "bad@example.com", "password": "password1",
	})
	for _, code := range []string{"12345", "1234567", "abcdef", ""} {
		w := postJSON(r, "/api/v1/auth/register/verify", map[string]string{
			"email": "bad@example.com", "code": code,
		})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("code=%q status=%d", code, w.Code)
		}
	}
}

func TestRegisterEmailNormalization(t *testing.T) {
	mem := store.NewMemoryAuth()
	fm := &fakeMailer{}
	r, _ := testRegisterRouter(mem, fm)
	_ = postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "  Mix@Example.COM ", "password": "password1",
	})
	code := fm.lastCode(t)
	w := postJSON(r, "/api/v1/auth/register/verify", map[string]string{
		"email": "mix@example.com", "code": code,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
}

func TestRegisterExistingAdminLoginUnaffected(t *testing.T) {
	mem := store.NewMemoryAuth()
	fm := &fakeMailer{}
	r, _ := testRegisterRouter(mem, fm)
	u, err := mem.CreateUser("admin@example.com", "password1", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.SetIsAdmin(u.ID, true); err != nil {
		t.Fatal(err)
	}
	w := postJSON(r, "/api/v1/auth/login", map[string]string{
		"email": "admin@example.com", "password": "password1",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login=%d", w.Code)
	}
	var resp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/me", nil)
	req.Header.Set("Authorization", "Bearer "+resp.Data.Token)
	wMe := httptest.NewRecorder()
	r.ServeHTTP(wMe, req)
	if wMe.Code != http.StatusOK {
		t.Fatalf("admin/me=%d %s", wMe.Code, wMe.Body.String())
	}
}

func TestRegisterMailNotConfigured(t *testing.T) {
	mem := store.NewMemoryAuth()
	gin.SetMode(gin.TestMode)
	api := &API{
		Cfg:         config.Config{AuthSecret: "x", EmailCodePepper: ""},
		Users:       mem.MemoryUsers,
		PendingRegs: mem.MemoryPendingRegistrations,
		Mailer:      nil,
	}
	r := gin.New()
	r.POST("/api/v1/auth/register/start", api.RegisterStart)
	w := postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "x@example.com", "password": "password1",
	})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestRegisterStartSMTPFailurePreservesExistingPending(t *testing.T) {
	mem := store.NewMemoryAuth()
	fm := &fakeMailer{}
	r, _ := testRegisterRouter(mem, fm)
	if w := postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "keep@example.com", "password": "password1",
	}); w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	oldCode := fm.lastCode(t)
	before, err := mem.GetPendingRegistration("keep@example.com")
	if err != nil {
		t.Fatal(err)
	}
	mem.TestingForceLastSentAt("keep@example.com", time.Now().UTC().Add(-2*time.Minute))
	before, _ = mem.GetPendingRegistration("keep@example.com")
	fm.err = errors.New("smtp down")
	w := postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "keep@example.com", "password": "password2",
	})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
	after, err := mem.GetPendingRegistration("keep@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if after.CodeHash != before.CodeHash || after.PasswordHash != before.PasswordHash {
		t.Fatal("failed start must not replace pending state")
	}
	fm.err = nil
	if w := postJSON(r, "/api/v1/auth/register/verify", map[string]string{
		"email": "keep@example.com", "code": oldCode,
	}); w.Code != http.StatusOK {
		t.Fatalf("old code: %d %s", w.Code, w.Body.String())
	}
}

func TestRegisterStartDoesNotOverwriteNewerPending(t *testing.T) {
	mem := store.NewMemoryAuth()
	fm := &fakeMailer{}
	r, _ := testRegisterRouter(mem, fm)
	if w := postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "race-start@example.com", "password": "password1",
	}); w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	mem.TestingForceLastSentAt("race-start@example.com", time.Now().UTC().Add(-2*time.Minute))
	newerHash := "newer-code-hash-not-the-inflight-one"
	fm.onSend = func() error {
		p, err := mem.GetPendingRegistration("race-start@example.com")
		if err != nil {
			return err
		}
		return mem.RotatePendingCodeIfHash("race-start@example.com", p.CodeHash, newerHash, time.Now().UTC())
	}
	w := postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "race-start@example.com", "password": "password2",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
	after, err := mem.GetPendingRegistration("race-start@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if after.CodeHash != newerHash {
		t.Fatalf("newer hash overwritten: %s", after.CodeHash)
	}
	if after.PasswordHash == "" {
		t.Fatal("pending password missing")
	}
}

func TestRegisterResendDoesNotOverwriteNewerCode(t *testing.T) {
	mem := store.NewMemoryAuth()
	fm := &fakeMailer{}
	r, _ := testRegisterRouter(mem, fm)
	_ = postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "race-resend@example.com", "password": "password1",
	})
	mem.TestingForceLastSentAt("race-resend@example.com", time.Now().UTC().Add(-2*time.Minute))
	newerHash := "resend-newer-hash"
	fm.onSend = func() error {
		p, err := mem.GetPendingRegistration("race-resend@example.com")
		if err != nil {
			return err
		}
		return mem.RotatePendingCodeIfHash("race-resend@example.com", p.CodeHash, newerHash, time.Now().UTC())
	}
	w := postJSON(r, "/api/v1/auth/register/resend", map[string]string{"email": "race-resend@example.com"})
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
	after, err := mem.GetPendingRegistration("race-resend@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if after.CodeHash != newerHash {
		t.Fatalf("got %s", after.CodeHash)
	}
}

func TestRegisterVerifyCleansStalePendingWhenUserExists(t *testing.T) {
	mem := store.NewMemoryAuth()
	fm := &fakeMailer{}
	r, _ := testRegisterRouter(mem, fm)
	_ = postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "stale@example.com", "password": "password1",
	})
	code := fm.lastCode(t)
	if _, err := mem.CreateUser("stale@example.com", "other-pass", "Already"); err != nil {
		t.Fatal(err)
	}
	w := postJSON(r, "/api/v1/auth/register/verify", map[string]string{
		"email": "stale@example.com", "code": code,
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
	if _, err := mem.GetPendingRegistration("stale@example.com"); !errors.Is(err, store.ErrPendingNotFound) {
		t.Fatal("stale pending must be removed")
	}
}

func TestRegisterExpiryBoundary(t *testing.T) {
	mem := store.NewMemoryAuth()
	fm := &fakeMailer{}
	r, _ := testRegisterRouter(mem, fm)
	_ = postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "edge@example.com", "password": "password1",
	})
	code := fm.lastCode(t)
	mem.TestingForceExpiresAt("edge@example.com", time.Now().UTC().Add(2*time.Second))
	if w := postJSON(r, "/api/v1/auth/register/verify", map[string]string{
		"email": "edge@example.com", "code": code,
	}); w.Code != http.StatusOK {
		t.Fatalf("just before expiry: %d %s", w.Code, w.Body.String())
	}

	_ = postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "edge2@example.com", "password": "password1",
	})
	code2 := fm.lastCode(t)
	mem.TestingForceExpiresAt("edge2@example.com", time.Now().UTC().Add(-time.Second))
	w := postJSON(r, "/api/v1/auth/register/verify", map[string]string{
		"email": "edge2@example.com", "code": code2,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("just after expiry: %d %s", w.Code, w.Body.String())
	}
}

func TestRegisterConcurrentVerifyOnlyOneSucceeds(t *testing.T) {
	mem := store.NewMemoryAuth()
	fm := &fakeMailer{}
	r, _ := testRegisterRouter(mem, fm)
	_ = postJSON(r, "/api/v1/auth/register/start", map[string]string{
		"email": "race@example.com", "password": "password1",
	})
	code := fm.lastCode(t)
	var wg sync.WaitGroup
	results := make(chan int, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := postJSON(r, "/api/v1/auth/register/verify", map[string]string{
				"email": "race@example.com", "code": code,
			})
			results <- w.Code
		}()
	}
	wg.Wait()
	close(results)
	ok := 0
	for c := range results {
		if c == http.StatusOK {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("ok count=%d want 1", ok)
	}
}
