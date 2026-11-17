package server

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/OXeu/authcenter/internal/store"
	"github.com/gin-gonic/gin"
)

func (s *Server) mountTokenEndpoints(r *gin.Engine) {
	public := r.Group("", oauthPublic)
	for _, path := range []string{"/.well-known/oauth-authorization-server", "/.well-known/openid-configuration"} {
		public.GET(path, s.authorizationMetadata)
		public.OPTIONS(path, func(c *gin.Context) { c.Status(http.StatusNoContent) })
	}
	public.POST("/oauth/register", s.registerClient)
	public.POST("/oauth/token", s.token)
	public.POST("/oauth/revoke", s.revokeToken)
	public.GET("/oauth/userinfo", s.userinfo)
	for _, path := range []string{"/oauth/register", "/oauth/token", "/oauth/revoke", "/oauth/userinfo"} {
		public.OPTIONS(path, func(c *gin.Context) { c.Status(http.StatusNoContent) })
	}
}

func (s *Server) authenticateClient(c *gin.Context) (store.OAuthClient, bool) {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/x-www-form-urlencoded" {
		oauthError(c, 400, "invalid_request")
		return store.OAuthClient{}, false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
	if err := c.Request.ParseForm(); err != nil {
		oauthError(c, 400, "invalid_request")
		return store.OAuthClient{}, false
	}
	if c.Request.URL.RawQuery != "" {
		oauthError(c, 400, "invalid_request")
		return store.OAuthClient{}, false
	}
	for _, values := range c.Request.PostForm {
		if len(values) != 1 {
			oauthError(c, 400, "invalid_request")
			return store.OAuthClient{}, false
		}
	}
	id, secret := c.PostForm("client_id"), c.PostForm("client_secret")
	if c.GetHeader("Authorization") != "" {
		basicID, basicSecret, ok := c.Request.BasicAuth()
		if !ok || id != "" || secret != "" {
			oauthError(c, 400, "invalid_request")
			return store.OAuthClient{}, false
		}
		id, err = url.QueryUnescape(basicID)
		if err != nil {
			oauthError(c, 400, "invalid_request")
			return store.OAuthClient{}, false
		}
		secret, err = url.QueryUnescape(basicSecret)
		if err != nil {
			oauthError(c, 400, "invalid_request")
			return store.OAuthClient{}, false
		}
	}
	client, err := s.store.OAuthClientByID(c.Request.Context(), id)
	valid := err == nil
	if valid && client.Public {
		valid = secret == "" && c.GetHeader("Authorization") == ""
	}
	if valid && !client.Public {
		valid = secret != "" && subtle.ConstantTimeCompare([]byte(client.SecretHash), []byte(digest(secret))) == 1
	}
	if !valid {
		c.Header("WWW-Authenticate", `Basic realm="authcenter"`)
		oauthError(c, 401, "invalid_client")
		return store.OAuthClient{}, false
	}
	return client, true
}

func (s *Server) token(c *gin.Context) {
	client, ok := s.authenticateClient(c)
	if !ok {
		return
	}
	resource := c.PostForm("resource")
	if resource != "" && resource != s.mcpResource() {
		oauthError(c, 400, "invalid_target")
		return
	}
	access, refresh := rand.Text()+rand.Text(), rand.Text()+rand.Text()
	refreshHash := digest(refresh)
	token := store.OAuthToken{AccessHash: digest(access), RefreshHash: &refreshHash, ExpiresAt: time.Now().UTC().Add(time.Hour), RefreshExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour)}
	var grant store.OAuthGrant
	var err error
	switch c.PostForm("grant_type") {
	case "authorization_code":
		redirect := c.PostForm("redirect_uri")
		if redirect == "" {
			redirect = client.RedirectURIs[0]
		}
		if !slices.Contains(client.RedirectURIs, redirect) {
			oauthError(c, 400, "invalid_grant")
			return
		}
		verifier := c.PostForm("code_verifier")
		challenge := ""
		if verifier != "" {
			if !validVerifier(verifier) {
				oauthError(c, 400, "invalid_grant")
				return
			}
			challenge = challengeFor(verifier)
		}
		if client.Public && challenge == "" {
			oauthError(c, 400, "invalid_grant")
			return
		}
		if resource != "" && (challenge == "" || c.PostForm("redirect_uri") == "") {
			oauthError(c, 400, "invalid_grant")
			return
		}
		grant, err = s.store.RedeemAuthorizationCode(c.Request.Context(), digest(c.PostForm("code")), client.ID, redirect, challenge, resource, token)
	case "refresh_token":
		if c.PostForm("scope") != "" {
			oauthError(c, 400, "invalid_scope")
			return
		}
		grant, err = s.store.RotateOAuthToken(c.Request.Context(), digest(c.PostForm("refresh_token")), client.ID, resource, token)
	default:
		oauthError(c, 400, "unsupported_grant_type")
		return
	}
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrInvalidGrant) {
			oauthError(c, 400, "invalid_grant")
		} else {
			s.log.Error("issue OAuth token", "error", err)
			oauthError(c, 500, "server_error")
		}
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	body := gin.H{"access_token": access, "token_type": "Bearer", "expires_in": 3600, "scope": grant.Scope}
	if slices.Contains(strings.Fields(grant.Scope), "offline_access") {
		body["refresh_token"] = refresh
	}
	c.JSON(200, body)
}

func (s *Server) revokeToken(c *gin.Context) {
	client, ok := s.authenticateClient(c)
	if !ok {
		return
	}
	if c.PostForm("token") == "" {
		oauthError(c, 400, "invalid_request")
		return
	}
	if err := s.store.RevokeOAuthToken(c.Request.Context(), digest(c.PostForm("token")), client.ID); err != nil {
		oauthError(c, 500, "server_error")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
}

func (s *Server) userinfo(c *gin.Context) {
	parts := strings.Fields(c.GetHeader("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		c.Header("WWW-Authenticate", `Bearer realm="authcenter"`)
		oauthError(c, 401, "invalid_token")
		return
	}
	grant, err := s.store.AccessTokenGrant(c.Request.Context(), digest(parts[1]))
	if err != nil || grant.Resource != "" {
		c.Header("WWW-Authenticate", `Bearer error="invalid_token"`)
		oauthError(c, 401, "invalid_token")
		return
	}
	user, err := s.store.UserByID(c.Request.Context(), grant.UserID)
	if err != nil {
		oauthError(c, 401, "invalid_token")
		return
	}
	body := gin.H{"sub": strconv.FormatUint(uint64(user.ID), 10), "id": user.ID}
	scopes := strings.Fields(grant.Scope)
	if slices.Contains(scopes, "profile") {
		body["name"], body["picture"], body["avatar_url"] = user.Name, user.AvatarURL, user.AvatarURL
	}
	if slices.Contains(scopes, "email") {
		body["email"] = user.Email
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, body)
}
