package server

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/OXeu/authcenter/internal/store"
)

func oauthPublic(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
	c.Next()
}

type clientRegistration struct {
	Name          string   `json:"client_name"`
	RedirectURIs  []string `json:"redirect_uris"`
	GrantTypes    []string `json:"grant_types"`
	ResponseTypes []string `json:"response_types"`
	AuthMethod    string   `json:"token_endpoint_auth_method"`
	Scope         string   `json:"scope"`
}

func (s *Server) registerClient(c *gin.Context) {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		oauthError(c, http.StatusBadRequest, "invalid_client_metadata")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var input clientRegistration
	decoder := json.NewDecoder(c.Request.Body)
	if err := decoder.Decode(&input); err != nil {
		oauthError(c, http.StatusBadRequest, "invalid_client_metadata")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		oauthError(c, http.StatusBadRequest, "invalid_client_metadata")
		return
	}
	if len(input.RedirectURIs) == 0 || len(input.RedirectURIs) > 20 {
		oauthError(c, http.StatusBadRequest, "invalid_client_metadata")
		return
	}
	if len(input.GrantTypes) == 0 {
		input.GrantTypes = []string{"authorization_code"}
	}
	if len(input.ResponseTypes) == 0 {
		input.ResponseTypes = []string{"code"}
	}
	if len(input.Name) > 128 || input.AuthMethod != "" && input.AuthMethod != "none" || len(input.ResponseTypes) != 1 || input.ResponseTypes[0] != "code" || !slices.Contains(input.GrantTypes, "authorization_code") || len(input.GrantTypes) > 2 {
		oauthError(c, http.StatusBadRequest, "invalid_client_metadata")
		return
	}
	for i, grant := range input.GrantTypes {
		if grant != "authorization_code" && grant != "refresh_token" || slices.Contains(input.GrantTypes[:i], grant) {
			oauthError(c, http.StatusBadRequest, "invalid_client_metadata")
			return
		}
	}
	refresh := slices.Contains(input.GrantTypes, "refresh_token")
	scopes := strings.Fields(input.Scope)
	if len(scopes) == 0 {
		scopes = []string{mcpScope}
		if refresh {
			scopes = append(scopes, "offline_access")
		}
	}
	if !slices.Contains(scopes, mcpScope) || len(scopes) > 2 {
		oauthError(c, http.StatusBadRequest, "invalid_client_metadata")
		return
	}
	for i, scope := range scopes {
		if scope != mcpScope && (scope != "offline_access" || !refresh) || slices.Contains(scopes[:i], scope) {
			oauthError(c, http.StatusBadRequest, "invalid_client_metadata")
			return
		}
	}
	if !s.rateLimit(c, "oauth-register", c.ClientIP(), 30, 60) {
		return
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "MCP client"
	}
	client := store.OAuthRegistration{ID: rand.Text(), Name: name, RedirectURIs: input.RedirectURIs, Scopes: scopes, CreatedAt: time.Now().UTC()}
	if err := s.store.CreateOAuthRegistration(c.Request.Context(), client); err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"client_id": client.ID, "client_id_issued_at": client.CreatedAt.Unix(), "client_name": client.Name,
		"redirect_uris": client.RedirectURIs, "grant_types": input.GrantTypes, "response_types": input.ResponseTypes,
		"token_endpoint_auth_method": "none", "scope": strings.Join(scopes, " "),
	})
}
