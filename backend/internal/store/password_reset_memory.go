package store

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/tycdn/vplayer/internal/auth"
)

// MemoryPasswordResets is an in-memory PasswordResetStore for tests.
type MemoryPasswordResets struct {
	mu    sync.Mutex
	rows  map[string]*PasswordReset
	Users *MemoryUsers
	next  uint64
}

func NewMemoryPasswordResets(users *MemoryUsers) *MemoryPasswordResets {
	return &MemoryPasswordResets{
		rows:  map[string]*PasswordReset{},
		Users: users,
		next:  1,
	}
}

func (m *MemoryPasswordResets) DeleteExpiredPasswordResets() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	for email, p := range m.rows {
		if now.After(p.ExpiresAt.UTC()) {
			delete(m.rows, email)
		}
	}
	return nil
}

func (m *MemoryPasswordResets) GetPasswordReset(email string) (*PasswordReset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	email = auth.NormalizeEmail(email)
	p, ok := m.rows[email]
	if !ok {
		return nil, ErrPasswordResetNotFound
	}
	cp := *p
	return &cp, nil
}

func (m *MemoryPasswordResets) UpsertPasswordReset(email, codeHash string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	email = auth.NormalizeEmail(email)
	codeHash = strings.TrimSpace(codeHash)
	if email == "" || codeHash == "" {
		return errors.New("email and code hash required")
	}
	id := m.next
	if existing, ok := m.rows[email]; ok {
		id = existing.ID
	} else {
		m.next++
	}
	m.rows[email] = &PasswordReset{
		ID:           id,
		Email:        email,
		CodeHash:     codeHash,
		ExpiresAt:    now.Add(PasswordResetCodeTTL),
		AttemptCount: 0,
		ResendCount:  0,
		LastSentAt:   now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	return nil
}

func (m *MemoryPasswordResets) RotatePasswordResetIfHash(email, expectHash, newCodeHash string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	email = auth.NormalizeEmail(email)
	p, ok := m.rows[email]
	if !ok || p.CodeHash != strings.TrimSpace(expectHash) {
		return ErrPasswordResetConflict
	}
	p.CodeHash = strings.TrimSpace(newCodeHash)
	p.ExpiresAt = now.Add(PasswordResetCodeTTL)
	p.AttemptCount = 0
	p.ResendCount++
	p.LastSentAt = now
	p.UpdatedAt = now
	return nil
}

func (m *MemoryPasswordResets) IncrementPasswordResetAttempts(email string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	email = auth.NormalizeEmail(email)
	p, ok := m.rows[email]
	if !ok {
		return 0, ErrPasswordResetNotFound
	}
	p.AttemptCount++
	p.UpdatedAt = time.Now().UTC()
	return p.AttemptCount, nil
}

func (m *MemoryPasswordResets) ConsumePasswordResetAndSetPassword(email, expectHash, passwordHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	email = auth.NormalizeEmail(email)
	p, ok := m.rows[email]
	if !ok {
		return ErrPasswordResetNotFound
	}
	if p.CodeHash != strings.TrimSpace(expectHash) {
		return ErrPasswordResetConflict
	}
	if time.Now().UTC().After(p.ExpiresAt.UTC()) {
		delete(m.rows, email)
		return ErrPasswordResetExpired
	}
	if p.AttemptCount >= PasswordResetMaxVerifyAttempts {
		return ErrPasswordResetTooMany
	}
	if m.Users == nil {
		return ErrUserNotFound
	}
	if err := m.Users.UpdatePasswordHashLocked(email, passwordHash); err != nil {
		return err
	}
	delete(m.rows, email)
	return nil
}

func (m *MemoryPasswordResets) UserEmailExists(email string) (bool, error) {
	if m.Users == nil {
		return false, nil
	}
	return m.Users.EmailExists(email)
}
