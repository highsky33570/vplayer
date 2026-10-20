package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/tycdn/vplayer/internal/auth"
	"github.com/tycdn/vplayer/internal/model"
)

var (
	ErrPendingNotFound     = errors.New("pending registration not found")
	ErrPendingExpired      = errors.New("pending registration expired")
	ErrPendingTooManyTries = errors.New("too many verification attempts")
	ErrPendingConflict     = errors.New("pending registration changed")
	ErrPendingSendBudget   = errors.New("pending registration send budget exceeded")
)

const (
	PendingCodeTTL           = 10 * time.Minute
	PendingSendCooldown      = 60 * time.Second
	PendingMaxVerifyAttempts = 5
	// PendingMaxSendsPerRow caps emails for one pending email row (initial + resends).
	PendingMaxSendsPerRow = 10
)

// PendingRegistration is a pre-user email verification record.
type PendingRegistration struct {
	ID           uint64
	Email        string
	PasswordHash string
	Nickname     string
	CodeHash     string
	ExpiresAt    time.Time
	AttemptCount int
	ResendCount  int
	LastSentAt   time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PendingRegistrationStore manages pending email-verification registrations.
type PendingRegistrationStore interface {
	DeleteExpiredPendingRegistrations() error
	GetPendingRegistration(email string) (*PendingRegistration, error)
	// InsertPendingIfAbsent creates a row only when email is absent.
	// Duplicate email returns ErrPendingConflict and does not overwrite.
	InsertPendingIfAbsent(email, passwordHash, nickname, codeHash string, now time.Time) error
	// ReplacePendingIfCodeHash updates password and code only when code_hash still matches.
	// Zero rows returns ErrPendingConflict.
	ReplacePendingIfCodeHash(email, expectedCodeHash, passwordHash, nickname, newCodeHash string, now time.Time) error
	// RotatePendingCodeIfHash replaces the code only when code_hash still matches.
	// It does not change password_hash. Zero rows returns ErrPendingConflict.
	RotatePendingCodeIfHash(email, expectedCodeHash, newCodeHash string, now time.Time) error
	IncrementPendingVerifyAttempts(email string) (int, error)
	// DeletePendingIfCodeHash deletes only the row whose code_hash still matches.
	DeletePendingIfCodeHash(email, codeHash string) error
	UserEmailExists(email string) (bool, error)
	// ConsumePendingAndCreateUser deletes the pending row and inserts users in one transaction.
	ConsumePendingAndCreateUser(email, expectedCodeHash string) (*model.User, error)
}

func (m *MySQL) DeleteExpiredPendingRegistrations() error {
	// expires_at is stored as UTC wall time. Compare with UTC_TIMESTAMP(), not NOW().
	_, err := m.DB.Exec(`DELETE FROM pending_registrations WHERE expires_at < UTC_TIMESTAMP()`)
	return err
}

func (m *MySQL) GetPendingRegistration(email string) (*PendingRegistration, error) {
	email = auth.NormalizeEmail(email)
	var p PendingRegistration
	var expiresAt, lastSentAt, createdAt, updatedAt string
	err := m.DB.QueryRow(`
		SELECT id, email, password_hash, COALESCE(nickname,''), code_hash,
		       DATE_FORMAT(expires_at, '%Y-%m-%d %H:%i:%s'),
		       attempt_count, resend_count,
		       DATE_FORMAT(last_sent_at, '%Y-%m-%d %H:%i:%s'),
		       DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:%s'),
		       DATE_FORMAT(updated_at, '%Y-%m-%d %H:%i:%s')
		FROM pending_registrations WHERE email=? LIMIT 1`, email).Scan(
		&p.ID, &p.Email, &p.PasswordHash, &p.Nickname, &p.CodeHash, &expiresAt,
		&p.AttemptCount, &p.ResendCount, &lastSentAt, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPendingNotFound
	}
	if err != nil {
		return nil, err
	}
	var perr error
	if p.ExpiresAt, perr = parseUTCDateTime(expiresAt); perr != nil {
		return nil, perr
	}
	if p.LastSentAt, perr = parseUTCDateTime(lastSentAt); perr != nil {
		return nil, perr
	}
	if p.CreatedAt, perr = parseUTCDateTime(createdAt); perr != nil {
		return nil, perr
	}
	if p.UpdatedAt, perr = parseUTCDateTime(updatedAt); perr != nil {
		return nil, perr
	}
	return &p, nil
}

func (m *MySQL) InsertPendingIfAbsent(email, passwordHash, nickname, codeHash string, now time.Time) error {
	email = auth.NormalizeEmail(email)
	nickname = strings.TrimSpace(nickname)
	now = now.UTC()
	expires := formatUTCDateTime(now.Add(PendingCodeTTL))
	sent := formatUTCDateTime(now)
	_, err := m.DB.Exec(`
		INSERT INTO pending_registrations
		  (email, password_hash, nickname, code_hash, expires_at, attempt_count, resend_count, last_sent_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, 0, ?, ?, ?)`,
		email, passwordHash, nickname, codeHash, expires, sent, sent, sent,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return ErrPendingConflict
		}
		return err
	}
	return nil
}

func (m *MySQL) ReplacePendingIfCodeHash(email, expectedCodeHash, passwordHash, nickname, newCodeHash string, now time.Time) error {
	email = auth.NormalizeEmail(email)
	nickname = strings.TrimSpace(nickname)
	now = now.UTC()
	res, err := m.DB.Exec(`
		UPDATE pending_registrations SET
		  password_hash=?,
		  nickname=?,
		  code_hash=?,
		  expires_at=?,
		  attempt_count=0,
		  resend_count=resend_count+1,
		  last_sent_at=?,
		  updated_at=?
		WHERE email=? AND code_hash=?`,
		passwordHash, nickname, newCodeHash, formatUTCDateTime(now.Add(PendingCodeTTL)),
		formatUTCDateTime(now), formatUTCDateTime(now), email, expectedCodeHash,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrPendingConflict
	}
	return nil
}

func (m *MySQL) RotatePendingCodeIfHash(email, expectedCodeHash, newCodeHash string, now time.Time) error {
	email = auth.NormalizeEmail(email)
	now = now.UTC()
	res, err := m.DB.Exec(`
		UPDATE pending_registrations SET
		  code_hash=?,
		  expires_at=?,
		  attempt_count=0,
		  resend_count=resend_count+1,
		  last_sent_at=?,
		  updated_at=?
		WHERE email=? AND code_hash=?`,
		newCodeHash, formatUTCDateTime(now.Add(PendingCodeTTL)),
		formatUTCDateTime(now), formatUTCDateTime(now), email, expectedCodeHash,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrPendingConflict
	}
	return nil
}

func (m *MySQL) IncrementPendingVerifyAttempts(email string) (int, error) {
	email = auth.NormalizeEmail(email)
	tx, err := m.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var attempts int
	err = tx.QueryRow(`SELECT attempt_count FROM pending_registrations WHERE email=? FOR UPDATE`, email).Scan(&attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrPendingNotFound
	}
	if err != nil {
		return 0, err
	}
	attempts++
	if _, err := tx.Exec(`UPDATE pending_registrations SET attempt_count=?, updated_at=UTC_TIMESTAMP() WHERE email=?`, attempts, email); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return attempts, nil
}

func (m *MySQL) DeletePendingIfCodeHash(email, codeHash string) error {
	email = auth.NormalizeEmail(email)
	_, err := m.DB.Exec(`DELETE FROM pending_registrations WHERE email=? AND code_hash=?`, email, codeHash)
	return err
}

func (m *MySQL) UserEmailExists(email string) (bool, error) {
	email = auth.NormalizeEmail(email)
	var n int
	err := m.DB.QueryRow(`SELECT COUNT(1) FROM users WHERE email=?`, email).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (m *MySQL) ConsumePendingAndCreateUser(email, expectedCodeHash string) (*model.User, error) {
	email = auth.NormalizeEmail(email)
	tx, err := m.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var p PendingRegistration
	var expiresAt string
	err = tx.QueryRow(`
		SELECT id, email, password_hash, COALESCE(nickname,''), code_hash,
		       DATE_FORMAT(expires_at, '%Y-%m-%d %H:%i:%s'), attempt_count
		FROM pending_registrations WHERE email=? FOR UPDATE`, email).Scan(
		&p.ID, &p.Email, &p.PasswordHash, &p.Nickname, &p.CodeHash, &expiresAt, &p.AttemptCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPendingNotFound
	}
	if err != nil {
		return nil, err
	}
	if p.ExpiresAt, err = parseUTCDateTime(expiresAt); err != nil {
		return nil, err
	}
	if time.Now().UTC().After(p.ExpiresAt) {
		if _, err := tx.Exec(`DELETE FROM pending_registrations WHERE email=? AND code_hash=?`, email, p.CodeHash); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, ErrPendingExpired
	}
	if p.AttemptCount >= PendingMaxVerifyAttempts {
		return nil, ErrPendingTooManyTries
	}
	if p.CodeHash != expectedCodeHash {
		return nil, ErrInvalidCredentials
	}

	var exists int
	if err := tx.QueryRow(`SELECT COUNT(1) FROM users WHERE email=?`, email).Scan(&exists); err != nil {
		return nil, err
	}
	if exists > 0 {
		if _, err := tx.Exec(`DELETE FROM pending_registrations WHERE email=? AND code_hash=?`, email, p.CodeHash); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, ErrUserExists
	}

	res, err := tx.Exec(
		`INSERT INTO users (email, password_hash, nickname) VALUES (?, ?, ?)`,
		email, p.PasswordHash, p.Nickname,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			if _, derr := tx.Exec(`DELETE FROM pending_registrations WHERE email=? AND code_hash=?`, email, p.CodeHash); derr != nil {
				return nil, derr
			}
			if cerr := tx.Commit(); cerr != nil {
				return nil, cerr
			}
			return nil, ErrUserExists
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	if _, err := tx.Exec(`DELETE FROM pending_registrations WHERE email=? AND code_hash=?`, email, p.CodeHash); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &model.User{
		ID:        uint64(id),
		Email:     email,
		Nickname:  p.Nickname,
		AvatarHue: -1,
		IsAdmin:   false,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// Pending timestamps are stored as UTC wall-clock DATETIME strings so the
// driver loc=Local setting cannot reinterpret them as server-local time.
func formatUTCDateTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}

func parseUTCDateTime(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(s), time.UTC)
}
