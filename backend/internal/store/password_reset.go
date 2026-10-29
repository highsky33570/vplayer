package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/tycdn/vplayer/internal/auth"
)

var (
	ErrPasswordResetNotFound   = errors.New("password reset not found")
	ErrPasswordResetExpired    = errors.New("password reset expired")
	ErrPasswordResetTooMany    = errors.New("too many password reset attempts")
	ErrPasswordResetConflict   = errors.New("password reset changed")
	ErrPasswordResetSendBudget = errors.New("password reset send budget exceeded")
)

// Reuse registration TTL / cooldown / attempt budgets for password reset codes.
const (
	PasswordResetCodeTTL           = PendingCodeTTL
	PasswordResetSendCooldown      = PendingSendCooldown
	PasswordResetMaxVerifyAttempts = PendingMaxVerifyAttempts
	PasswordResetMaxSendsPerRow    = PendingMaxSendsPerRow
)

// PasswordReset is a short-lived email code used to set a new password.
type PasswordReset struct {
	ID           uint64
	Email        string
	CodeHash     string
	ExpiresAt    time.Time
	AttemptCount int
	ResendCount  int
	LastSentAt   time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PasswordResetStore manages forgot-password email codes.
type PasswordResetStore interface {
	DeleteExpiredPasswordResets() error
	GetPasswordReset(email string) (*PasswordReset, error)
	UpsertPasswordReset(email, codeHash string, now time.Time) error
	RotatePasswordResetIfHash(email, expectHash, newCodeHash string, now time.Time) error
	IncrementPasswordResetAttempts(email string) (int, error)
	ConsumePasswordResetAndSetPassword(email, expectHash, passwordHash string) error
	UserEmailExists(email string) (bool, error)
}

func (m *MySQL) DeleteExpiredPasswordResets() error {
	_, err := m.DB.Exec(`DELETE FROM password_resets WHERE expires_at < UTC_TIMESTAMP()`)
	return err
}

func (m *MySQL) GetPasswordReset(email string) (*PasswordReset, error) {
	email = auth.NormalizeEmail(email)
	var p PasswordReset
	err := m.DB.QueryRow(`
		SELECT id, email, code_hash, expires_at, attempt_count, resend_count, last_sent_at, created_at, updated_at
		FROM password_resets WHERE email=? LIMIT 1`, email).
		Scan(&p.ID, &p.Email, &p.CodeHash, &p.ExpiresAt, &p.AttemptCount, &p.ResendCount, &p.LastSentAt, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPasswordResetNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (m *MySQL) UpsertPasswordReset(email, codeHash string, now time.Time) error {
	email = auth.NormalizeEmail(email)
	codeHash = strings.TrimSpace(codeHash)
	if email == "" || codeHash == "" {
		return errors.New("email and code hash required")
	}
	expires := formatUTCDateTime(now.Add(PasswordResetCodeTTL))
	sent := formatUTCDateTime(now)
	_, err := m.DB.Exec(`
		INSERT INTO password_resets
		  (email, code_hash, expires_at, attempt_count, resend_count, last_sent_at, created_at, updated_at)
		VALUES (?, ?, ?, 0, 0, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		  code_hash=VALUES(code_hash),
		  expires_at=VALUES(expires_at),
		  attempt_count=0,
		  resend_count=0,
		  last_sent_at=VALUES(last_sent_at),
		  updated_at=VALUES(updated_at)`,
		email, codeHash, expires, sent, sent, sent,
	)
	return err
}

func (m *MySQL) RotatePasswordResetIfHash(email, expectHash, newCodeHash string, now time.Time) error {
	email = auth.NormalizeEmail(email)
	expectHash = strings.TrimSpace(expectHash)
	newCodeHash = strings.TrimSpace(newCodeHash)
	if email == "" || expectHash == "" || newCodeHash == "" {
		return errors.New("email and code hashes required")
	}
	res, err := m.DB.Exec(`
		UPDATE password_resets SET
		  code_hash=?,
		  expires_at=?,
		  attempt_count=0,
		  resend_count=resend_count+1,
		  last_sent_at=?,
		  updated_at=?
		WHERE email=? AND code_hash=?`,
		newCodeHash,
		formatUTCDateTime(now.Add(PasswordResetCodeTTL)),
		formatUTCDateTime(now),
		formatUTCDateTime(now),
		email, expectHash,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrPasswordResetConflict
	}
	return nil
}

func (m *MySQL) IncrementPasswordResetAttempts(email string) (int, error) {
	email = auth.NormalizeEmail(email)
	res, err := m.DB.Exec(`UPDATE password_resets SET attempt_count=attempt_count+1, updated_at=UTC_TIMESTAMP() WHERE email=?`, email)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, ErrPasswordResetNotFound
	}
	var attempts int
	if err := m.DB.QueryRow(`SELECT attempt_count FROM password_resets WHERE email=?`, email).Scan(&attempts); err != nil {
		return 0, err
	}
	return attempts, nil
}

// ConsumePasswordResetAndSetPassword verifies the row still matches expectHash,
// updates users.password_hash, and deletes the reset row.
func (m *MySQL) ConsumePasswordResetAndSetPassword(email, expectHash, passwordHash string) error {
	email = auth.NormalizeEmail(email)
	expectHash = strings.TrimSpace(expectHash)
	passwordHash = strings.TrimSpace(passwordHash)
	if email == "" || expectHash == "" || passwordHash == "" {
		return errors.New("email, code hash, and password hash required")
	}
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var p PasswordReset
	err = tx.QueryRow(`
		SELECT id, email, code_hash, expires_at, attempt_count
		FROM password_resets WHERE email=? FOR UPDATE`, email).
		Scan(&p.ID, &p.Email, &p.CodeHash, &p.ExpiresAt, &p.AttemptCount)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPasswordResetNotFound
	}
	if err != nil {
		return err
	}
	if p.CodeHash != expectHash {
		return ErrPasswordResetConflict
	}
	if time.Now().UTC().After(p.ExpiresAt.UTC()) {
		_, _ = tx.Exec(`DELETE FROM password_resets WHERE id=?`, p.ID)
		return ErrPasswordResetExpired
	}
	if p.AttemptCount >= PasswordResetMaxVerifyAttempts {
		return ErrPasswordResetTooMany
	}

	res, err := tx.Exec(`UPDATE users SET password_hash=? WHERE email=?`, passwordHash, email)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUserNotFound
	}
	if _, err := tx.Exec(`DELETE FROM password_resets WHERE id=?`, p.ID); err != nil {
		return err
	}
	return tx.Commit()
}
