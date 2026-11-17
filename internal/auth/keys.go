package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/xeu/configcenter/internal/apikey"
	"github.com/xeu/configcenter/internal/store"
)

// ActorID returns the account responsible for a session or API key operation.
func (i Identity) ActorID() uint {
	if i.User != nil {
		return i.User.ID
	}
	if i.APIKey != nil {
		return i.APIKey.CreatedBy
	}
	return 0
}

func (a *Authenticator) requestKey(c *gin.Context) (store.APIKey, bool) {
	token, err := apikey.ParseHeader(c.GetHeader("Authorization"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
		return store.APIKey{}, false
	}
	key, err := a.store.APIKeyByDigest(c.Request.Context(), apikey.Hash(token))
	if err != nil {
		a.unauthorised(c, err)
		return store.APIKey{}, false
	}
	if apikeyExpired(key, a.now()) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "api key expired"})
		return store.APIKey{}, false
	}
	return key, true
}

// ManagementMiddleware admits browser users or global keys to project management.
func (a *Authenticator) ManagementMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			RequireUser()(c)
			return
		}
		key, ok := a.requestKey(c)
		if !ok {
			return
		}
		if !key.IsGlobal() {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "global api key required"})
			return
		}
		a.store.TouchAPIKey(c.Request.Context(), key.ID)
		c.Request = c.Request.WithContext(WithIdentity(c.Request.Context(), Identity{APIKey: &key, Role: key.Role}))
		c.Next()
	}
}
