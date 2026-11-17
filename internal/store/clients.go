package store

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"gorm.io/gorm/clause"
)

// CreateOAuthClient registers an application.
func (s *Store) CreateOAuthClient(ctx context.Context, client OAuthClient) error {
	if err := s.db.WithContext(ctx).Create(&client).Error; err != nil {
		return fmt.Errorf("create oauth client: %w", err)
	}
	return nil
}

// OAuthClientByID loads an enabled application.
func (s *Store) OAuthClientByID(ctx context.Context, id string) (OAuthClient, error) {
	var client OAuthClient
	err := s.db.WithContext(ctx).First(&client, "id = ? AND disabled = ?", id, false).Error
	return client, queryErr("load oauth client", err)
}

func (s *Store) lockOAuthClient(ctx context.Context, id string) error {
	var client OAuthClient
	err := s.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&client, "id = ? AND disabled = ?", id, false).Error
	return queryErr("lock oauth client", err)
}

// ListOAuthClients lists active applications without exposing secrets.
func (s *Store) ListOAuthClients(ctx context.Context) ([]OAuthClient, error) {
	clients := []OAuthClient{}
	err := s.db.WithContext(ctx).Where("disabled = ?", false).Order("created_at DESC").Find(&clients).Error
	return clients, queryErr("list oauth clients", err)
}

// DisableOAuthClient immediately invalidates an application's codes and tokens.
func (s *Store) DisableOAuthClient(ctx context.Context, id string) error {
	return s.mustAffect(ctx, s.db.WithContext(ctx).Model(&OAuthClient{}).Where("id = ? AND disabled = ?", id, false).Update("disabled", true), "disable client")
}

// ListOAuthGrants lists the user's authorizations.
func (s *Store) ListOAuthGrants(ctx context.Context, userID uint) ([]OAuthGrant, error) {
	grants := []OAuthGrant{}
	err := s.db.WithContext(ctx).Where("user_id = ? AND revoked = ?", userID, false).Order("created_at DESC").Find(&grants).Error
	return grants, queryErr("list grants", err)
}

// RevokeOAuthGrant revokes a user's own authorization and all its tokens.
func (s *Store) RevokeOAuthGrant(ctx context.Context, userID uint, id string) error {
	return s.mustAffect(ctx, s.db.WithContext(ctx).Model(&OAuthGrant{}).Where("id = ? AND user_id = ?", id, userID).Update("revoked", true), "revoke grant")
}

// UpdateOAuthClient updates editable application metadata without changing credentials.
func (s *Store) UpdateOAuthClient(ctx context.Context, client OAuthClient) error {
	redirects, err := json.Marshal(client.RedirectURIs)
	if err != nil {
		return fmt.Errorf("encode callbacks: %w", err)
	}
	return s.mustAffect(ctx, s.db.WithContext(ctx).Model(&OAuthClient{}).Where("id = ? AND disabled = ?", client.ID, false).Updates(map[string]any{"name": client.Name, "homepage": client.Homepage, "redirect_uris": string(redirects), "logo": client.Logo, "logo_type": client.LogoType}), "update client")
}

func (s *Store) validateOAuthRedirect(ctx context.Context, clientID, redirect string) error {
	client, err := s.OAuthClientByID(ctx, clientID)
	if err != nil {
		return err
	}
	if !slices.Contains(client.RedirectURIs, redirect) {
		return ErrInvalidGrant
	}
	return nil
}
