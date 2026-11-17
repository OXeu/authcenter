package store

import (
	"context"
	"crypto/subtle"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Store) lockUser(ctx context.Context, id uint) (User, error) {
	var user User
	err := s.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, id).Error
	return user, queryErr("lock user", err)
}

func (s *Store) invalidateUserCredentials(ctx context.Context, id uint, session string) error {
	if err := s.db.WithContext(ctx).Where("user_id = ? AND id <> ?", id, session).Delete(&Session{}).Error; err != nil {
		return queryErr("invalidate sessions", err)
	}
	if err := s.db.WithContext(ctx).Where("user_id = ?", id).Delete(&LoginChallenge{}).Error; err != nil {
		return queryErr("invalidate challenges", err)
	}
	return queryErr("invalidate grants", s.db.WithContext(ctx).Model(&OAuthGrant{}).Where("user_id = ?", id).Update("revoked", true).Error)
}

// SetPassword replaces a verified account's password and invalidates other credentials.
func (s *Store) SetPassword(ctx context.Context, expected User, hash, session string) error {
	return s.Transaction(ctx, func(tx *Store) error {
		user, err := tx.lockUser(ctx, expected.ID)
		if err != nil {
			return err
		}
		if user.PasswordHash != expected.PasswordHash {
			return ErrInvalidGrant
		}
		if err := tx.db.WithContext(ctx).Model(&user).Update("password_hash", hash).Error; err != nil {
			return queryErr("set password", err)
		}
		return tx.invalidateUserCredentials(ctx, user.ID, session)
	})
}

// DisableTOTP consumes a fresh authenticator code and invalidates other credentials.
func (s *Store) DisableTOTP(ctx context.Context, expected User, step int64, session string) error {
	return s.Transaction(ctx, func(tx *Store) error {
		user, err := tx.lockUser(ctx, expected.ID)
		if err != nil {
			return err
		}
		if user.PasswordHash != expected.PasswordHash || user.TOTPSecret == "" || user.TOTPSecret != expected.TOTPSecret || user.TOTPStep >= step {
			return ErrInvalidGrant
		}
		if err := tx.db.WithContext(ctx).Model(&user).Updates(map[string]any{"totp_secret": "", "totp_step": 0}).Error; err != nil {
			return queryErr("disable authenticator", err)
		}
		return tx.invalidateUserCredentials(ctx, user.ID, session)
	})
}

// VerifyEmailChange consumes a session-bound code and changes the account email atomically.
func (s *Store) VerifyEmailChange(ctx context.Context, userID uint, id, sessionHash, codeHash, session string) error {
	rejected := false
	err := s.Transaction(ctx, func(tx *Store) error {
		var challenge LoginChallenge
		err := tx.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&challenge, "id = ? AND user_id = ? AND session_hash = ? AND purpose = ? AND used = ? AND attempts < ? AND expires_at > ?", id, userID, sessionHash, "email_change", false, 5, time.Now().UTC()).Error
		if err != nil {
			return queryErr("lock email change", err)
		}
		if challenge.EmailHash == "" || subtle.ConstantTimeCompare([]byte(challenge.EmailHash), []byte(codeHash)) != 1 {
			rejected = true
			return queryErr("count email verification", tx.db.WithContext(ctx).Model(&challenge).Update("attempts", gorm.Expr("attempts + 1")).Error)
		}
		user, err := tx.lockUser(ctx, userID)
		if err != nil {
			return err
		}
		if user.PasswordHash != challenge.PasswordHash || user.Email != challenge.PreviousEmail {
			return ErrInvalidGrant
		}
		if err := tx.db.WithContext(ctx).Model(&user).Updates(map[string]any{"email": challenge.Email, "email_verified": true}).Error; err != nil {
			return queryErr("change email", err)
		}
		return tx.invalidateUserCredentials(ctx, userID, session)
	})
	if err != nil {
		return err
	}
	if rejected {
		return ErrInvalidGrant
	}
	return nil
}
