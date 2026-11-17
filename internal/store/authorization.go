package store

import "time"

// OAuthClient is an administrator-registered relying application.
type OAuthClient struct {
	ID           string    `gorm:"primaryKey;size:64" json:"id"`
	Name         string    `gorm:"not null" json:"name"`
	OwnerID      uint      `gorm:"index;not null" json:"owner_id"`
	Homepage     string    `gorm:"not null" json:"homepage"`
	Logo         []byte    `gorm:"type:mediumblob" json:"-"`
	LogoType     string    `json:"-"`
	SecretHash   string    `gorm:"size:64" json:"-"`
	Public       bool      `gorm:"not null" json:"public"`
	RedirectURIs []string  `gorm:"serializer:json;type:text;not null" json:"redirect_uris"`
	Scopes       []string  `gorm:"serializer:json;type:text;not null" json:"scopes"`
	Disabled     bool      `gorm:"not null" json:"disabled"`
	CreatedAt    time.Time `json:"created_at"`
}

// AuthorizationRequest holds validated parameters until the user decides.
type AuthorizationRequest struct {
	ID          string `gorm:"primaryKey;size:64"`
	ClientID    string `gorm:"size:64;index;not null"`
	RedirectURI string `gorm:"type:text;not null"`
	Scope       string `gorm:"not null"`
	Resource    string `gorm:"size:2048;not null;default:''"`
	State       string `gorm:"type:text"`
	Challenge   string `gorm:"size:64;not null"`
	CSRFHash    string `gorm:"size:64"`
	UserID      uint
	Used        bool      `gorm:"not null"`
	ExpiresAt   time.Time `gorm:"index;not null"`
}

// OAuthGrant groups all tokens issued for a consent decision.
type OAuthGrant struct {
	ID        string    `gorm:"primaryKey;size:64" json:"id"`
	ClientID  string    `gorm:"size:64;index;not null" json:"client_id"`
	UserID    uint      `gorm:"index;not null" json:"-"`
	Scope     string    `gorm:"not null" json:"scope"`
	Resource  string    `gorm:"size:2048;not null;default:''" json:"resource,omitempty"`
	Revoked   bool      `gorm:"not null" json:"revoked"`
	CreatedAt time.Time `json:"created_at"`
}

// AuthorizationCode stores only a digest of a short-lived, single-use code.
type AuthorizationCode struct {
	Digest      string    `gorm:"primaryKey;size:64"`
	GrantID     string    `gorm:"size:64;index;not null"`
	ClientID    string    `gorm:"size:64;not null"`
	RedirectURI string    `gorm:"type:text;not null"`
	Challenge   string    `gorm:"size:64;not null"`
	Used        bool      `gorm:"not null"`
	ExpiresAt   time.Time `gorm:"index;not null"`
}

// OAuthToken stores opaque token digests and rotation history.
type OAuthToken struct {
	AccessHash       string    `gorm:"primaryKey;size:64"`
	RefreshHash      *string   `gorm:"uniqueIndex;size:64"`
	GrantID          string    `gorm:"index;size:64;not null"`
	ExpiresAt        time.Time `gorm:"index;not null"`
	RefreshExpiresAt time.Time
	Used             bool `gorm:"not null"`
}

// TableName fixes the OAuth client table name across database dialects.
func (OAuthClient) TableName() string { return "oauth_clients" }

// TableName fixes the OAuth grant table name across database dialects.
func (OAuthGrant) TableName() string { return "oauth_grants" }

// TableName fixes the OAuth token table name across database dialects.
func (OAuthToken) TableName() string { return "oauth_tokens" }
