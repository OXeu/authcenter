package store

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm/clause"
)

// ErrInvalidGrant indicates an expired, mismatched, revoked or reused credential.
var ErrInvalidGrant = errors.New("invalid grant")

func (s *Store) lockGrant(ctx context.Context, id, clientID string) (OAuthGrant, error) {
	var grant OAuthGrant
	err := s.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&grant, "id = ? AND client_id = ? AND revoked = ?", id, clientID, false).Error
	if err != nil {
		return OAuthGrant{}, queryErr("lock grant", err)
	}
	user, err := s.UserByID(ctx, grant.UserID)
	if err != nil {
		return OAuthGrant{}, err
	}
	if !user.Active() {
		return OAuthGrant{}, ErrInvalidGrant
	}
	return grant, nil
}

// RedeemAuthorizationCode atomically consumes a code and issues its tokens.
func (s *Store) RedeemAuthorizationCode(ctx context.Context, digest, clientID, redirectURI, challenge, resource string, token OAuthToken) (OAuthGrant, error) {
	var grant OAuthGrant
	replay := false
	err := s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockOAuthClient(ctx, clientID); err != nil {
			return err
		}
		if err := tx.validateOAuthRedirect(ctx, clientID, redirectURI); err != nil {
			return err
		}
		var code AuthorizationCode
		if err := tx.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&code, "digest = ? AND client_id = ?", digest, clientID).Error; err != nil {
			return queryErr("load code", err)
		}
		if code.RedirectURI != redirectURI || code.Challenge != challenge {
			return ErrInvalidGrant
		}
		if code.Used {
			replay = true
			return tx.revokeGrant(ctx, code.GrantID)
		}
		if !code.ExpiresAt.After(time.Now().UTC()) {
			return ErrInvalidGrant
		}
		var err error
		grant, err = tx.lockGrant(ctx, code.GrantID, clientID)
		if err != nil {
			return err
		}
		if grant.Resource != resource {
			return ErrInvalidGrant
		}
		result := tx.db.WithContext(ctx).Model(&AuthorizationCode{}).Where("digest = ? AND used = ?", digest, false).Update("used", true)
		if err := tx.mustAffect(ctx, result, "consume code"); err != nil {
			return err
		}
		token.GrantID = grant.ID
		if !slices.Contains(strings.Fields(grant.Scope), "offline_access") {
			token.RefreshHash = nil
		}
		return queryErr("issue tokens", tx.db.WithContext(ctx).Create(&token).Error)
	})
	if err != nil {
		return OAuthGrant{}, err
	}
	if replay {
		return OAuthGrant{}, ErrInvalidGrant
	}
	return grant, nil
}

// RotateOAuthToken replaces a refresh token and revokes its family on reuse.
func (s *Store) RotateOAuthToken(ctx context.Context, digest, clientID, resource string, next OAuthToken) (OAuthGrant, error) {
	var grant OAuthGrant
	replay := false
	err := s.Transaction(ctx, func(tx *Store) error {
		if err := tx.lockOAuthClient(ctx, clientID); err != nil {
			return err
		}
		var previous OAuthToken
		if err := tx.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&previous, "refresh_hash = ?", digest).Error; err != nil {
			return queryErr("load refresh token", err)
		}
		var err error
		grant, err = tx.lockGrant(ctx, previous.GrantID, clientID)
		if err != nil {
			return err
		}
		if grant.Resource != resource {
			return ErrInvalidGrant
		}
		if previous.Used {
			replay = true
			return tx.revokeGrant(ctx, grant.ID)
		}
		if !previous.RefreshExpiresAt.After(time.Now().UTC()) {
			return ErrInvalidGrant
		}
		result := tx.db.WithContext(ctx).Model(&OAuthToken{}).Where("access_hash = ? AND used = ?", previous.AccessHash, false).Update("used", true)
		if err := tx.mustAffect(ctx, result, "consume refresh token"); err != nil {
			return err
		}
		next.GrantID = grant.ID
		next.RefreshExpiresAt = previous.RefreshExpiresAt
		return queryErr("rotate token", tx.db.WithContext(ctx).Create(&next).Error)
	})
	if err != nil {
		return OAuthGrant{}, err
	}
	if replay {
		return OAuthGrant{}, ErrInvalidGrant
	}
	return grant, nil
}

func (s *Store) revokeGrant(ctx context.Context, id string) error {
	return queryErr("revoke token family", s.db.WithContext(ctx).Model(&OAuthGrant{}).Where("id = ?", id).Update("revoked", true).Error)
}

// AccessTokenGrant authenticates an unexpired token against live grant and client state.
func (s *Store) AccessTokenGrant(ctx context.Context, digest string) (OAuthGrant, error) {
	var grant OAuthGrant
	err := s.db.WithContext(ctx).Model(&OAuthGrant{}).Select("oauth_grants.*").Joins("JOIN oauth_tokens ON oauth_tokens.grant_id = oauth_grants.id").Joins("JOIN oauth_clients ON oauth_clients.id = oauth_grants.client_id").Where("oauth_tokens.access_hash = ? AND oauth_tokens.expires_at > ? AND oauth_grants.revoked = ? AND oauth_clients.disabled = ?", digest, time.Now().UTC(), false, false).First(&grant).Error
	if err != nil {
		return OAuthGrant{}, queryErr("authenticate access token", err)
	}
	user, err := s.UserByID(ctx, grant.UserID)
	if err != nil {
		return OAuthGrant{}, err
	}
	if !user.Active() {
		return OAuthGrant{}, ErrInvalidGrant
	}
	return grant, nil
}

// RevokeOAuthToken revokes a token family only when it belongs to the requesting client.
func (s *Store) RevokeOAuthToken(ctx context.Context, digest, clientID string) error {
	var token OAuthToken
	err := s.db.WithContext(ctx).First(&token, "access_hash = ? OR refresh_hash = ?", digest, digest).Error
	if errors.Is(queryErr("token", err), ErrNotFound) {
		return nil
	}
	if err != nil {
		return queryErr("find token", err)
	}
	return queryErr("revoke token", s.db.WithContext(ctx).Model(&OAuthGrant{}).Where("id = ? AND client_id = ?", token.GrantID, clientID).Update("revoked", true).Error)
}
