package auth

import (
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"

	"github.com/OXeu/authcenter/internal/apikey"
	"github.com/OXeu/authcenter/internal/store"
)

// Allows intersects key permissions with the owner's current role.
func (i Identity) Allows(permission store.Permission) bool {
	if i.User == nil || !i.User.Active() || permission.AdminOnly() && i.Role != store.RoleAdmin {
		return false
	}
	return i.APIKey == nil || slices.Contains(i.APIKey.Scopes, permission)
}

func (a *Authenticator) authenticateKey(c *gin.Context) bool {
	if len(c.Request.Header.Values("Authorization")) != 1 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return false
	}
	token, err := apikey.ParseHeader(c.GetHeader("Authorization"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
		return false
	}
	key, err := a.store.APIKeyByDigest(c.Request.Context(), apikey.Hash(token))
	if err != nil || key.ExpiresAt != nil && !key.ExpiresAt.After(a.now()) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
		return false
	}
	user, err := a.store.UserByID(c.Request.Context(), key.CreatedBy)
	if err != nil || !user.Active() {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
		return false
	}
	if err := a.store.TouchAPIKey(c.Request.Context(), key.ID); err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return false
	}
	c.Request = c.Request.WithContext(WithIdentity(c.Request.Context(), Identity{User: &user, Role: user.Role, APIKey: &key}))
	return true
}
