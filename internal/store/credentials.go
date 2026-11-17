package store

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LoginChallenge is a browser-bound pending password and second-factor check.
type LoginChallenge struct {
	ID            string `gorm:"primaryKey;size:64"`
	UserID        uint   `gorm:"index"`
	Email         string `gorm:"size:254;not null"`
	Name          string
	PasswordHash  string
	SessionHash   string `gorm:"size:64"`
	PreviousEmail string
	EmailHash     string `gorm:"size:64"`
	Purpose       string `gorm:"size:32;not null"`
	Secret        string
	Attempts      int
	Used          bool
	ExpiresAt     time.Time `gorm:"index;not null"`
}

// LoginLimit counts attempts within a fixed time window.
type LoginLimit struct {
	ID        string `gorm:"primaryKey;size:128"`
	Count     int
	ExpiresAt time.Time `gorm:"index;not null"`
}

// AllowAttempt applies a database-backed attempt limit across processes.
func (s *Store) AllowAttempt(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	now := time.Now().UTC()
	record := LoginLimit{ID: key, Count: 1, ExpiresAt: now.Add(window)}
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.Assignments(map[string]any{"count": gorm.Expr("count + 1")})}).Create(&record).Error
	if err != nil {
		return false, queryErr("count login attempts", err)
	}
	if err := s.db.WithContext(ctx).First(&record, "id = ?", key).Error; err != nil {
		return false, queryErr("read login attempts", err)
	}
	return record.Count <= limit, nil
}

// CreateLoginChallenge saves a pending login without creating a session.
func (s *Store) CreateLoginChallenge(ctx context.Context, c LoginChallenge) error {
	return queryErr("create login challenge", s.db.WithContext(ctx).Create(&c).Error)
}

// LoginChallengeByID returns a live challenge.
func (s *Store) LoginChallengeByID(ctx context.Context, id string) (LoginChallenge, error) {
	var challenge LoginChallenge
	err := s.db.WithContext(ctx).First(&challenge, "id = ? AND used = ? AND attempts < ? AND expires_at > ?", id, false, 5, time.Now().UTC()).Error
	return challenge, queryErr("load login challenge", err)
}

// SetChallengeEmail replaces the email code after a rate-limited delivery request.
func (s *Store) SetChallengeEmail(ctx context.Context, id, hash string) error {
	return s.mustAffect(ctx, s.db.WithContext(ctx).Model(&LoginChallenge{}).Where("id = ? AND used = ? AND attempts < ? AND expires_at > ?", id, false, 5, time.Now().UTC()).Update("email_hash", hash), "set email code")
}

// VerifyLoginChallenge atomically checks attempts, replay state and account changes.
func (s *Store) VerifyLoginChallenge(ctx context.Context, id, emailHash string, step int64, totpValid bool) (User, error) {
	var user User
	rejected := false
	err := s.Transaction(ctx, func(tx *Store) error {
		var challenge LoginChallenge
		if err := tx.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&challenge, "id = ? AND used = ? AND attempts < ? AND expires_at > ?", id, false, 5, time.Now().UTC()).Error; err != nil {
			return queryErr("lock login challenge", err)
		}
		emailValid := emailHash != "" && challenge.EmailHash != "" && subtle.ConstantTimeCompare([]byte(emailHash), []byte(challenge.EmailHash)) == 1
		if !emailValid && !totpValid {
			rejected = true
			return queryErr("record failed verification", tx.db.WithContext(ctx).Model(&LoginChallenge{}).Where("id = ?", id).Update("attempts", gorm.Expr("attempts + 1")).Error)
		}
		if challenge.UserID != 0 {
			var err error
			user, err = tx.UserByID(ctx, challenge.UserID)
			if err != nil {
				return err
			}
		}
		switch challenge.Purpose {
		case "register":
			if !emailValid {
				return ErrInvalidGrant
			}
			if user.ID == 0 {
				user = User{Email: challenge.Email, Name: challenge.Name, Role: RoleUser}
				if err := tx.db.WithContext(ctx).Create(&user).Error; err != nil {
					return queryErr("register account", err)
				}
			}
			result := tx.db.WithContext(ctx).Model(&User{}).Where("id = ? AND password_hash = ?", user.ID, "").Updates(map[string]any{"password_hash": challenge.PasswordHash, "email_verified": true})
			if err := tx.mustAffect(ctx, result, "set initial password"); err != nil {
				return err
			}
		case "enroll":
			if !totpValid {
				return ErrInvalidGrant
			}
			if err := tx.mustAffect(ctx, tx.db.WithContext(ctx).Model(&User{}).Where("id = ? AND totp_secret = ?", user.ID, "").Updates(map[string]any{"totp_secret": challenge.Secret, "totp_step": step}), "enable authenticator"); err != nil {
				return err
			}
		case "login":
			if user.PasswordHash != challenge.PasswordHash {
				return ErrInvalidGrant
			}
			if totpValid {
				result := tx.db.WithContext(ctx).Model(&User{}).Where("id = ? AND totp_step < ? AND totp_secret = ?", user.ID, step, challenge.Secret).Update("totp_step", step)
				if err := tx.mustAffect(ctx, result, "consume TOTP step"); err != nil {
					return err
				}
			}
		default:
			return ErrInvalidGrant
		}
		if err := tx.mustAffect(ctx, tx.db.WithContext(ctx).Model(&LoginChallenge{}).Where("id = ? AND used = ?", id, false).Update("used", true), "consume login challenge"); err != nil {
			return err
		}
		return queryErr("record local login", tx.db.WithContext(ctx).Model(&User{}).Where("id = ?", user.ID).Update("last_login_at", time.Now().UTC()).Error)
	})
	if err != nil {
		return User{}, err
	}
	if rejected {
		return User{}, ErrInvalidGrant
	}
	return user, nil
}

// DeleteExpiredCredentials bounds persisted rate limits and pending requests.
func (s *Store) DeleteExpiredCredentials(ctx context.Context) error {
	for _, model := range []any{&LoginLimit{}, &LoginChallenge{}, &AuthorizationRequest{}} {
		if err := s.db.WithContext(ctx).Where("expires_at < ?", time.Now().UTC()).Delete(model).Error; err != nil {
			return queryErr("expire credentials", err)
		}
	}
	return nil
}

// ErrIdentityConflict prevents implicit linking of independent identities by email.
var ErrIdentityConflict = errors.New("email already belongs to another identity")
