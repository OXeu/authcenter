package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/OXeu/authcenter/internal/auth"
	"github.com/OXeu/authcenter/internal/store"
	"github.com/gin-gonic/gin"
)

const authorizationCookie = "ac_authorization"

func (s *Server) mountAuthorization(r *gin.Engine) {
	r.GET("/oauth/authorize", s.authorize)
	r.POST("/oauth/authorize", auth.RequireUser(), s.consent)
	r.GET("/applications/:id/logo", s.clientLogo)
}

func (s *Server) loginDestination(c *gin.Context) string {
	request, err := c.Cookie(authorizationCookie)
	if err == nil && request != "" {
		return "/oauth/authorize?request=" + url.QueryEscape(request)
	}
	return "/applications"
}

func oauthError(c *gin.Context, status int, code string) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(status, gin.H{"error": code})
}

func authorizeRedirect(uri, state, key, value string) string {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "/login"
	}
	q := parsed.Query()
	q.Set(key, value)
	if state != "" {
		q.Set("state", state)
	}
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

func challengeFor(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func validVerifier(verifier string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	for _, r := range verifier {
		valid := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-._~", r)
		if !valid {
			return false
		}
	}
	return true
}

func (s *Server) authorize(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	if c.Query("request") != "" {
		s.authorizationPage(c)
		return
	}
	q, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil || len(c.Request.URL.RawQuery) > 8192 {
		oauthError(c, 400, "invalid_request")
		return
	}
	for _, v := range q {
		if len(v) != 1 {
			oauthError(c, 400, "invalid_request")
			return
		}
	}
	client, err := s.store.OAuthClientByID(c.Request.Context(), q.Get("client_id"))
	if err != nil {
		oauthError(c, 400, "invalid_client")
		return
	}
	redirect := q.Get("redirect_uri")
	if redirect == "" {
		redirect = client.RedirectURIs[0]
	}
	if !slices.Contains(client.RedirectURIs, redirect) {
		oauthError(c, 400, "invalid_request")
		return
	}
	fail := func(code string) {
		c.Redirect(http.StatusFound, s.resourceRedirect(redirect, q.Get("state"), "error", code, q.Get("resource")))
	}
	if q.Get("response_type") != "" && q.Get("response_type") != "code" {
		fail("unsupported_response_type")
		return
	}
	scopes := strings.Fields(q.Get("scope"))
	if len(scopes) == 0 {
		scopes = []string{"profile", "email"}
	}
	for _, scope := range scopes {
		if !slices.Contains(client.Scopes, scope) {
			fail("invalid_scope")
			return
		}
	}
	resource := q.Get("resource")
	isMCP := slices.Contains(scopes, mcpScope)
	if resource != "" && resource != s.mcpResource() || isMCP && resource != s.mcpResource() {
		fail("invalid_target")
		return
	}
	if resource != "" && !isMCP {
		fail("invalid_scope")
		return
	}
	if isMCP && q.Get("redirect_uri") == "" {
		fail("invalid_request")
		return
	}
	challenge := q.Get("code_challenge")
	if challenge != "" || client.Public || isMCP {
		decoded, decodeErr := base64.RawURLEncoding.DecodeString(challenge)
		if decodeErr != nil || len(decoded) != 32 || len(challenge) != 43 || q.Get("code_challenge_method") != "S256" {
			fail("invalid_request")
			return
		}
	} else if q.Get("code_challenge_method") != "" {
		fail("invalid_request")
		return
	}
	request := store.AuthorizationRequest{ID: rand.Text(), ClientID: client.ID, RedirectURI: redirect, Scope: strings.Join(scopes, " "), Resource: resource, State: q.Get("state"), Challenge: challenge, ExpiresAt: time.Now().UTC().Add(10 * time.Minute)}
	if err := s.store.CreateAuthorizationRequest(c.Request.Context(), request); err != nil {
		oauthError(c, 500, "server_error")
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: authorizationCookie, Value: request.ID, Path: "/", HttpOnly: true, Secure: c.GetBool("secureCookies"), SameSite: http.SameSiteLaxMode, MaxAge: 600})
	c.Redirect(http.StatusFound, "/oauth/authorize?request="+url.QueryEscape(request.ID))
}

func (s *Server) authorizationPage(c *gin.Context) {
	raw, err := c.Cookie(authorizationCookie)
	if err != nil || raw != c.Query("request") {
		oauthError(c, 400, "invalid_request")
		return
	}
	request, err := s.store.AuthorizationRequestByID(c.Request.Context(), raw)
	if err != nil {
		oauthError(c, 400, "invalid_request")
		return
	}
	user, ok := s.me(c)
	if !ok {
		c.Redirect(http.StatusFound, "/login")
		return
	}
	if err := s.store.BindAuthorizationRequest(c.Request.Context(), raw, digest(auth.CSRF(c)), user.ID); err != nil {
		oauthError(c, 500, "server_error")
		return
	}
	client, err := s.store.OAuthClientByID(c.Request.Context(), request.ClientID)
	if err != nil || !slices.Contains(client.RedirectURIs, request.RedirectURI) {
		oauthError(c, 400, "invalid_request")
		return
	}
	s.render(c, 200, gin.H{"Page": "consent", "Title": "oauth.consent", "Authorization": gin.H{"id": raw, "client": client, "scopes": strings.Fields(request.Scope), "redirect_uri": request.RedirectURI}})
}

func (s *Server) consent(c *gin.Context) {
	var in struct {
		Request string `json:"request"`
		Approve *bool  `json:"approve"`
	}
	if err := bindJSON(c, &in); err != nil || in.Approve == nil {
		oauthError(c, 400, "invalid_request")
		return
	}
	raw, err := c.Cookie(authorizationCookie)
	if err != nil || raw != in.Request {
		oauthError(c, 400, "invalid_request")
		return
	}
	request, err := s.store.AuthorizationRequestByID(c.Request.Context(), raw)
	if err != nil {
		oauthError(c, 400, "invalid_request")
		return
	}
	user, _ := s.me(c)
	if request.UserID != user.ID || request.CSRFHash != digest(auth.CSRF(c)) {
		oauthError(c, 403, "access_denied")
		return
	}
	client, err := s.store.OAuthClientByID(c.Request.Context(), request.ClientID)
	if err != nil || !slices.Contains(client.RedirectURIs, request.RedirectURI) {
		oauthError(c, 400, "invalid_request")
		return
	}
	code := rand.Text() + rand.Text()
	grant := store.OAuthGrant{ID: rand.Text(), ClientID: request.ClientID, UserID: user.ID, Scope: request.Scope, Resource: request.Resource}
	record := store.AuthorizationCode{Digest: digest(code), GrantID: grant.ID, ClientID: request.ClientID, RedirectURI: request.RedirectURI, Challenge: request.Challenge, ExpiresAt: time.Now().UTC().Add(5 * time.Minute)}
	if err := s.store.DecideAuthorization(c.Request.Context(), request, grant, record, *in.Approve); err != nil {
		oauthError(c, 400, "invalid_request")
		return
	}
	clearCookie(c, authorizationCookie, "/")
	key, value := "code", code
	if !*in.Approve {
		key, value = "error", "access_denied"
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, gin.H{"redirect": s.resourceRedirect(request.RedirectURI, request.State, key, value, request.Resource)})
}

func (s *Server) resourceRedirect(uri, state, key, value, resource string) string {
	redirect := authorizeRedirect(uri, state, key, value)
	if resource != "" {
		return authorizeRedirect(redirect, state, "iss", s.cfg.PublicURL)
	}
	return redirect
}
