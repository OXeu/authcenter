package server

import (
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
)

func (s *Server) authorizationMetadata(c *gin.Context) {
	metadata := gin.H{
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
	}
	if s.cfg.OIDC != nil {
		metadata["jwks_uri"] = s.cfg.PublicURL + "/oauth/jwks"
		metadata["subject_types_supported"] = []string{"public"}
		metadata["id_token_signing_alg_values_supported"] = []string{"RS256"}
		metadata["claims_supported"] = []string{"iss", "sub", "aud", "iat", "exp", "nonce", "at_hash", "name", "picture", "email", "email_verified"}
		metadata["claims_parameter_supported"] = false
		metadata["request_parameter_supported"] = false
		metadata["request_uri_parameter_supported"] = false
		if c.Request.URL.Path == "/.well-known/openid-configuration" {
			delete(metadata, "registration_endpoint")
		}
	} else {
		metadata["scopes_supported"] = slices.DeleteFunc(slices.Clone(supportedScopes), func(scope string) bool { return scope == "openid" })
	}
	c.JSON(http.StatusOK, metadata)
}
