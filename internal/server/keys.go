package server

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/OXeu/authcenter/internal/apikey"
	"github.com/OXeu/authcenter/internal/auth"
	"github.com/OXeu/authcenter/internal/store"
)

func canManageKey(id auth.Identity, key store.APIKey) bool {
	if id.User == nil {
		return false
	}
	if id.APIKey != nil {
		withinExpiry := id.APIKey.ExpiresAt == nil || key.ExpiresAt != nil && !key.ExpiresAt.After(*id.APIKey.ExpiresAt)
		return key.CreatedBy == id.User.ID && id.APIKey.Covers(key.Scopes) && withinExpiry
	}
	return id.Role == store.RoleAdmin || key.CreatedBy == id.User.ID
}

func (s *Server) apiKeyScopes(c *gin.Context) {
	id, _ := auth.FromContext(c.Request.Context())
	permissions := []store.Permission{}
	for _, permission := range store.KeyPermissions() {
		if id.Allows(permission) {
			permissions = append(permissions, permission)
		}
	}
	c.JSON(200, gin.H{"permissions": permissions})
}

func (s *Server) apiListKeys(c *gin.Context) {
	keys, err := s.store.ListAPIKeys(c.Request.Context())
	if err != nil {
		s.fail(c, err)
		return
	}
	id, _ := auth.FromContext(c.Request.Context())
	visible := []store.APIKey{}
	for _, key := range keys {
		if canManageKey(id, key) {
			visible = append(visible, key)
		}
	}
	c.JSON(200, gin.H{"keys": visible})
}

func (s *Server) keyInput(c *gin.Context, key store.APIKey) (store.APIKey, bool) {
	var in struct {
		Name      string              `json:"name"`
		Scopes    *[]store.Permission `json:"scopes"`
		ExpiresAt *time.Time          `json:"expires_at"`
	}
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return key, false
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 128 || in.Scopes == nil || len(*in.Scopes) > len(store.KeyPermissions()) {
		s.fail(c, fmt.Errorf("%w: name and valid scopes are required", errBadRequest))
		return key, false
	}
	id, _ := auth.FromContext(c.Request.Context())
	for index, scope := range *in.Scopes {
		if !slices.Contains(store.KeyPermissions(), scope) || slices.Contains((*in.Scopes)[:index], scope) {
			s.fail(c, fmt.Errorf("%w: unknown or duplicate scope", errBadRequest))
			return key, false
		}
		if !id.Allows(scope) {
			oauthError(c, 403, "insufficient_scope")
			return key, false
		}
	}
	if key.ID == 0 {
		key.ExpiresAt = in.ExpiresAt
		if key.ExpiresAt != nil && !key.ExpiresAt.After(time.Now().UTC()) {
			s.fail(c, fmt.Errorf("%w: expiry must be in the future", errBadRequest))
			return key, false
		}
		if id.APIKey != nil && id.APIKey.ExpiresAt != nil && (key.ExpiresAt == nil || key.ExpiresAt.After(*id.APIKey.ExpiresAt)) {
			oauthError(c, 403, "insufficient_scope")
			return key, false
		}
	}
	key.Name, key.Scopes = in.Name, *in.Scopes
	return key, true
}

func (s *Server) apiCreateKey(c *gin.Context) {
	key, ok := s.keyInput(c, store.APIKey{})
	if !ok {
		return
	}
	token, hash, err := apikey.Generate()
	if err != nil {
		s.fail(c, err)
		return
	}
	id, _ := auth.FromContext(c.Request.Context())
	key.CreatedBy, key.CreatedAt, key.Digest, key.Prefix = id.User.ID, time.Now().UTC(), hash[:], apikey.Display(token)
	key, err = s.store.CreateAPIKey(c.Request.Context(), key)
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(201, gin.H{"key": key, "token": token})
}

func (s *Server) managedKey(c *gin.Context) (store.APIKey, bool) {
	id, err := userID(c)
	if err != nil {
		s.fail(c, err)
		return store.APIKey{}, false
	}
	key, err := s.store.APIKeyByID(c.Request.Context(), id)
	if err != nil {
		s.fail(c, err)
		return key, false
	}
	identity, _ := auth.FromContext(c.Request.Context())
	if !canManageKey(identity, key) {
		oauthError(c, 403, "insufficient_scope")
		return key, false
	}
	return key, true
}

func (s *Server) apiUpdateKey(c *gin.Context) {
	key, ok := s.managedKey(c)
	if !ok {
		return
	}
	key, ok = s.keyInput(c, key)
	if !ok {
		return
	}
	if err := s.store.UpdateAPIKey(c.Request.Context(), key); err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(200, gin.H{"key": key})
}

func (s *Server) apiDeleteKey(c *gin.Context) {
	key, ok := s.managedKey(c)
	if !ok {
		return
	}
	if err := s.store.DeleteAPIKey(c.Request.Context(), key); err != nil {
		s.fail(c, err)
		return
	}
	c.Status(204)
}

func (s *Server) apiRotateKey(c *gin.Context) {
	key, ok := s.managedKey(c)
	if !ok {
		return
	}
	token, hash, err := apikey.Generate()
	if err != nil {
		s.fail(c, err)
		return
	}
	if err := s.store.RotateAPIKey(c.Request.Context(), key, hash, apikey.Display(token)); err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(200, gin.H{"token": token})
}
