package store

import (
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/tycdn/vplayer/internal/auth"
	"github.com/tycdn/vplayer/internal/model"
	"golang.org/x/crypto/bcrypt"
)

func authNormalizeEmail(email string) string {
	return auth.NormalizeEmail(email)
}

var (
	ErrUserExists         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")
)

type UserRepo interface {
	CreateUser(email, password, nickname string) (*model.User, error)
	CreateUserWithHash(email, passwordHash, nickname string) (*model.User, error)
	Authenticate(email, password string) (*model.User, error)
	GetUserByID(id uint64) (*model.User, error)
	UpdateProfile(id uint64, nickname string, avatarHue int) (*model.User, error)
}

func (m *MySQL) CreateUser(email, password, nickname string) (*model.User, error) {
	email = authNormalizeEmail(email)
	nickname = strings.TrimSpace(nickname)
	if email == "" || password == "" {
		return nil, errors.New("email and password required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return m.CreateUserWithHash(email, string(hash), nickname)
}

func (m *MySQL) CreateUserWithHash(email, passwordHash, nickname string) (*model.User, error) {
	email = authNormalizeEmail(email)
	nickname = strings.TrimSpace(nickname)
	passwordHash = strings.TrimSpace(passwordHash)
	if email == "" || passwordHash == "" {
		return nil, errors.New("email and password hash required")
	}
	res, err := m.DB.Exec(
		`INSERT INTO users (email, password_hash, nickname) VALUES (?, ?, ?)`,
		email, passwordHash, nickname,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return nil, ErrUserExists
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &model.User{
		ID:        uint64(id),
		Email:     email,
		Nickname:  nickname,
		AvatarHue: -1,
		IsAdmin:   false,
		CreatedAt: time.Now(),
	}, nil
}

// UpdateProfile sets nickname and avatar hue. Callers validate ranges.
func (m *MySQL) UpdateProfile(id uint64, nickname string, avatarHue int) (*model.User, error) {
	if _, err := m.DB.Exec(`UPDATE users SET nickname=?, avatar_hue=? WHERE id=?`, nickname, avatarHue, id); err != nil {
		return nil, err
	}
	return m.GetUserByID(id)
}

func (m *MySQL) Authenticate(email, password string) (*model.User, error) {
	email = authNormalizeEmail(email)
	var u model.User
	var hash string
	var isAdmin int
	err := m.DB.QueryRow(
		`SELECT id, email, password_hash, COALESCE(nickname,''), COALESCE(avatar_hue,-1), COALESCE(is_admin,0), created_at FROM users WHERE email=? LIMIT 1`,
		email,
	).Scan(&u.ID, &u.Email, &hash, &u.Nickname, &u.AvatarHue, &isAdmin, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	u.IsAdmin = isAdmin == 1
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	return &u, nil
}

func (m *MySQL) GetUserByID(id uint64) (*model.User, error) {
	var u model.User
	var isAdmin int
	err := m.DB.QueryRow(
		`SELECT id, email, COALESCE(nickname,''), COALESCE(avatar_hue,-1), COALESCE(is_admin,0), created_at FROM users WHERE id=? LIMIT 1`,
		id,
	).Scan(&u.ID, &u.Email, &u.Nickname, &u.AvatarHue, &isAdmin, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	u.IsAdmin = isAdmin == 1
	return &u, nil
}

// MemoryUsers is used when MySQL is unavailable (local demo).
type MemoryUsers struct {
	mu     sync.Mutex
	seq    uint64
	byID   map[uint64]*memUser
	byMail map[string]*memUser
}

type memUser struct {
	user model.User
	hash string
}

func NewMemoryUsers() *MemoryUsers {
	return &MemoryUsers{
		byID:   map[uint64]*memUser{},
		byMail: map[string]*memUser{},
	}
}

func (m *MemoryUsers) CreateUser(email, password, nickname string) (*model.User, error) {
	email = authNormalizeEmail(email)
	nickname = strings.TrimSpace(nickname)
	if email == "" || password == "" {
		return nil, errors.New("email and password required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return m.CreateUserWithHash(email, string(hash), nickname)
}

func (m *MemoryUsers) CreateUserWithHash(email, passwordHash, nickname string) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.CreateUserWithHashLocked(email, passwordHash, nickname)
}

// CreateUserWithHashLocked requires m.mu held.
func (m *MemoryUsers) CreateUserWithHashLocked(email, passwordHash, nickname string) (*model.User, error) {
	email = authNormalizeEmail(email)
	nickname = strings.TrimSpace(nickname)
	passwordHash = strings.TrimSpace(passwordHash)
	if email == "" || passwordHash == "" {
		return nil, errors.New("email and password hash required")
	}
	if _, ok := m.byMail[email]; ok {
		return nil, ErrUserExists
	}
	m.seq++
	u := &memUser{
		user: model.User{
			ID:        m.seq,
			Email:     email,
			Nickname:  nickname,
			AvatarHue: -1,
			IsAdmin:   false,
			CreatedAt: time.Now(),
		},
		hash: passwordHash,
	}
	m.byID[u.user.ID] = u
	m.byMail[email] = u
	cp := u.user
	return &cp, nil
}

func (m *MemoryUsers) Authenticate(email, password string) (*model.User, error) {
	email = authNormalizeEmail(email)
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byMail[email]
	if !ok || bcrypt.CompareHashAndPassword([]byte(u.hash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	cp := u.user
	return &cp, nil
}

func (m *MemoryUsers) GetUserByID(id uint64) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byID[id]
	if !ok {
		return nil, ErrUserNotFound
	}
	cp := u.user
	return &cp, nil
}

func (m *MemoryUsers) UpdateProfile(id uint64, nickname string, avatarHue int) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byID[id]
	if !ok {
		return nil, ErrUserNotFound
	}
	u.user.Nickname = nickname
	u.user.AvatarHue = avatarHue
	cp := u.user
	return &cp, nil
}

// SetIsAdmin updates admin flag for tests / out-of-band promotion (not exposed via HTTP).
func (m *MemoryUsers) SetIsAdmin(id uint64, isAdmin bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byID[id]
	if !ok {
		return ErrUserNotFound
	}
	u.user.IsAdmin = isAdmin
	return nil
}
