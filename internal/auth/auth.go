package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/OXeu/authcenter/internal/store"
)

// Authenticator resolves browser sessions and API keys.
type Authenticator struct {
	store Store
	ttl   time.Duration
	now   func() time.Time
}

const (
	sessionCookie = "ac_session"
	stateCookie   = "ac_oauth_state"
	csrfHeader    = "X-CSRF-Token"
)

// Store is the subset of persistence the authenticator needs.
type Store interface {
	APIKeyByDigest(ctx context.Context, digest [32]byte) (store.APIKey, error)
	TouchAPIKey(ctx context.Context, id uint) error
	UserByID(ctx context.Context, id uint) (store.User, error)
	SessionUser(ctx context.Context, id string) (store.Session, store.User, error)
	CreateSession(ctx context.Context, sess store.Session) error
	DeleteSession(ctx context.Context, id string) error
}

// Config is the upstream identity provider configuration.
type Config struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	Scopes       []string
	RedirectURL  string
	GroupsClaim  string
}

// Enabled reports whether OAuth login is configured.
func (c Config) Enabled() bool {
	return c.IssuerURL != "" && c.ClientID != "" && c.ClientSecret != ""
}

// Identity is the authenticated principal of a request.
type Identity struct {
	APIKey *store.APIKey
	User   *store.User
	Role   store.Role
}

// CanWrite reports whether the principal may change configuration.
func (i Identity) CanWrite() bool {
	return i.Role.CanWrite()
}

// New builds an Authenticator.
func New(s Store, ttl time.Duration) *Authenticator {
	return &Authenticator{
		store: s,
		ttl:   ttl,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

type identityKey struct{}

// FromContext returns the principal stored on the request context.
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	return id, ok
}

// WithIdentity returns a copy of ctx carrying id.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

// CSRF returns the token issued with the current session.
func CSRF(c *gin.Context) string {
	tok, ok := c.Get("ac_csrf")
	if !ok {
		return ""
	}
	str, ok := tok.(string)
	if !ok {
		return ""
	}
	return str
}

// RequireUser sends browsers to the login page unless a session exists, and
// answers 401 to API clients.
func RequireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := FromContext(c.Request.Context())
		if ok && id.User != nil {
			c.Next()
			return
		}
		if c.Request.Method == http.MethodGet && !strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "login required"})
	}
}

// RequireWriter aborts with 403 unless the principal may write.
func RequireWriter() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := FromContext(c.Request.Context())
		if !ok || !id.CanWrite() {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "write access required"})
			return
		}
		c.Next()
	}
}

// RequireAdmin aborts with 403 unless the principal is an administrator.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := FromContext(c.Request.Context())
		if !ok || id.Role != store.RoleAdmin || id.User == nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin required"})
			return
		}
		c.Next()
	}
}

// BrowserMiddleware resolves the session and enforces the CSRF token on the
// same routes, so every page and JSON call behind it behaves alike.
func (a *Authenticator) BrowserMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if strings.HasPrefix(c.Request.URL.Path, "/api/v1/") && len(c.Request.Header.Values("Authorization")) > 0 {
			if a.authenticateKey(c) {
				c.Next()
			}
			return
		}
		raw, err := c.Cookie(sessionCookie)
		if err == nil {
			sess, user, sessErr := a.store.SessionUser(c.Request.Context(), raw)
			if sessErr == nil {
				c.Request = c.Request.WithContext(WithIdentity(c.Request.Context(), Identity{User: &user, Role: user.Role}))
				c.Set("ac_csrf", sess.CSRFToken)
			} else if errors.Is(sessErr, store.ErrNotFound) {
				clearSessionCookie(c)
			}
		}
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		id, ok := FromContext(c.Request.Context())
		if c.Request.Method == http.MethodPost && c.Request.URL.Path == "/oauth/authorize" && c.ContentType() == "application/x-www-form-urlencoded" {
			c.Next()
			return
		}
		if !ok || id.User == nil {
			c.Next()
			return
		}
		if subtleCompare(CSRF(c), c.GetHeader(csrfHeader)) != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid csrf token"})
			return
		}
		c.Next()
	}
}

func subtleCompare(a, b string) error {
	if a == "" || subtle.ConstantTimeCompare([]byte(a), []byte(b)) != 1 {
		return errors.New("token mismatch")
	}
	return nil
}

// Login creates a session for the user and sets the session cookie.
func (a *Authenticator) Login(c *gin.Context, userID uint) error {
	id, err := randomToken(32)
	if err != nil {
		return err
	}
	csrf, err := randomToken(32)
	if err != nil {
		return err
	}
	now := a.now()
	sess := store.Session{ID: id, UserID: userID, CSRFToken: csrf, CreatedAt: now, ExpiresAt: now.Add(a.ttl)}
	if err := a.store.CreateSession(c.Request.Context(), sess); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Secure:   c.Request.TLS != nil || c.GetBool("secureCookies"),
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
	})
	c.Set("ac_csrf", csrf)
	return nil
}

// Logout deletes the current session.
func (a *Authenticator) Logout(c *gin.Context) error {
	raw, err := c.Cookie(sessionCookie)
	if errors.Is(err, http.ErrNoCookie) {
		clearSessionCookie(c)
		return nil
	}
	if err != nil {
		return fmt.Errorf("read session cookie: %w", err)
	}
	if err := a.store.DeleteSession(c.Request.Context(), raw); err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("logout: %w", err)
	}
	clearSessionCookie(c)
	return nil
}

func clearSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		Secure: c.Request.TLS != nil, SameSite: http.SameSiteLaxMode,
	})
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

var sweepOnce sync.Once

// SweepExpiredSessions periodically drops expired sessions until ctx ends.
func (a *Authenticator) SweepExpiredSessions(ctx context.Context, interval time.Duration) {
	sweeper, ok := a.store.(interface {
		DeleteExpiredSessions(ctx context.Context) error
	})
	if !ok {
		return
	}
	sweepOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if sweeper.DeleteExpiredSessions(ctx) == nil {
						continue
					}
				}
			}
		}()
	})
}
