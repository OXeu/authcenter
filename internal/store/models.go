package store

import "time"

// Role is the permission level of a user.
type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// Valid reports whether r is a known role.
func (r Role) Valid() bool {
	switch r {
	case RoleUser, RoleAdmin:
		return true
	default:
		return false
	}
}

// CanWrite reports whether the role may modify projects.
func (r Role) CanWrite() bool {
	return r == RoleUser || r == RoleAdmin
}

// User is a human account, optionally linked to an OAuth subject.
type User struct {
	ID            uint   `gorm:"primaryKey"`
	PasswordHash  string `gorm:"not null;default:''"`
	EmailVerified bool
	TOTPSecret    string
	TOTPStep      int64
	Email         string `gorm:"uniqueIndex;size:254;not null"`
	Name          string `gorm:"not null"`
	Role          Role   `gorm:"not null"`
	OAuthIssuer   string `gorm:"column:oauth_issuer;index"`
	AvatarURL     string
	OAuthSub      string     `gorm:"column:oauth_sub;index"`
	CreatedAt     time.Time  `gorm:"not null"`
	LastLoginAt   *time.Time `gorm:""`
	BannedUntil   *time.Time
}

// Active reports whether the account can authenticate at the current time.
func (u User) Active() bool { return u.BannedUntil == nil || !u.BannedUntil.After(time.Now().UTC()) }

// Session is a browser login session.
type Session struct {
	ID        string `gorm:"primaryKey;size:64"`
	UserID    uint   `gorm:"index;not null"`
	CSRFToken string `gorm:"not null"`
	CreatedAt time.Time
	ExpiresAt time.Time `gorm:"index;not null"`
}

// Identity is the normalized OAuth profile of a logging-in user.
type Identity struct {
	EmailVerified bool
	Issuer        string
	Sub           string
	Email         string
	Name          string
	AvatarURL     string
	Role          Role
	Now           any
}
