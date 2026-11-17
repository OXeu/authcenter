package store

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Permission is an independently grantable management operation.
type Permission string

const (
	AccountRead   Permission = "account:read"
	AccountWrite  Permission = "account:write"
	ClientsRead   Permission = "clients:read"
	ClientsWrite  Permission = "clients:write"
	ClientsDelete Permission = "clients:delete"
	GrantsRead    Permission = "grants:read"
	GrantsDelete  Permission = "grants:delete"
	KeysRead      Permission = "keys:read"
	KeysWrite     Permission = "keys:write"
	UsersRead     Permission = "users:read"
	UsersWrite    Permission = "users:write"
	UsersDelete   Permission = "users:delete"
	SettingsRead  Permission = "settings:read"
	SettingsWrite Permission = "settings:write"
)

// KeyPermissions returns all supported permissions in display order.
func KeyPermissions() []Permission {
	return []Permission{AccountRead, AccountWrite, ClientsRead, ClientsWrite, ClientsDelete, GrantsRead, GrantsDelete, KeysRead, KeysWrite, UsersRead, UsersWrite, UsersDelete, SettingsRead, SettingsWrite}
}

// AdminOnly reports whether the operation requires a live administrator role.
func (p Permission) AdminOnly() bool {
	return strings.HasPrefix(string(p), "users:") || strings.HasPrefix(string(p), "settings:")
}

// APIKey stores an explicitly scoped credential digest and its owner.
type APIKey struct {
	ID         uint         `gorm:"primaryKey" json:"id"`
	Name       string       `gorm:"not null" json:"name"`
	Digest     []byte       `gorm:"uniqueIndex;size:32;not null" json:"-"`
	Prefix     string       `gorm:"not null" json:"prefix"`
	Scopes     []Permission `gorm:"serializer:json;type:text;not null" json:"scopes"`
	CreatedBy  uint         `gorm:"index;not null" json:"created_by"`
	Revision   uint64       `gorm:"not null;default:0" json:"-"`
	CreatedAt  time.Time    `json:"created_at"`
	LastUsedAt *time.Time   `json:"last_used_at"`
	ExpiresAt  *time.Time   `json:"expires_at"`
}

// Covers reports whether all requested permissions are granted to the key.
func (k APIKey) Covers(scopes []Permission) bool {
	for _, scope := range scopes {
		if !slices.Contains(k.Scopes, scope) {
			return false
		}
	}
	return true
}

// ErrKeyChanged requires authorization to be rechecked after a concurrent edit.
var ErrKeyChanged = errors.New("api key changed; reload and retry")

func keyMutation(result *gorm.DB) error {
	if result.Error != nil {
		return queryErr("change api key", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrKeyChanged
	}
	return nil
}

// CreateAPIKey saves only the digest of a newly generated credential.
func (s *Store) CreateAPIKey(ctx context.Context, key APIKey) (APIKey, error) {
	err := s.db.WithContext(ctx).Create(&key).Error
	return key, queryErr("create api key", err)
}

// ListAPIKeys lists metadata without credential digests.
func (s *Store) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	keys := []APIKey{}
	err := s.db.WithContext(ctx).Omit("digest").Order("id DESC").Find(&keys).Error
	return keys, queryErr("list api keys", err)
}

// APIKeyByID loads a key for authorized management.
func (s *Store) APIKeyByID(ctx context.Context, id uint) (APIKey, error) {
	var key APIKey
	err := s.db.WithContext(ctx).Omit("digest").First(&key, id).Error
	return key, queryErr("load api key", err)
}

// APIKeyByDigest resolves an API credential.
func (s *Store) APIKeyByDigest(ctx context.Context, digest [32]byte) (APIKey, error) {
	var key APIKey
	err := s.db.WithContext(ctx).First(&key, "digest = ?", digest[:]).Error
	return key, queryErr("authenticate api key", err)
}

// TouchAPIKey records the most recent successful authentication.
func (s *Store) TouchAPIKey(ctx context.Context, id uint) error {
	return queryErr("touch api key", s.db.WithContext(ctx).Model(&APIKey{}).Where("id = ?", id).Update("last_used_at", time.Now().UTC()).Error)
}

// UpdateAPIKey changes metadata after checking the authorized revision.
func (s *Store) UpdateAPIKey(ctx context.Context, key APIKey) error {
	revision := key.Revision
	key.Revision++
	return keyMutation(s.db.WithContext(ctx).Model(&APIKey{}).Where("id = ? AND revision = ?", key.ID, revision).Select("name", "scopes", "revision").Updates(&key))
}

// DeleteAPIKey revokes a credential after checking the authorized revision.
func (s *Store) DeleteAPIKey(ctx context.Context, key APIKey) error {
	return keyMutation(s.db.WithContext(ctx).Where("id = ? AND revision = ?", key.ID, key.Revision).Delete(&APIKey{}))
}

// RotateAPIKey immediately replaces a credential after checking its revision.
func (s *Store) RotateAPIKey(ctx context.Context, key APIKey, digest [32]byte, prefix string) error {
	return keyMutation(s.db.WithContext(ctx).Model(&APIKey{}).Where("id = ? AND revision = ?", key.ID, key.Revision).Updates(map[string]any{"digest": digest[:], "prefix": prefix, "last_used_at": nil, "revision": key.Revision + 1}))
}
