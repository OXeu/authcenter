package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ErrNotFound is returned when a scoped lookup finds no row.
var ErrNotFound = errors.New("not found")

// Store owns the database handle and every query.
type Store struct {
	db                  *gorm.DB
	registrationAllowed bool
}

// Transaction runs fn inside a database transaction; fn receives a Store
// bound to the transaction.
func (s *Store) Transaction(ctx context.Context, fn func(tx *Store) error) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Store{db: tx, registrationAllowed: s.registrationAllowed})
	})
	if err != nil {
		return fmt.Errorf("transaction: %w", err)
	}
	return nil
}

// Open migrates the schema and ensures the bootstrap administrator exists.
func Open(ctx context.Context, dialector gorm.Dialector, adminEmail, adminName string) (*Store, error) {
	db, err := gorm.Open(dialector, &gorm.Config{TranslateError: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if dialector.Name() == "sqlite" {
		pool, err := db.DB()
		if err != nil {
			return nil, fmt.Errorf("open sqlite pool: %w", err)
		}
		pool.SetMaxOpenConns(1)
	}
	if err := db.WithContext(ctx).AutoMigrate(
		&User{}, &Session{}, &APIKey{}, &LoginChallenge{}, &LoginLimit{}, &OAuthClient{}, &AuthorizationRequest{}, &OAuthGrant{}, &AuthorizationCode{}, &OAuthToken{},
	); err != nil {
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	s := &Store{db: db, registrationAllowed: true}
	if err := s.ensureAdmin(ctx, adminEmail, adminName); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) ensureAdmin(ctx context.Context, email, name string) error {
	var count int64
	if err := s.db.WithContext(ctx).Model(&User{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return nil
	}
	admin := User{Email: email, Name: name, Role: RoleAdmin}
	if err := s.db.WithContext(ctx).Create(&admin).Error; err != nil {
		return fmt.Errorf("create bootstrap admin: %w", err)
	}
	return nil
}

func queryErr(op string, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%s: %w", op, ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// UserByID loads a user.
func (s *Store) UserByID(ctx context.Context, id uint) (User, error) {
	var u User
	err := s.db.WithContext(ctx).First(&u, id).Error
	return u, queryErr("load user", err)
}

// UserByEmail loads a user by email.
func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := s.db.WithContext(ctx).Where("email = ?", email).First(&u).Error
	return u, queryErr("load user by email", err)
}

// UserBySubject loads a user linked to an OAuth issuer/subject pair.
func (s *Store) UserBySubject(ctx context.Context, issuer, sub string) (User, error) {
	var u User
	err := s.db.WithContext(ctx).Where("oauth_issuer = ? AND oauth_sub = ?", issuer, sub).First(&u).Error
	return u, queryErr("load user by subject", err)
}

// LinkOAuthUser links an OAuth identity to a user, creating it on first login.
// Identity.Role is applied only when the user is new; existing roles are
// managed in the user admin screen.
func (s *Store) LinkOAuthUser(ctx context.Context, identity Identity) (User, error) {
	u, err := s.UserBySubject(ctx, identity.Issuer, identity.Sub)
	switch {
	case err == nil:
	case errors.Is(err, ErrNotFound):
		u, err = s.adoptByEmail(ctx, identity)
		if err != nil {
			return User{}, err
		}
	default:
		return User{}, err
	}
	updates := map[string]any{
		"oauth_issuer":   identity.Issuer,
		"oauth_sub":      identity.Sub,
		"name":           identity.Name,
		"avatar_url":     identity.AvatarURL,
		"email_verified": u.EmailVerified || (identity.Email == u.Email && identity.EmailVerified),
		"last_login_at":  identity.Now,
	}
	if !u.Active() {
		return User{}, ErrInvalidGrant
	}
	if err := s.db.WithContext(ctx).Model(&u).Updates(updates).Error; err != nil {
		return User{}, fmt.Errorf("update oauth user: %w", err)
	}
	return s.UserByID(ctx, u.ID)
}

func (s *Store) adoptByEmail(ctx context.Context, identity Identity) (User, error) {
	u, err := s.UserByEmail(ctx, identity.Email)
	switch {
	case err == nil:
		if u.OAuthSub != "" || u.PasswordHash != "" || !identity.EmailVerified {
			return User{}, ErrIdentityConflict
		}
		return u, nil
	case errors.Is(err, ErrNotFound):
		allowed, policyErr := s.RegistrationAllowed(ctx)
		if policyErr != nil {
			return User{}, policyErr
		}
		if !allowed {
			return User{}, ErrRegistrationClosed
		}
		u = User{Email: identity.Email, Name: identity.Name, Role: identity.Role}
		if u.Role != RoleAdmin {
			u.Role = RoleUser
		}
		if err := s.db.WithContext(ctx).Create(&u).Error; err != nil {
			return User{}, fmt.Errorf("create oauth user: %w", err)
		}
		return u, nil
	default:
		return User{}, err
	}
}

// CreateUser inserts a user without an OAuth identity, for local accounts.
func (s *Store) CreateUser(ctx context.Context, u User) (User, error) {
	if err := s.db.WithContext(ctx).Create(&u).Error; err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return s.UserByID(ctx, u.ID)
}

// ListUsers returns all users ordered by id.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	var users []User
	if err := s.db.WithContext(ctx).Order("id").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return users, nil
}

// SetUserRole changes a user's role.
func (s *Store) SetUserRole(ctx context.Context, id uint, role Role) error {
	return s.mustAffect(ctx, s.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Update("role", role), "set user role")
}

// CreateSession persists a browser session.
func (s *Store) CreateSession(ctx context.Context, sess Session) error {
	return s.Transaction(ctx, func(tx *Store) error {
		user, err := tx.lockUser(ctx, sess.UserID)
		if err != nil {
			return err
		}
		if !user.Active() {
			return ErrInvalidGrant
		}
		return queryErr("create session", tx.db.WithContext(ctx).Create(&sess).Error)
	})
}

// SessionUser joins a session with its user and deletes expired sessions.
func (s *Store) SessionUser(ctx context.Context, id string) (Session, User, error) {
	var sess Session
	if err := s.db.WithContext(ctx).Where("id = ? AND expires_at > ?", id, time.Now().UTC()).First(&sess).Error; err != nil {
		return Session{}, User{}, queryErr("load session", err)
	}
	u, err := s.UserByID(ctx, sess.UserID)
	if err != nil {
		return Session{}, User{}, err
	}
	if !u.Active() {
		return Session{}, User{}, ErrNotFound
	}
	return sess, u, nil
}

// DeleteSession removes a session.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	if err := s.db.WithContext(ctx).Delete(&Session{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions drops sessions past their expiry.
func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	if err := s.db.WithContext(ctx).Where("expires_at < ?", time.Now().UTC()).Delete(&Session{}).Error; err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}

func (s *Store) mustAffect(_ context.Context, res *gorm.DB, op string) error {
	if err := res.Error; err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", op, ErrNotFound)
	}
	return nil
}
