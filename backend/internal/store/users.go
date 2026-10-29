package store

import (
	"database/sql"
	"errors"
	"strconv"
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
	// SearchUsersForMessage finds users by numeric id, email, or nickname (exact, case-insensitive).
	SearchUsersForMessage(query string, limit int) ([]model.User, error)
	UpdateProfile(id uint64, nickname string, avatarHue int) (*model.User, error)
	UpdatePasswordHash(email, passwordHash string) error
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

// UpdatePasswordHash sets users.password_hash by normalized email.
func (m *MySQL) UpdatePasswordHash(email, passwordHash string) error {
	email = authNormalizeEmail(email)
	passwordHash = strings.TrimSpace(passwordHash)
	if email == "" || passwordHash == "" {
		return errors.New("email and password hash required")
	}
	res, err := m.DB.Exec(`UPDATE users SET password_hash=? WHERE email=?`, passwordHash, email)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (m *MySQL) Authenticate(email, password string) (*model.User, error) {
	email = authNormalizeEmail(email)
	var u model.User
	var hash string
	var isAdmin, isMember int
	var exp sql.NullTime
	err := m.DB.QueryRow(
		`SELECT id, email, password_hash, COALESCE(nickname,''), COALESCE(avatar_hue,-1), COALESCE(is_admin,0), COALESCE(is_member,0), membership_expires_at, created_at FROM users WHERE email=? LIMIT 1`,
		email,
	).Scan(&u.ID, &u.Email, &hash, &u.Nickname, &u.AvatarHue, &isAdmin, &isMember, &exp, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	u.IsAdmin = isAdmin == 1
	u.IsMember = isMember == 1
	if exp.Valid {
		t := exp.Time
		u.MembershipExpiresAt = &t
	}
	applyEffectiveMembership(&u, time.Now())
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	return &u, nil
}

func (m *MySQL) GetUserByID(id uint64) (*model.User, error) {
	var u model.User
	var isAdmin, isMember int
	var exp sql.NullTime
	err := m.DB.QueryRow(
		`SELECT id, email, COALESCE(nickname,''), COALESCE(avatar_hue,-1), COALESCE(is_admin,0), COALESCE(is_member,0), membership_expires_at, created_at FROM users WHERE id=? LIMIT 1`,
		id,
	).Scan(&u.ID, &u.Email, &u.Nickname, &u.AvatarHue, &isAdmin, &isMember, &exp, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	u.IsAdmin = isAdmin == 1
	u.IsMember = isMember == 1
	if exp.Valid {
		t := exp.Time
		u.MembershipExpiresAt = &t
	}
	applyEffectiveMembership(&u, time.Now())
	return &u, nil
}

func clampUserSearchLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	if limit > 20 {
		return 20
	}
	return limit
}

func scanPublicUserRow(scanner interface {
	Scan(dest ...any) error
}) (model.User, error) {
	var u model.User
	var isAdmin, isMember int
	var exp sql.NullTime
	err := scanner.Scan(&u.ID, &u.Email, &u.Nickname, &u.AvatarHue, &isAdmin, &isMember, &exp, &u.CreatedAt)
	if err != nil {
		return model.User{}, err
	}
	u.IsAdmin = isAdmin == 1
	u.IsMember = isMember == 1
	if exp.Valid {
		t := exp.Time
		u.MembershipExpiresAt = &t
	}
	applyEffectiveMembership(&u, time.Now())
	return u, nil
}

const userPublicSelect = `SELECT id, email, COALESCE(nickname,''), COALESCE(avatar_hue,-1), COALESCE(is_admin,0), COALESCE(is_member,0), membership_expires_at, created_at FROM users`

// SearchUsersForMessage matches numeric id, exact email, or exact nickname (case-insensitive).
func (m *MySQL) SearchUsersForMessage(query string, limit int) ([]model.User, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	limit = clampUserSearchLimit(limit)
	seen := map[uint64]struct{}{}
	out := make([]model.User, 0, limit)

	add := func(u model.User) {
		if _, ok := seen[u.ID]; ok {
			return
		}
		seen[u.ID] = struct{}{}
		out = append(out, u)
	}

	if id, err := strconv.ParseUint(q, 10, 64); err == nil && id > 0 {
		if u, err := m.GetUserByID(id); err == nil && u != nil {
			add(*u)
		} else if err != nil && !errors.Is(err, ErrUserNotFound) {
			return nil, err
		}
	}

	email := authNormalizeEmail(q)
	if email != "" {
		row := m.DB.QueryRow(userPublicSelect+` WHERE email=? LIMIT 1`, email)
		if u, err := scanPublicUserRow(row); err == nil {
			add(u)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	if len(out) >= limit {
		return out[:limit], nil
	}

	// Nickname exact match, or email local-part (before @) exact match — e.g. "test2" → test2@mail.com
	rows, err := m.DB.Query(
		userPublicSelect+`
		WHERE LOWER(TRIM(nickname)) = LOWER(?)
		   OR LOWER(SUBSTRING_INDEX(email, '@', 1)) = LOWER(?)
		ORDER BY id ASC LIMIT ?`,
		q, q, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		u, err := scanPublicUserRow(rows)
		if err != nil {
			return nil, err
		}
		add(u)
		if len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}

func applyEffectiveMembership(u *model.User, now time.Time) {
	if u == nil {
		return
	}
	active := EffectiveMember(u, now)
	u.IsMember = active
	if active && u.MembershipExpiresAt != nil {
		u.MembershipDaysRemaining = MembershipDaysRemaining(u.MembershipExpiresAt, now)
	} else {
		u.MembershipDaysRemaining = 0
	}
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

func (m *MemoryUsers) SearchUsersForMessage(query string, limit int) ([]model.User, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	limit = clampUserSearchLimit(limit)
	m.mu.Lock()
	defer m.mu.Unlock()

	seen := map[uint64]struct{}{}
	out := make([]model.User, 0, limit)
	add := func(u model.User) {
		if _, ok := seen[u.ID]; ok {
			return
		}
		seen[u.ID] = struct{}{}
		out = append(out, u)
	}

	if id, err := strconv.ParseUint(q, 10, 64); err == nil && id > 0 {
		if u, ok := m.byID[id]; ok {
			add(u.user)
		}
	}
	email := authNormalizeEmail(q)
	if email != "" {
		if u, ok := m.byMail[email]; ok {
			add(u.user)
		}
	}
	needle := strings.ToLower(q)
	for _, u := range m.byID {
		nick := strings.ToLower(strings.TrimSpace(u.user.Nickname))
		local, _, _ := strings.Cut(strings.ToLower(u.user.Email), "@")
		if nick == needle || local == needle {
			add(u.user)
		}
		if len(out) >= limit {
			break
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
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

func (m *MemoryUsers) EmailExists(email string) (bool, error) {
	email = authNormalizeEmail(email)
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.byMail[email]
	return ok, nil
}

func (m *MemoryUsers) UpdatePasswordHash(email, passwordHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.UpdatePasswordHashLocked(email, passwordHash)
}

// UpdatePasswordHashLocked requires m.mu held.
func (m *MemoryUsers) UpdatePasswordHashLocked(email, passwordHash string) error {
	email = authNormalizeEmail(email)
	passwordHash = strings.TrimSpace(passwordHash)
	if email == "" || passwordHash == "" {
		return errors.New("email and password hash required")
	}
	u, ok := m.byMail[email]
	if !ok {
		return ErrUserNotFound
	}
	u.hash = passwordHash
	return nil
}
