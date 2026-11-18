package store

import (
	"context"
	"time"
)

// TOTPDevice is an independently enrolled authenticator and its replay state.
type TOTPDevice struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"-"`
	Name      string    `gorm:"size:100;not null" json:"name"`
	Secret    string    `gorm:"not null" json:"-"`
	Step      int64     `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

// TOTPDevices lists the authenticators belonging to an account.
func (s *Store) TOTPDevices(ctx context.Context, userID uint) ([]TOTPDevice, error) {
	devices := []TOTPDevice{}
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("id").Find(&devices).Error
	return devices, queryErr("list authenticators", err)
}

// RenameTOTPDevice changes the display name of an authenticator owned by an active account.
func (s *Store) RenameTOTPDevice(ctx context.Context, userID, deviceID uint, name string) error {
	return s.Transaction(ctx, func(tx *Store) error {
		user, err := tx.lockUser(ctx, userID)
		if err != nil {
			return err
		}
		if !user.Active() {
			return ErrInvalidGrant
		}
		var device TOTPDevice
		if err := tx.db.WithContext(ctx).Where("id = ? AND user_id = ?", deviceID, userID).First(&device).Error; err != nil {
			return queryErr("load authenticator", err)
		}
		return queryErr("rename authenticator", tx.db.WithContext(ctx).Model(&device).Update("name", name).Error)
	})
}

func (s *Store) migrateTOTPDevices(ctx context.Context) error {
	return s.Transaction(ctx, func(tx *Store) error {
		var users []User
		if err := tx.db.WithContext(ctx).Where("totp_secret <> ?", "").Find(&users).Error; err != nil {
			return queryErr("find legacy authenticators", err)
		}
		for _, user := range users {
			if err := tx.migrateUserTOTP(ctx, user); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) migrateUserTOTP(ctx context.Context, expected User) error {
	user, err := s.lockUser(ctx, expected.ID)
	if err != nil || user.TOTPSecret == "" {
		return err
	}
	device := TOTPDevice{UserID: user.ID, Name: "Authenticator", Secret: user.TOTPSecret, Step: user.TOTPStep}
	if err := s.db.WithContext(ctx).Create(&device).Error; err != nil {
		return queryErr("migrate authenticator", err)
	}
	return queryErr("clear legacy authenticator", s.db.WithContext(ctx).Model(&user).Updates(map[string]any{"totp_secret": "", "totp_step": 0}).Error)
}

// DisableTOTP removes one authenticator after checking its fresh code and account state.
func (s *Store) DisableTOTP(ctx context.Context, expected User, deviceID uint, step int64, session string, fallback bool) error {
	return s.Transaction(ctx, func(tx *Store) error {
		user, err := tx.lockUser(ctx, expected.ID)
		if err != nil {
			return err
		}
		if !user.Active() || user.PasswordHash != expected.PasswordHash {
			return ErrInvalidGrant
		}
		devices, err := tx.TOTPDevices(ctx, user.ID)
		if err != nil {
			return err
		}
		if len(devices) <= 1 && user.OAuthSub == "" && !fallback {
			return ErrInvalidGrant
		}
		result := tx.db.WithContext(ctx).Where("id = ? AND user_id = ? AND step < ?", deviceID, user.ID, step).Delete(&TOTPDevice{})
		if err := tx.mustAffect(ctx, result, "remove authenticator"); err != nil {
			return err
		}
		return tx.invalidateUserCredentials(ctx, user.ID, session)
	})
}
