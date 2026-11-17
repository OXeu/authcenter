package store

import (
	"context"
	"time"
)

// CreateAuthorizationRequest saves validated authorization parameters.
func (s *Store) CreateAuthorizationRequest(ctx context.Context, request AuthorizationRequest) error {
	return queryErr("create authorization request", s.db.WithContext(ctx).Create(&request).Error)
}

// AuthorizationRequestByID loads an unexpired, unused request.
func (s *Store) AuthorizationRequestByID(ctx context.Context, id string) (AuthorizationRequest, error) {
	var request AuthorizationRequest
	err := s.db.WithContext(ctx).First(&request, "id = ? AND used = ? AND expires_at > ?", id, false, time.Now().UTC()).Error
	return request, queryErr("load authorization request", err)
}

// BindAuthorizationRequest binds a pending request to the consenting session.
func (s *Store) BindAuthorizationRequest(ctx context.Context, id, csrfHash string, userID uint) error {
	err := s.db.WithContext(ctx).Model(&AuthorizationRequest{}).Where("id = ? AND csrf_hash = ? AND used = ?", id, "", false).Updates(map[string]any{"csrf_hash": csrfHash, "user_id": userID}).Error
	return queryErr("bind authorization request", err)
}

// DecideAuthorization consumes a consent request and optionally issues a code atomically.
func (s *Store) DecideAuthorization(ctx context.Context, request AuthorizationRequest, grant OAuthGrant, code AuthorizationCode, approve bool) error {
	return s.Transaction(ctx, func(tx *Store) error {
		user, err := tx.lockUser(ctx, request.UserID)
		if err != nil {
			return err
		}
		if !user.Active() {
			return ErrInvalidGrant
		}
		if err := tx.lockOAuthClient(ctx, request.ClientID); err != nil {
			return err
		}
		if err := tx.validateOAuthRedirect(ctx, request.ClientID, request.RedirectURI); err != nil {
			return err
		}
		result := tx.db.WithContext(ctx).Model(&AuthorizationRequest{}).Where("id = ? AND used = ? AND expires_at > ? AND user_id = ? AND csrf_hash = ?", request.ID, false, time.Now().UTC(), request.UserID, request.CSRFHash).Update("used", true)
		if err := tx.mustAffect(ctx, result, "consume authorization request"); err != nil {
			return err
		}
		if !approve {
			return nil
		}
		if err := tx.db.WithContext(ctx).Create(&grant).Error; err != nil {
			return queryErr("create grant", err)
		}
		return queryErr("create authorization code", tx.db.WithContext(ctx).Create(&code).Error)
	})
}
