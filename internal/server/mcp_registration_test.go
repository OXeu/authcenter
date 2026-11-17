package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/OXeu/authcenter/internal/store"
)

func registerMCPClient(t *testing.T, f *flows, redirects []string) map[string]any {
	t.Helper()
	anonymous := &flows{t: t, srv: f.srv, router: f.router, cookies: map[string]*http.Cookie{}}
	response := anonymous.request("POST", "/oauth/register", map[string]any{
		"client_name": "Cursor", "redirect_uris": redirects,
		"grant_types":    []string{"authorization_code", "refresh_token"},
		"response_types": []string{"code"}, "token_endpoint_auth_method": "none",
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("MCP registration: %d %s", response.Code, response.Body.String())
	}
	registered := responseJSON(t, response)
	if response.Header().Get("Cache-Control") != "no-store" || registered["client_secret"] != nil || registered["token_endpoint_auth_method"] != "none" {
		t.Fatal("invalid public registration response")
	}
	client, err := f.srv.store.AuthorizationClientByID(t.Context(), registered["client_id"].(string))
	if err != nil || !slices.Equal(client.RedirectURIs, redirects) || !client.Public || !client.Dynamic || client.SecretHash != "" || client.OwnerID != 0 {
		t.Fatal("registration did not preserve the public client metadata", err)
	}
	if _, err := f.srv.store.OAuthClientByID(t.Context(), client.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("registration created an application", err)
	}
	listed := responseJSON(t, f.request("GET", "/api/v1/clients", nil))["clients"].([]any)
	for _, item := range listed {
		if item.(map[string]any)["id"] == client.ID {
			t.Fatal("registration appeared in application management")
		}
	}
	return registered
}

func registeredAuthorizationQuery(f *flows, client, redirect string) url.Values {
	return url.Values{
		"client_id": {client}, "redirect_uri": {redirect}, "response_type": {"code"},
		"resource": {f.srv.mcpResource()}, "state": {"cursor-state+value"},
		"code_challenge": {challengeFor(strings.Repeat("v", 43))}, "code_challenge_method": {"S256"},
	}
}

func registrationCode(t *testing.T, f *flows, query url.Values, approve bool) string {
	t.Helper()
	response := f.request("GET", "/oauth/authorize?"+query.Encode(), nil)
	if response.Code != 302 || !strings.HasPrefix(response.Header().Get("Location"), "/oauth/authorize?request=") {
		t.Fatal("registered callback did not reach consent", response.Code, response.Header().Get("Location"))
	}
	if page := f.request("GET", response.Header().Get("Location"), nil); page.Code != 200 || !strings.Contains(page.Body.String(), `"dynamic":true`) {
		t.Fatal("dynamic client consent missing", page.Code)
	}
	request := f.cookies[authorizationCookie].Value
	csrf := f.csrf
	f.csrf = ""
	if f.request("POST", "/oauth/authorize", map[string]any{"request": request, "approve": true}).Code != 403 {
		t.Fatal("dynamic consent bypassed CSRF")
	}
	f.csrf = csrf
	response = f.request("POST", "/oauth/authorize", map[string]any{"request": request, "approve": approve})
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	redirect, err := url.Parse(responseJSON(t, response)["redirect"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if redirect.Query().Get("state") != query.Get("state") || redirect.Query().Get("iss") != f.srv.cfg.PublicURL {
		t.Fatal("OAuth response lost state or issuer")
	}
	callback, err := url.Parse(query.Get("redirect_uri"))
	if err != nil {
		t.Fatal(err)
	}
	if redirect.Scheme != callback.Scheme || redirect.Host != callback.Host || redirect.Path != callback.Path || redirect.Fragment != callback.Fragment {
		t.Fatal("OAuth response substituted the callback")
	}
	if !approve {
		if redirect.Query().Get("error") != "access_denied" || redirect.Query().Has("code") {
			t.Fatal("consent denial issued a code")
		}
		return ""
	}
	if redirect.Query().Get("code") == "" || redirect.Query().Has("error") {
		t.Fatal("consent did not issue a code")
	}
	return redirect.Query().Get("code")
}

func TestMCPCursorRegistration(t *testing.T) {
	f := newFlows(t)
	redirects := []string{
		"cursor://anysphere.cursor-mcp/oauth/callback",
		"https://www.cursor.com/agents/mcp/oauth/callback",
		"http://localhost:8787/callback",
		"http://desktop.example:8765/callback",
		"custom-app:/oauth/callback?instance=one",
	}
	registered := registerMCPClient(t, f, redirects)
	client := registered["client_id"].(string)
	for _, callback := range redirects {
		t.Run(callback, func(t *testing.T) {
			query := registeredAuthorizationQuery(f, client, callback)
			code := registrationCode(t, f, query, true)
			form := url.Values{"client_id": {client}, "grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callback}, "resource": {f.srv.mcpResource()}, "code_verifier": {strings.Repeat("v", 43)}}
			for _, invalid := range []struct{ field, value string }{
				{"redirect_uri", "other-app://callback"},
				{"code_verifier", strings.Repeat("x", 43)},
				{"resource", "https://other.example/mcp"},
			} {
				original := form.Get(invalid.field)
				form.Set(invalid.field, invalid.value)
				if tokenRequest(f.router, form).Code != 400 {
					t.Fatalf("invalid %s accepted", invalid.field)
				}
				form.Set(invalid.field, original)
			}
			tokens := tokenRequest(f.router, form)
			if tokens.Code != 200 {
				t.Fatal(tokens.Code, tokens.Body.String())
			}
			access := responseJSON(t, tokens)["access_token"].(string)
			if toolStatus(t, mcpCall(t, f, access, "get_identity", nil)) != 200 {
				t.Fatal("registered client cannot call MCP")
			}
		})
	}
	query := registeredAuthorizationQuery(f, client, redirects[0])
	query.Set("scope", "mcp offline_access")
	code := registrationCode(t, f, query, true)
	form := url.Values{"client_id": {client}, "grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirects[0]}, "resource": {f.srv.mcpResource()}, "code_verifier": {strings.Repeat("v", 43)}}
	tokens := responseJSON(t, tokenRequest(f.router, form))
	refresh := url.Values{"client_id": {client}, "grant_type": {"refresh_token"}, "refresh_token": {tokens["refresh_token"].(string)}, "resource": {f.srv.mcpResource()}}
	rotated := tokenRequest(f.router, refresh)
	if rotated.Code != 200 {
		t.Fatal("registered client refresh failed", rotated.Body.String())
	}
	if f.request("PATCH", "/api/v1/clients/"+client, clientBody(t)).Code != 404 {
		t.Fatal("dynamic registration was edited through management API")
	}
	if f.request("DELETE", "/api/v1/clients/"+client, nil).Code != 404 {
		t.Fatal("dynamic registration was treated as an application")
	}
	grants := responseJSON(t, f.request("GET", "/api/v1/grants", nil))["grants"].([]any)
	if len(grants) != len(redirects)+1 {
		t.Fatal("dynamic client grants missing from authorization management")
	}
	for _, item := range grants {
		grant := item.(map[string]any)
		if f.request("DELETE", "/api/v1/grants/"+grant["id"].(string), nil).Code != 204 {
			t.Fatal("user cannot revoke registered client authorization")
		}
	}
	if mcpRequest(t, f.router, responseJSON(t, rotated)["access_token"].(string), "tools/list", nil).Code != 401 {
		t.Fatal("revoked dynamic client remains authorized")
	}
	refresh.Set("refresh_token", responseJSON(t, rotated)["refresh_token"].(string))
	if tokenRequest(f.router, refresh).Code != 401 {
		t.Fatal("revoked dynamic client can refresh tokens")
	}
	if _, err := f.srv.store.AuthorizationClientByID(t.Context(), client); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("revoked registration was retained", err)
	}
}

func TestMCPRegistrationPreservesCallbackValues(t *testing.T) {
	f := newFlows(t)
	redirects := []string{
		"different-app://unrestricted-host/custom/path?context=one#section",
		"http://remote.example/callback", "relative/callback", "not a URI", "",
		"https://example.com/callback?code=old&error=old&state=old&iss=old",
	}
	registerMCPClient(t, f, redirects)
	registered := registerMCPClient(t, f, redirects[5:])
	query := registeredAuthorizationQuery(f, registered["client_id"].(string), redirects[5])
	registrationCode(t, f, query, false)
	registrationCode(t, f, query, true)
}

func TestMCPRegistrationDiscoveryAndPreflight(t *testing.T) {
	f := newFlows(t)
	metadata := responseJSON(t, f.request("GET", "/.well-known/oauth-authorization-server", nil))
	if metadata["registration_endpoint"] != f.srv.cfg.PublicURL+"/oauth/register" {
		t.Fatal("registration endpoint missing from discovery")
	}
	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-authorization-server", "/oauth/register", "/oauth/token", "/oauth/revoke"} {
		req := httptest.NewRequest("OPTIONS", path, http.NoBody)
		req.Header.Set("Origin", "https://client.example")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "content-type")
		response := httptest.NewRecorder()
		f.router.ServeHTTP(response, req)
		if response.Code != 204 || response.Header().Get("Access-Control-Allow-Origin") != "*" || response.Header().Get("Access-Control-Allow-Credentials") != "" {
			t.Fatal("OAuth preflight failed", path, response.Code)
		}
	}
}

func TestMCPRegistrationRejectsInvalidMetadata(t *testing.T) {
	f := newFlows(t)
	for _, fields := range []map[string]any{
		{"redirect_uris": []string{}},
		{"redirect_uris": make([]string, 21)},
		{"token_endpoint_auth_method": "client_secret_basic"},
		{"grant_types": []string{"client_credentials"}},
		{"grant_types": []string{"refresh_token"}},
		{"grant_types": []string{"authorization_code", "authorization_code"}},
		{"response_types": []string{"token"}},
		{"scope": "profile"},
		{"scope": "mcp mcp"},
		{"scope": "mcp offline_access"},
		{"client_name": strings.Repeat("n", 129)},
	} {
		input := map[string]any{"redirect_uris": []string{"cursor://anysphere.cursor-mcp/oauth/callback"}}
		for key, value := range fields {
			input[key] = value
		}
		response := f.request("POST", "/oauth/register", input)
		if response.Code != 400 || responseJSON(t, response)["error"] != "invalid_client_metadata" {
			t.Fatal("invalid registration metadata accepted", fields, response.Code)
		}
	}
	for _, raw := range []string{"{", `{"redirect_uris":["app:/callback"]} {}`, `{"redirect_uris":["app:/callback"],"ignored":"` + strings.Repeat("x", 16<<10) + `"}`} {
		req := httptest.NewRequest("POST", "/oauth/register", strings.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		f.router.ServeHTTP(response, req)
		if response.Code != 400 {
			t.Fatal("malformed registration accepted", response.Code)
		}
	}
	clients, err := f.srv.store.ListOAuthClients(t.Context())
	if err != nil || len(clients) != 0 {
		t.Fatal("invalid registration persisted a client", err)
	}
}

func TestMCPRegisteredClientAuthorizationGuards(t *testing.T) {
	f := newFlows(t)
	callback := "cursor://anysphere.cursor-mcp/oauth/callback"
	client := registerMCPClient(t, f, []string{callback})["client_id"].(string)
	for _, invalid := range []struct{ field, value, code string }{
		{"redirect_uri", "cursor://another-app/oauth/callback", ""},
		{"redirect_uri", "cursor://anysphere.cursor-mcp/oauth/callback?added=yes", ""},
		{"scope", "profile", "invalid_scope"},
		{"scope", "offline_access", "invalid_scope"},
		{"resource", "", "invalid_target"},
		{"resource", "https://other.example/mcp", "invalid_target"},
		{"code_challenge", "", "invalid_request"},
		{"code_challenge_method", "plain", "invalid_request"},
	} {
		query := registeredAuthorizationQuery(f, client, callback)
		query.Set(invalid.field, invalid.value)
		response := f.request("GET", "/oauth/authorize?"+query.Encode(), nil)
		if invalid.code == "" {
			if response.Code != 400 || response.Header().Get("Location") != "" {
				t.Fatal("unregistered callback accepted")
			}
			continue
		}
		redirect, err := url.Parse(response.Header().Get("Location"))
		if err != nil || response.Code != 302 || redirect.Query().Get("error") != invalid.code {
			t.Fatal("invalid authorization accepted", invalid.field, response.Code, err)
		}
	}
	anonymous := &flows{t: t, srv: f.srv, router: f.router, cookies: map[string]*http.Cookie{}}
	query := registeredAuthorizationQuery(f, client, callback)
	start := anonymous.request("GET", "/oauth/authorize?"+query.Encode(), nil)
	if start.Code != 302 {
		t.Fatal(start.Code)
	}
	login := anonymous.request("GET", start.Header().Get("Location"), nil)
	if login.Code != 302 || login.Header().Get("Location") != "/login" {
		t.Fatal("anonymous authorization did not require login")
	}
	f.cookies[authorizationCookie] = anonymous.cookies[authorizationCookie]
	if resumed := f.request("GET", start.Header().Get("Location"), nil); resumed.Code != 200 {
		t.Fatal("login cannot resume registered authorization")
	}
}

func TestMCPRegistrationBodyAndCookies(t *testing.T) {
	f := newFlows(t)
	body, err := json.Marshal(map[string]any{"redirect_uris": []string{"custom-app:/callback"}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/oauth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, cookie := range f.cookies {
		req.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, req)
	if response.Code != 201 {
		t.Fatal("registration incorrectly requires browser CSRF", response.Code)
	}
}
