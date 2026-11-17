package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm/clause"
)

// OAuthRegistration stores dynamic client metadata independently of applications.
type OAuthRegistration struct {
	ID           string   `gorm:"primaryKey;size:64"`
	Name         string   `gorm:"not null"`
	RedirectURIs []string `gorm:"serializer:json;type:text;not null"`
	Scopes       []string `gorm:"serializer:json;type:text;not null"`
	Disabled     bool     `gorm:"not null"`
	CreatedAt    time.Time
}

// TableName fixes the registration table name across database dialects.
func (OAuthRegistration) TableName() string { return "oauth_registrations" }

func (r OAuthRegistration) client() OAuthClient {
	return OAuthClient{ID: r.ID, Name: r.Name, Public: true, Dynamic: true, RedirectURIs: r.RedirectURIs, Scopes: r.Scopes, Disabled: r.Disabled, CreatedAt: r.CreatedAt}
}

// CreateOAuthRegistration saves a dynamic registration without creating an application.
func (s *Store) CreateOAuthRegistration(ctx context.Context, registration OAuthRegistration) error {
	return queryErr("create oauth registration", s.db.WithContext(ctx).Create(&registration).Error)
}

// AuthorizationClientByID resolves an enabled application or dynamic registration.
func (s *Store) AuthorizationClientByID(ctx context.Context, id string) (OAuthClient, error) {
	client, err := s.OAuthClientByID(ctx, id)
	if !errors.Is(err, ErrNotFound) {
		return client, err
	}
	var registration OAuthRegistration
	err = s.db.WithContext(ctx).First(&registration, "id = ? AND disabled = ?", id, false).Error
	return registration.client(), queryErr("load oauth registration", err)
}

func (s *Store) lockAuthorizationClient(ctx context.Context, id string) error {
	var client OAuthClient
	err := s.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&client, "id = ? AND disabled = ? AND dynamic = ?", id, false, false).Error
	if !errors.Is(queryErr("lock oauth client", err), ErrNotFound) {
		return queryErr("lock oauth client", err)
	}
	var registration OAuthRegistration
	err = s.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&registration, "id = ? AND disabled = ?", id, false).Error
	return queryErr("lock oauth registration", err)
}

func (s *Store) lockOAuthRegistration(ctx context.Context, id string) error {
	var registration OAuthRegistration
	err := s.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).Find(&registration).Error
	return queryErr("lock registration for revocation", err)
}

func (s *Store) deleteUnboundOAuthRegistration(ctx context.Context, id string) error {
	active := s.db.WithContext(ctx).Model(&OAuthGrant{}).Select("1").Where("client_id = ? AND revoked = ?", id, false)
	err := s.db.WithContext(ctx).Where("id = ? AND NOT EXISTS (?)", id, active).Delete(&OAuthRegistration{}).Error
	return queryErr("delete unbound oauth registration", err)
}

func (s *Store) migrateOAuthRegistrations(ctx context.Context) error {
	return s.Transaction(ctx, func(tx *Store) error {
		var clients []OAuthClient
		if err := tx.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("dynamic = ?", true).Find(&clients).Error; err != nil {
			return queryErr("load legacy registrations", err)
		}
		for _, client := range clients {
			registration := OAuthRegistration{ID: client.ID, Name: client.Name, RedirectURIs: client.RedirectURIs, Scopes: client.Scopes, Disabled: client.Disabled, CreatedAt: client.CreatedAt}
			if err := tx.CreateOAuthRegistration(ctx, registration); err != nil {
				return err
			}
			if err := tx.db.WithContext(ctx).Delete(&OAuthClient{}, "id = ? AND dynamic = ?", client.ID, true).Error; err != nil {
				return queryErr("remove legacy registration", err)
			}
		}
		return nil
	})
}
