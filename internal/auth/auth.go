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

	"github.com/OXeu/configcenter/internal/store"
)

// Authenticator resolves browser sessions and API keys.
type Authenticator struct {
	store Store
	keys  ProjectLookup
	ttl   time.Duration
	now   func() time.Time
}

const (
	sessionCookie = "cc_session"
	stateCookie   = "cc_oauth_state"
	csrfHeader    = "X-CSRF-Token"
)

// Store is the subset of persistence the authenticator needs.
type Store interface {
	ProjectLookup
	SessionUser(ctx context.Context, id string) (store.Session, store.User, error)
	CreateSession(ctx context.Context, sess store.Session) error
	DeleteSession(ctx context.Context, id string) error
	ProjectBySlug(ctx context.Context, slug string) (store.Project, error)
	APIKeyByDigest(ctx context.Context, digest [32]byte) (store.APIKey, error)
	TouchAPIKey(ctx context.Context, id uint)
}

// ProjectLookup resolves the project a bearer token belongs to.
type ProjectLookup interface {
	SlugForKey(ctx context.Context, header string) (string, error)
}

// AgentStore additionally resolves sessions for the MCP endpoint.
type AgentStore interface {
	SessionUser(ctx context.Context, id string) (store.Session, store.User, error)
}

// AgentMiddleware authenticates an MCP call either with a project API key or,
// for the web UI, with a browser session carrying the CSRF token.
func (a *Authenticator) AgentMiddleware(st AgentStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		if header := c.GetHeader("Authorization"); header != "" {
			a.BearerMiddleware()(c)
			return
		}
		raw, err := c.Cookie(sessionCookie)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "api key or session required"})
			return
		}
		sess, user, err := st.SessionUser(c.Request.Context(), raw)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "session expired"})
			return
		}
		if subtleCompare(sess.CSRFToken, c.GetHeader(csrfHeader)) != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid csrf token"})
			return
		}
		project, err := a.store.ProjectBySlug(c.Request.Context(), c.Param("project"))
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, store.ErrNotFound) {
				status = http.StatusNotFound
			}
			c.AbortWithStatusJSON(status, gin.H{"error": "project not found"})
			return
		}
		if user.Role != store.RoleAdmin && project.OwnerID != user.ID {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		identity := Identity{User: &user, Project: &project, Role: user.Role}
		c.Request = c.Request.WithContext(WithIdentity(c.Request.Context(), identity))
		c.Next()
	}
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
	User    *store.User
	APIKey  *store.APIKey
	Project *store.Project
	Role    store.Role
}

// CanWrite reports whether the principal may change configuration.
func (i Identity) CanWrite() bool {
	return i.Role.CanWrite()
}

// New builds an Authenticator.
func New(s Store, keys ProjectLookup, ttl time.Duration) *Authenticator {
	return &Authenticator{
		store: s,
		keys:  keys,
		ttl:   ttl,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

type (
	identityKey struct{}
	projectKey  struct{}
)

// WithProjectSlug records which project a bearer token was resolved to.
func WithProjectSlug(ctx context.Context, slug string) context.Context {
	return context.WithValue(ctx, projectKey{}, slug)
}

func projectSlug(ctx context.Context) string {
	slug, ok := ctx.Value(projectKey{}).(string)
	if !ok {
		return ""
	}
	return slug
}

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
	tok, ok := c.Get("cc_csrf")
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
		if c.GetHeader("Authorization") != "" && (strings.HasPrefix(c.Request.URL.Path, "/api/") || strings.HasPrefix(c.Request.URL.Path, "/mcp/")) {
			c.Next()
			return
		}
		raw, err := c.Cookie(sessionCookie)
		if err == nil {
			sess, user, sessErr := a.store.SessionUser(c.Request.Context(), raw)
			if sessErr == nil {
				c.Request = c.Request.WithContext(WithIdentity(c.Request.Context(), Identity{User: &user, Role: user.Role}))
				c.Set("cc_csrf", sess.CSRFToken)
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

// BearerMiddleware authenticates `Authorization: ApiKey <token>` requests,
// resolving the key inside the project named by the :project route parameter.
func (a *Authenticator) BearerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key, ok := a.requestKey(c)
		if !ok {
			return
		}
		slugParam := c.Param("project")
		if slugParam == "" {
			slugParam = projectSlug(c.Request.Context())
		}
		if slugParam == "" && key.IsGlobal() {
			slugParam = c.Query("project")
			if slugParam == "" {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "project is required for a global api key"})
				return
			}
		}
		if slugParam == "" {
			var err error
			slugParam, err = a.keys.SlugForKey(c.Request.Context(), c.GetHeader("Authorization"))
			if err != nil {
				a.unauthorised(c, err)
				return
			}
		}
		project, err := a.store.ProjectBySlug(c.Request.Context(), slugParam)
		if err != nil {
			a.unauthorised(c, err)
			return
		}
		if !key.IsGlobal() && key.ProjectID != project.ID {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "api key does not belong to this project"})
			return
		}
		a.store.TouchAPIKey(c.Request.Context(), key.ID)
		ctx := WithIdentity(c.Request.Context(), Identity{APIKey: &key, Project: &project, Role: key.Role})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func (a *Authenticator) unauthorised(c *gin.Context, err error) {
	if errors.Is(err, store.ErrNotFound) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
		return
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
}

func apikeyExpired(k store.APIKey, now time.Time) bool {
	return k.ExpiresAt != nil && !k.ExpiresAt.After(now)
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
		Secure:   c.Request.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
	})
	c.Set("cc_csrf", csrf)
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
