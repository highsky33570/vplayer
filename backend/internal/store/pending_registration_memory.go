package store

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/tycdn/vplayer/internal/auth"
	"github.com/tycdn/vplayer/internal/model"
)

// MemoryPendingRegistrations is an in-memory PendingRegistrationStore for tests.
// Pair with *MemoryUsers via NewMemoryAuth().
type MemoryPendingRegistrations struct {
	mu     sync.Mutex
	byMail map[string]*PendingRegistration
	Users  *MemoryUsers
}

// MemoryAuth combines users + pending registration for handler tests.
type MemoryAuth struct {
	*MemoryUsers
	*MemoryPendingRegistrations
}

func NewMemoryAuth() *MemoryAuth {
	users := NewMemoryUsers()
	pending := &MemoryPendingRegistrations{
		byMail: map[string]*PendingRegistration{},
		Users:  users,
	}
	return &MemoryAuth{MemoryUsers: users, MemoryPendingRegistrations: pending}
}

func (m *MemoryPendingRegistrations) DeleteExpiredPendingRegistrations() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	for k, p := range m.byMail {
		if now.After(p.ExpiresAt.UTC()) {
			delete(m.byMail, k)
		}
	}
	return nil
}

func (m *MemoryPendingRegistrations) GetPendingRegistration(email string) (*PendingRegistration, error) {
	email = auth.NormalizeEmail(email)
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.byMail[email]
	if !ok {
		return nil, ErrPendingNotFound
	}
	cp := *p
	return &cp, nil
}

func (m *MemoryPendingRegistrations) InsertPendingIfAbsent(email, passwordHash, nickname, codeHash string, now time.Time) error {
	email = auth.NormalizeEmail(email)
	nickname = strings.TrimSpace(nickname)
	now = now.UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byMail[email]; ok {
		return ErrPendingConflict
	}
	m.byMail[email] = &PendingRegistration{
		ID:           1,
		Email:        email,
		PasswordHash: passwordHash,
		Nickname:     nickname,
		CodeHash:     codeHash,
		ExpiresAt:    now.Add(PendingCodeTTL),
		AttemptCount: 0,
		ResendCount:  0,
		LastSentAt:   now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	return nil
}

func (m *MemoryPendingRegistrations) ReplacePendingIfCodeHash(email, expectedCodeHash, passwordHash, nickname, newCodeHash string, now time.Time) error {
	email = auth.NormalizeEmail(email)
	nickname = strings.TrimSpace(nickname)
	now = now.UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.byMail[email]
	if !ok || p.CodeHash != expectedCodeHash {
		return ErrPendingConflict
	}
	p.PasswordHash = passwordHash
	p.Nickname = nickname
	p.CodeHash = newCodeHash
	p.ExpiresAt = now.Add(PendingCodeTTL)
	p.AttemptCount = 0
	p.ResendCount++
	p.LastSentAt = now
	p.UpdatedAt = now
	return nil
}

func (m *MemoryPendingRegistrations) RotatePendingCodeIfHash(email, expectedCodeHash, newCodeHash string, now time.Time) error {
	email = auth.NormalizeEmail(email)
	now = now.UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.byMail[email]
	if !ok || p.CodeHash != expectedCodeHash {
		return ErrPendingConflict
	}
	p.CodeHash = newCodeHash
	p.ExpiresAt = now.Add(PendingCodeTTL)
	p.AttemptCount = 0
	p.ResendCount++
	p.LastSentAt = now
	p.UpdatedAt = now
	return nil
}

func (m *MemoryPendingRegistrations) IncrementPendingVerifyAttempts(email string) (int, error) {
	email = auth.NormalizeEmail(email)
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.byMail[email]
	if !ok {
		return 0, ErrPendingNotFound
	}
	p.AttemptCount++
	p.UpdatedAt = time.Now().UTC()
	return p.AttemptCount, nil
}

func (m *MemoryPendingRegistrations) DeletePendingIfCodeHash(email, codeHash string) error {
	email = auth.NormalizeEmail(email)
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.byMail[email]
	if !ok || p.CodeHash != codeHash {
		return nil
	}
	delete(m.byMail, email)
	return nil
}

func (m *MemoryPendingRegistrations) UserEmailExists(email string) (bool, error) {
	email = auth.NormalizeEmail(email)
	if m.Users == nil {
		return false, nil
	}
	m.Users.mu.Lock()
	defer m.Users.mu.Unlock()
	_, ok := m.Users.byMail[email]
	return ok, nil
}

// TestingForceLastSentAt adjusts last_sent_at for cooldown tests.
func (m *MemoryPendingRegistrations) TestingForceLastSentAt(email string, at time.Time) {
	email = auth.NormalizeEmail(email)
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.byMail[email]; ok {
		p.LastSentAt = at
	}
}

// TestingForceExpiresAt adjusts expires_at for expiry tests.
func (m *MemoryPendingRegistrations) TestingForceExpiresAt(email string, at time.Time) {
	email = auth.NormalizeEmail(email)
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.byMail[email]; ok {
		p.ExpiresAt = at
	}
}

func (m *MemoryPendingRegistrations) ConsumePendingAndCreateUser(email, expectedCodeHash string) (*model.User, error) {
	email = auth.NormalizeEmail(email)
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.byMail[email]
	if !ok {
		return nil, ErrPendingNotFound
	}
	if time.Now().UTC().After(p.ExpiresAt.UTC()) {
		if p.CodeHash == expectedCodeHash {
			delete(m.byMail, email)
		}
		return nil, ErrPendingExpired
	}
	if p.AttemptCount >= PendingMaxVerifyAttempts {
		return nil, ErrPendingTooManyTries
	}
	if p.CodeHash != expectedCodeHash {
		return nil, ErrInvalidCredentials
	}
	if m.Users == nil {
		return nil, errors.New("users store missing")
	}
	m.Users.mu.Lock()
	defer m.Users.mu.Unlock()
	u, err := m.Users.CreateUserWithHashLocked(email, p.PasswordHash, p.Nickname)
	if err != nil {
		if errors.Is(err, ErrUserExists) {
			delete(m.byMail, email)
		}
		return nil, err
	}
	delete(m.byMail, email)
	return u, nil
}
