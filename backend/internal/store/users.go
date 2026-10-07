package store

import (
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/tycdn/vplayer/internal/model"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserExists       = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound     = errors.New("user not found")
)

type UserRepo interface {
	CreateUser(email, password, nickname string) (*model.User, error)
	Authenticate(email, password string) (*model.User, error)
	GetUserByID(id uint64) (*model.User, error)
}

func (m *MySQL) CreateUser(email, password, nickname string) (*model.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	nickname = strings.TrimSpace(nickname)
	if email == "" || password == "" {
		return nil, errors.New("email and password required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	res, err := m.DB.Exec(
		`INSERT INTO users (email, password_hash, nickname) VALUES (?, ?, ?)`,
		email, string(hash), nickname,
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
		CreatedAt: time.Now(),
	}, nil
}

func (m *MySQL) Authenticate(email, password string) (*model.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var u model.User
	var hash string
	err := m.DB.QueryRow(
		`SELECT id, email, password_hash, COALESCE(nickname,''), created_at FROM users WHERE email=? LIMIT 1`,
		email,
	).Scan(&u.ID, &u.Email, &hash, &u.Nickname, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	return &u, nil
}

func (m *MySQL) GetUserByID(id uint64) (*model.User, error) {
	var u model.User
	err := m.DB.QueryRow(
		`SELECT id, email, COALESCE(nickname,''), created_at FROM users WHERE id=? LIMIT 1`,
		id,
	).Scan(&u.ID, &u.Email, &u.Nickname, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// MemoryUsers is used when MySQL is unavailable (local demo).
type MemoryUsers struct {
	mu    sync.Mutex
	seq   uint64
	byID  map[uint64]*memUser
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
	email = strings.ToLower(strings.TrimSpace(email))
	nickname = strings.TrimSpace(nickname)
	if email == "" || password == "" {
		return nil, errors.New("email and password required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byMail[email]; ok {
		return nil, ErrUserExists
	}
	m.seq++
	u := &memUser{
		user: model.User{
			ID:        m.seq,
			Email:     email,
			Nickname:  nickname,
			CreatedAt: time.Now(),
		},
		hash: string(hash),
	}
	m.byID[u.user.ID] = u
	m.byMail[email] = u
	cp := u.user
	return &cp, nil
}

func (m *MemoryUsers) Authenticate(email, password string) (*model.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
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
