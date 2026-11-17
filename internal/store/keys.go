package store

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"gorm.io/gorm"
)

// APIKeyByID loads a key for authorized administration.
func (s *Store) APIKeyByID(ctx context.Context, id uint) (APIKey, error) {
	var key APIKey
	err := s.db.WithContext(ctx).First(&key, id).Error
	return key, queryErr("load api key", err)
}

// UpdateAPIKey changes a key's name and scopes without replacing its credential.
func (s *Store) UpdateAPIKey(ctx context.Context, id uint, revision uint64, name string, scopes []KeyScope) error {
	key := APIKey{Name: name, Scopes: scopes, Revision: revision + 1}
	return keyMutation(s.db.WithContext(ctx).Model(&APIKey{}).Where("id = ? AND revision = ?", id, revision).
		Select("name", "scopes", "revision").Updates(&key), "update api key")
}

func (s *Store) removeProjectKeyScopes(ctx context.Context, projectID uint) error {
	keys, err := s.ListAPIKeys(ctx)
	if err != nil {
		return err
	}
	for _, key := range keys {
		scopes := slices.DeleteFunc(slices.Clone(key.Scopes), func(scope KeyScope) bool { return scope.ProjectID == projectID })
		if len(scopes) != len(key.Scopes) {
			if err := s.UpdateAPIKey(ctx, key.ID, key.Revision, key.Name, scopes); err != nil {
				return err
			}
		}
	}
	return nil
}

// ProjectByID resolves the stable project identifier stored in a scope.
func (s *Store) ProjectByID(ctx context.Context, id uint) (Project, error) {
	var project Project
	err := s.db.WithContext(ctx).First(&project, id).Error
	return project, queryErr("load project", err)
}

// ErrKeyChanged indicates that authorization must be rechecked after a concurrent key edit.
var ErrKeyChanged = errors.New("api key changed; reload and retry")

func keyMutation(result *gorm.DB, operation string) error {
	if result.Error != nil {
		return fmt.Errorf("%s: %w", operation, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%s: %w", operation, ErrKeyChanged)
	}
	return nil
}
