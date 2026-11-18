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
	if err != nil {
		return User{}, queryErr("lock user", err)
	}
	return user, nil
}

func (s *Store) invalidateUserCredentials(ctx context.Context, id uint, session string) error {
	if err := s.db.WithContext(ctx).Where("created_by = ?", id).Delete(&APIKey{}).Error; err != nil {
		return queryErr("invalidate api keys", err)
	}
	if err := s.db.WithContext(ctx).Where("user_id = ? AND id <> ?", id, session).Delete(&Session{}).Error; err != nil {
		return queryErr("invalidate sessions", err)
	}
	if err := s.db.WithContext(ctx).Where("user_id = ?", id).Delete(&LoginChallenge{}).Error; err != nil {
		return queryErr("invalidate challenges", err)
	}
	var grants []OAuthGrant
	if err := s.db.WithContext(ctx).Where("user_id = ? AND revoked = ?", id, false).Order("client_id, id").Find(&grants).Error; err != nil {
		return queryErr("find user grants", err)
	}
	for _, grant := range grants {
		if err := s.revokeGrant(ctx, grant.ID); err != nil {
			return err
		}
	}
	return nil
}

// SetPassword replaces a verified account's password and invalidates other credentials.
func (s *Store) SetPassword(ctx context.Context, expected User, hash, session string) error {
	return s.Transaction(ctx, func(tx *Store) error {
		user, err := tx.lockUser(ctx, expected.ID)
		if err != nil {
			return err
		}
		if !user.Active() || user.PasswordHash != expected.PasswordHash {
			return ErrInvalidGrant
		}
		if err := tx.db.WithContext(ctx).Model(&user).Update("password_hash", hash).Error; err != nil {
			return queryErr("set password", err)
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
		if !user.Active() || user.PasswordHash != challenge.PasswordHash || user.Email != challenge.PreviousEmail {
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
