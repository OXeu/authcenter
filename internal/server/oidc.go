package server

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/OXeu/authcenter/internal/oidc"
	"github.com/OXeu/authcenter/internal/store"
)

func (s *Server) jwks(c *gin.Context) {
	if s.cfg.OIDC == nil {
		c.JSON(http.StatusOK, gin.H{"keys": []oidc.JWK{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"keys": s.cfg.OIDC.Keys()})
}

func (s *Server) validateOpenIDRequest(c *gin.Context, query url.Values, scopes []string, public bool) string {
	if !slices.Contains(scopes, "openid") {
		return ""
	}
	if s.cfg.OIDC == nil || slices.Contains(scopes, mcpScope) || query.Get("resource") != "" {
		return "invalid_scope"
	}
	if query.Get("response_type") != "code" || !validOpenIDRedirect(query.Get("redirect_uri"), public) ||
		len(query.Get("nonce")) > 1024 || query.Has("nonce") && query.Get("nonce") == "" {
		return "invalid_request"
	}
	if query.Get("request") != "" {
		return "request_not_supported"
	}
	if query.Get("request_uri") != "" {
		return "request_uri_not_supported"
	}
	for _, unsupported := range []string{"max_age", "claims", "id_token_hint"} {
		if query.Has(unsupported) {
			return "invalid_request"
		}
	}
	if mode := query.Get("response_mode"); mode != "" && mode != "query" {
		return "invalid_request"
	}
	prompts := strings.Fields(query.Get("prompt"))
	for _, prompt := range prompts {
		if prompt != "consent" && prompt != "none" {
			return "invalid_request"
		}
	}
	if slices.Contains(prompts, "none") {
		if len(prompts) != 1 {
			return "invalid_request"
		}
		if _, ok := s.me(c); !ok {
			return "login_required"
		}
		return "consent_required"
	}
	return ""
}

func validOpenIDRedirect(raw string, public bool) bool {
	parsed, err := url.Parse(raw)
	if err != nil || raw == "" || parsed.Scheme == "" || parsed.Fragment != "" || parsed.User != nil || strings.ContainsAny(raw, "\r\n") {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return validAppURL(raw)
	case "javascript", "data", "vbscript", "file", "blob":
		return false
	}
	return public && (parsed.Host != "" || parsed.Path != "" || parsed.Opaque != "")
}

func (s *Server) idToken(c *gin.Context, grant store.OAuthGrant, access string) (string, error) {
	if s.cfg.OIDC == nil {
		return "", errors.New("OpenID signing is not configured")
	}
	user, err := s.store.UserByID(c.Request.Context(), grant.UserID)
	if err != nil {
		return "", fmt.Errorf("load ID Token user: %w", err)
	}
	if !user.Active() {
		return "", fmt.Errorf("issue ID Token: %w", store.ErrInvalidGrant)
	}
	now := time.Now().UTC()
	hash := sha256.Sum256([]byte(access))
	claims := oidc.Claims{
		Issuer: s.cfg.PublicURL, Subject: strconv.FormatUint(uint64(user.ID), 10), Audience: grant.ClientID,
		IssuedAt: now.Unix(), ExpiresAt: now.Add(5 * time.Minute).Unix(), Nonce: grant.Nonce,
		AccessHash: base64.RawURLEncoding.EncodeToString(hash[:16]),
	}
	scopes := strings.Fields(grant.Scope)
	if slices.Contains(scopes, "profile") {
		claims.Name, claims.Picture = user.Name, user.AvatarURL
	}
	if slices.Contains(scopes, "email") {
		claims.Email, claims.EmailVerified = user.Email, &user.EmailVerified
	}
	token, err := s.cfg.OIDC.Sign(claims)
	if err != nil {
		return "", fmt.Errorf("issue ID Token: %w", err)
	}
	return token, nil
}
