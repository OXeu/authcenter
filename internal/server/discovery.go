package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) authorizationMetadata(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"issuer":                                     s.cfg.PublicURL,
		"authorization_endpoint":                     s.cfg.PublicURL + "/oauth/authorize",
		"token_endpoint":                             s.cfg.PublicURL + "/oauth/token",
		"registration_endpoint":                      s.cfg.PublicURL + "/oauth/register",
		"revocation_endpoint":                        s.cfg.PublicURL + "/oauth/revoke",
		"userinfo_endpoint":                          s.cfg.PublicURL + "/oauth/userinfo",
		"response_types_supported":                   []string{"code"},
		"response_modes_supported":                   []string{"query"},
		"grant_types_supported":                      []string{"authorization_code", "refresh_token"},
		"token_endpoint_auth_methods_supported":      []string{"client_secret_basic", "client_secret_post", "none"},
		"revocation_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post", "none"},
		"code_challenge_methods_supported":           []string{"S256"},
		"scopes_supported":                           supportedScopes,
	})
}
