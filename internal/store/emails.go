package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ErrPrimaryEmail prevents removing the primary address or selecting an unverified address.
var ErrPrimaryEmail = errors.New("primary email must remain verified and bound")

// UserEmail is an address bound exclusively to one account.
type UserEmail struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"-"`
	Email     string    `gorm:"uniqueIndex;size:254;not null" json:"email"`
	Verified  bool      `json:"verified"`
	Primary   bool      `gorm:"-" json:"primary"`
	CreatedAt time.Time `json:"created_at"`
}

// AccountEmails returns an account's addresses with the current primary marked.
func (s *Store) AccountEmails(ctx context.Context, userID uint) ([]UserEmail, error) {
	user, err := s.UserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	emails := []UserEmail{}
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("id").Find(&emails).Error; err != nil {
		return nil, queryErr("list account emails", err)
	}
	for i := range emails {
		emails[i].Primary = emails[i].Email == user.Email
	}
	return emails, nil
}

func (s *Store) migrateAccountEmails(ctx context.Context) error {
	return s.Transaction(ctx, func(tx *Store) error {
		users, err := tx.ListUsers(ctx)
		if err != nil {
			return err
		}
		for _, user := range users {
			locked, err := tx.lockUser(ctx, user.ID)
			if err != nil {
				return err
			}
			var email UserEmail
			err = tx.db.WithContext(ctx).Where("email = ?", locked.Email).First(&email).Error
			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
				if err := tx.createAccountEmail(ctx, locked.ID, locked.Email, locked.EmailVerified); err != nil {
					return err
				}
			case err != nil:
				return queryErr("find primary email", err)
			case email.UserID != locked.ID:
				return ErrIdentityConflict
			}
		}
		return nil
	})
}

func (s *Store) createAccountEmail(ctx context.Context, userID uint, address string, verified bool) error {
	email := UserEmail{UserID: userID, Email: address, Verified: verified}
	return queryErr("bind email", s.db.WithContext(ctx).Create(&email).Error)
}

func (s *Store) verifyAccountEmail(ctx context.Context, userID uint, address string) error {
	if err := s.requireAccountEmail(ctx, userID, address); err != nil {
		return err
	}
	return queryErr("verify account email", s.db.WithContext(ctx).Model(&UserEmail{}).Where("user_id = ? AND email = ?", userID, address).Update("verified", true).Error)
}

// SetPrimaryEmail selects an existing verified address for OAuth claims and account display.
func (s *Store) SetPrimaryEmail(ctx context.Context, expected User, emailID uint, session string) error {
	return s.Transaction(ctx, func(tx *Store) error {
		user, err := tx.lockUser(ctx, expected.ID)
		if err != nil {
			return err
		}
		if !user.Active() || user.PasswordHash != expected.PasswordHash {
			return ErrInvalidGrant
		}
		var email UserEmail
		if err := tx.db.WithContext(ctx).Where("id = ? AND user_id = ?", emailID, user.ID).First(&email).Error; err != nil {
			return queryErr("load account email", err)
		}
		if !email.Verified {
			return ErrPrimaryEmail
		}
		if user.Email == email.Email {
			return nil
		}
		if err := tx.db.WithContext(ctx).Model(&user).Updates(map[string]any{"email": email.Email, "email_verified": true}).Error; err != nil {
			return queryErr("set primary email", err)
		}
		return tx.invalidateUserCredentials(ctx, user.ID, session)
	})
}

// RemoveAccountEmail unbinds a secondary address and invalidates other credentials.
func (s *Store) RemoveAccountEmail(ctx context.Context, expected User, emailID uint, session string) error {
	return s.Transaction(ctx, func(tx *Store) error {
		user, err := tx.lockUser(ctx, expected.ID)
		if err != nil {
			return err
		}
		if !user.Active() || user.PasswordHash != expected.PasswordHash {
			return ErrInvalidGrant
		}
		var email UserEmail
		if err := tx.db.WithContext(ctx).Where("id = ? AND user_id = ?", emailID, user.ID).First(&email).Error; err != nil {
			return queryErr("load account email", err)
		}
		if user.Email == email.Email {
			return ErrPrimaryEmail
		}
		if err := tx.mustAffect(ctx, tx.db.WithContext(ctx).Delete(&email), "remove account email"); err != nil {
			return err
		}
		return tx.invalidateUserCredentials(ctx, user.ID, session)
	})
}

func (s *Store) requireAccountEmail(ctx context.Context, userID uint, address string) error {
	var email UserEmail
	return queryErr("check login email", s.db.WithContext(ctx).Where("user_id = ? AND email = ?", userID, address).First(&email).Error)
}
