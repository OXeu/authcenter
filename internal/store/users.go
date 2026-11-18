package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm/clause"
)

// ErrProtectedUser prevents self-management or management by an inactive administrator.
var ErrProtectedUser = errors.New("protected user")

func (s *Store) managementTarget(ctx context.Context, actor, target uint) (User, error) {
	var admins []User
	if err := s.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("role = ?", RoleAdmin).Order("id").Find(&admins).Error; err != nil {
		return User{}, queryErr("lock administrators", err)
	}
	allowed := false
	for _, admin := range admins {
		if admin.ID == actor && admin.Active() {
			allowed = true
		}
	}
	if !allowed || actor == target {
		return User{}, ErrProtectedUser
	}
	return s.lockUser(ctx, target)
}

// ManageUserRole checks the current administrator before changing another account's role.
func (s *Store) ManageUserRole(ctx context.Context, actor, target uint, role Role) error {
	return s.Transaction(ctx, func(tx *Store) error {
		if _, err := tx.managementTarget(ctx, actor, target); err != nil {
			return err
		}
		return tx.SetUserRole(ctx, target, role)
	})
}

// SetUserBan changes a ban expiry and invalidates credentials when imposing a ban.
func (s *Store) SetUserBan(ctx context.Context, actor, target uint, until *time.Time) error {
	return s.Transaction(ctx, func(tx *Store) error {
		user, err := tx.managementTarget(ctx, actor, target)
		if err != nil {
			return err
		}
		if err := tx.db.WithContext(ctx).Model(&user).Update("banned_until", until).Error; err != nil {
			return queryErr("set user ban", err)
		}
		if until != nil {
			return tx.invalidateUserCredentials(ctx, target, "")
		}
		return nil
	})
}

// DeleteUser removes an account, its applications and every dependent credential.
func (s *Store) DeleteUser(ctx context.Context, actor, target uint) error {
	return s.Transaction(ctx, func(tx *Store) error {
		if _, err := tx.managementTarget(ctx, actor, target); err != nil {
			return err
		}
		if err := tx.invalidateUserCredentials(ctx, target, ""); err != nil {
			return err
		}
		clients := tx.db.WithContext(ctx).Model(&OAuthClient{}).Select("id").Where("owner_id = ?", target)
		grants := tx.db.WithContext(ctx).Model(&OAuthGrant{}).Select("id").Where("user_id = ? OR client_id IN (?)", target, clients)
		for _, model := range []any{&OAuthToken{}, &AuthorizationCode{}} {
			if err := tx.db.WithContext(ctx).Where("grant_id IN (?)", grants).Delete(model).Error; err != nil {
				return queryErr("delete user tokens", err)
			}
		}
		if err := tx.db.WithContext(ctx).Where("user_id = ? OR client_id IN (?)", target, clients).Delete(&AuthorizationRequest{}).Error; err != nil {
			return queryErr("delete authorization requests", err)
		}
		if err := tx.db.WithContext(ctx).Where("user_id = ? OR client_id IN (?)", target, clients).Delete(&OAuthGrant{}).Error; err != nil {
			return queryErr("delete user grants", err)
		}
		if err := tx.db.WithContext(ctx).Where("owner_id = ?", target).Delete(&OAuthClient{}).Error; err != nil {
			return queryErr("delete user applications", err)
		}
		if err := tx.db.WithContext(ctx).Where("user_id = ?", target).Delete(&TOTPDevice{}).Error; err != nil {
			return queryErr("delete user authenticators", err)
		}
		return tx.mustAffect(ctx, tx.db.WithContext(ctx).Delete(&User{}, target), "delete user")
	})
}
