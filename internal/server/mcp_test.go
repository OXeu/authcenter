package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/OXeu/authcenter/internal/store"
)

func mcpAuthorization(t *testing.T, f *flows, client, verifier string) string {
	t.Helper()
	q := url.Values{"client_id": {client}, "response_type": {"code"}, "redirect_uri": {"http://localhost:3000/callback"}, "scope": {"mcp offline_access"}, "resource": {f.srv.mcpResource()}, "state": {"mcp-state"}, "code_challenge": {challengeFor(verifier)}, "code_challenge_method": {"S256"}}
	response := f.request("GET", "/oauth/authorize?"+q.Encode(), nil)
	if response.Code != 302 || !strings.HasPrefix(response.Header().Get("Location"), "/oauth/authorize?") {
		t.Fatalf("authorize: %d %s", response.Code, response.Header().Get("Location"))
	}
	if page := f.request("GET", response.Header().Get("Location"), nil); page.Code != 200 || !strings.Contains(page.Body.String(), "oauth.scope.mcp") {
		t.Fatal("MCP consent missing")
	}
	consent := f.request("POST", "/oauth/authorize", map[string]any{"request": f.cookies[authorizationCookie].Value, "approve": true})
	redirect, err := url.Parse(responseJSON(t, consent)["redirect"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if redirect.Query().Get("iss") != f.srv.cfg.PublicURL || redirect.Query().Get("state") != "mcp-state" {
		t.Fatal("issuer/state missing")
	}
	return redirect.Query().Get("code")
}

func mcpTokens(t *testing.T, f *flows) (clientID string, tokens map[string]any) {
	t.Helper()
	client, _ := createClient(t, f, true)
	verifier := strings.Repeat("v", 43)
	code := mcpAuthorization(t, f, client, verifier)
	response := tokenRequest(f.router, url.Values{"client_id": {client}, "grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {"http://localhost:3000/callback"}, "resource": {f.srv.mcpResource()}, "code_verifier": {verifier}})
	if response.Code != 200 {
		t.Fatalf("exchange MCP token: %d %s", response.Code, response.Body.String())
	}
	return client, responseJSON(t, response)
}

func mcpRequest(t *testing.T, router http.Handler, token, method string, params any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", "2025-11-25")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func mcpCall(t *testing.T, f *flows, token, name string, args map[string]any) map[string]any {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	response := mcpRequest(t, f.router, token, "tools/call", map[string]any{"name": name, "arguments": args})
	if response.Code != 200 {
		t.Fatalf("MCP HTTP %d: %s", response.Code, response.Body.String())
	}
	body := responseJSON(t, response)
	result, ok := body["result"].(map[string]any)
	if !ok {
		t.Fatalf("MCP result missing: %v", body)
	}
	return result
}

func toolStatus(t *testing.T, result map[string]any) int {
	t.Helper()
	return int(result["structuredContent"].(map[string]any)["http_status"].(float64))
}

func TestMCPDiscoveryAndOAuthResourceBinding(t *testing.T) {
	f := newFlows(t)
	response := mcpRequest(t, f.router, "", "tools/list", map[string]any{})
	if response.Code != 401 || !strings.Contains(response.Header().Get("WWW-Authenticate"), `resource_metadata="http://cc.test/.well-known/oauth-protected-resource/mcp"`) {
		t.Fatal(response.Code, response.Header())
	}
	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
		metadata := responseJSON(t, f.request("GET", path, nil))
		if metadata["resource"] != f.srv.mcpResource() {
			t.Fatal(metadata)
		}
	}
	if got := f.request("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}); got.Code != 401 {
		t.Fatal("browser session accepted as MCP OAuth")
	}
	client, secret := createClient(t, f, false)
	q := url.Values{"client_id": {client}, "redirect_uri": {"http://localhost:3000/callback"}, "scope": {"mcp"}, "resource": {f.srv.mcpResource()}}
	if r := f.request("GET", "/oauth/authorize?"+q.Encode(), nil); !strings.Contains(r.Header().Get("Location"), "error=invalid_request") {
		t.Fatal("confidential MCP client bypassed PKCE")
	}
	q.Set("code_challenge", challengeFor(strings.Repeat("v", 43)))
	q.Set("code_challenge_method", "S256")
	for _, resource := range []string{"", "https://other.example/mcp"} {
		q.Set("resource", resource)
		if r := f.request("GET", "/oauth/authorize?"+q.Encode(), nil); !strings.Contains(r.Header().Get("Location"), "error=invalid_target") {
			t.Fatal("invalid resource accepted")
		}
	}
	legacy := exchange(t, f, client, secret, issueCode(t, f, client, ""), "")
	if mcpRequest(t, f.router, legacy["access_token"].(string), "tools/list", nil).Code != 401 {
		t.Fatal("userinfo token accepted for MCP")
	}
	verifier := strings.Repeat("v", 43)
	code := mcpAuthorization(t, f, client, verifier)
	values := url.Values{"client_id": {client}, "client_secret": {secret}, "grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {"http://localhost:3000/callback"}, "code_verifier": {verifier}}
	if tokenRequest(f.router, values).Code != 400 {
		t.Fatal("resource omitted at exchange")
	}
	values.Set("resource", "https://other.example/mcp")
	if tokenRequest(f.router, values).Code != 400 {
		t.Fatal("wrong resource accepted")
	}
	values.Set("resource", f.srv.mcpResource())
	tokens := responseJSON(t, tokenRequest(f.router, values))
	access := tokens["access_token"].(string)
	if userinfoStatus(f.router, access) != 401 {
		t.Fatal("MCP token accepted by userinfo")
	}
	if mcpRequest(t, f.router, access, "tools/list", nil).Code != 200 {
		t.Fatal("valid MCP token rejected")
	}
	refresh := url.Values{"client_id": {client}, "client_secret": {secret}, "grant_type": {"refresh_token"}, "refresh_token": {tokens["refresh_token"].(string)}}
	if tokenRequest(f.router, refresh).Code != 400 {
		t.Fatal("refresh lost resource binding")
	}
	refresh.Set("resource", f.srv.mcpResource())
	rotated := responseJSON(t, tokenRequest(f.router, refresh))
	if mcpRequest(t, f.router, rotated["access_token"].(string), "tools/list", nil).Code != 200 {
		t.Fatal("rotated token invalid")
	}
	if tokenRequest(f.router, refresh).Code != 400 || mcpRequest(t, f.router, access, "tools/list", nil).Code != 401 {
		t.Fatal("refresh replay did not revoke grant")
	}
}

func TestMCPToolParityAndPermissions(t *testing.T) {
	f := newFlows(t)
	_, tokens := mcpTokens(t, f)
	token := tokens["access_token"].(string)
	result := responseJSON(t, mcpRequest(t, f.router, token, "tools/list", nil))["result"].(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) != len(f.srv.apiOperations()) {
		t.Fatalf("tool coverage %d", len(tools))
	}
	for _, op := range f.srv.apiOperations() {
		if op.Method != "GET" {
			continue
		}
		rest := f.request("GET", "/api/v1"+op.Path, nil)
		tool := mcpCall(t, f, token, op.Name, nil)
		if toolStatus(t, tool) != rest.Code {
			t.Fatalf("%s status differs", op.Name)
		}
		if op.Name != "get_identity" {
			got := tool["structuredContent"].(map[string]any)["body"]
			want := responseJSON(t, rest)
			actual, _ := json.Marshal(got)
			expected, _ := json.Marshal(want)
			if !bytes.Equal(actual, expected) {
				t.Fatalf("%s body differs: %s != %s", op.Name, actual, expected)
			}
		}
	}
	created := mcpCall(t, f, token, "create_client", clientBody(t))
	if toolStatus(t, created) != 201 {
		t.Fatal(created)
	}
	body := created["structuredContent"].(map[string]any)["body"].(map[string]any)
	client := body["client"].(map[string]any)["id"].(string)
	if toolStatus(t, mcpCall(t, f, token, "delete_client", map[string]any{"id": client})) != 204 {
		t.Fatal("tool mutation failed")
	}
	for _, args := range []map[string]any{{"id": "../users"}, {"id": 1}, {"id": "x", "unexpected": true}} {
		if mcpCall(t, f, token, "delete_client", args)["isError"] != true {
			t.Fatal("invalid arguments accepted")
		}
	}
	user, err := f.srv.store.CreateUser(t.Context(), store.User{Email: "mcp-user@example.com", Name: "User", Role: store.RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	other := signedInUser(t, f, user)
	_, userTokens := mcpTokens(t, other)
	userToken := userTokens["access_token"].(string)
	listed := responseJSON(t, mcpRequest(t, f.router, userToken, "tools/list", nil))["result"].(map[string]any)["tools"].([]any)
	for _, item := range listed {
		name := item.(map[string]any)["name"].(string)
		if slices.Contains([]string{"list_users", "get_settings", "delete_user"}, name) {
			t.Fatal("admin tool exposed")
		}
	}
	denied := responseJSON(t, mcpRequest(t, f.router, userToken, "tools/call", map[string]any{"name": "list_users", "arguments": map[string]any{}}))
	if denied["error"] == nil {
		t.Fatal("unknown admin tool accepted")
	}
	ownedClient, _ := createClient(t, f, false)
	if toolStatus(t, mcpCall(t, f, userToken, "delete_client", map[string]any{"id": ownedClient})) != 403 {
		t.Fatal("client ownership bypass")
	}
	grant, err := f.srv.store.AccessTokenGrant(t.Context(), digest(token))
	if err != nil {
		t.Fatal(err)
	}
	if toolStatus(t, mcpCall(t, f, userToken, "revoke_grant", map[string]any{"id": grant.ID})) != 404 {
		t.Fatal("grant ownership bypass")
	}
	if toolStatus(t, mcpCall(t, f, token, "revoke_grant", map[string]any{"id": grant.ID})) != 204 || mcpRequest(t, f.router, token, "tools/list", nil).Code != 401 {
		t.Fatal("revocation ineffective")
	}
}

type mcpTransport struct {
	token string
	host  string
	base  http.RoundTripper
}

func (t mcpTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+t.token)
	if t.host != "" {
		r.Host = t.host
	}
	return t.base.RoundTrip(r)
}

func TestMCPSDKAndLiveAuthorization(t *testing.T) {
	f := newFlows(t)
	clientID, tokens := mcpTokens(t, f)
	token := tokens["access_token"].(string)
	server := httptest.NewServer(f.router)
	defer server.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "authcenter-test", Version: "1.0.0"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: server.URL + "/mcp", HTTPClient: &http.Client{Transport: mcpTransport{token: token, base: http.DefaultTransport}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_identity", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatal(result, err)
	}
	grant, err := f.srv.store.AccessTokenGrant(t.Context(), digest(token))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.srv.store.SetUserRole(t.Context(), grant.UserID, store.RoleUser); err != nil {
		t.Fatal(err)
	}
	if got := responseJSON(t, mcpRequest(t, f.router, token, "tools/call", map[string]any{"name": "list_users", "arguments": map[string]any{}})); got["error"] == nil {
		t.Fatal("stale administrator role")
	}
	for _, origin := range []string{"null", "https://evil.example"} {
		req := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{}`))
		req.Header.Set("Origin", origin)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		f.router.ServeHTTP(w, req)
		if w.Code != 403 {
			t.Fatal("origin accepted")
		}
	}
	if err := f.srv.store.DisableOAuthClient(t.Context(), clientID); err != nil {
		t.Fatal(err)
	}
	if mcpRequest(t, f.router, token, "tools/list", nil).Code != 401 {
		t.Fatal("deleted client remains authorized")
	}
}

func TestMCPBehindLoopbackProxy(t *testing.T) {
	f := newFlows(t)
	f.srv.cfg.PublicURL = "https://auth.xeu.life"
	_, tokens := mcpTokens(t, f)
	token := tokens["access_token"].(string)
	server := httptest.NewServer(f.router)
	defer server.Close()
	for _, tc := range []struct {
		name, method, token, origin string
		status                      int
	}{
		{"tools", "POST", token, "", 200},
		{"same origin", "POST", token, f.srv.cfg.PublicURL, 200},
		{"no event stream", "GET", token, "", 405},
		{"no session deletion", "DELETE", token, "", 405},
		{"unauthenticated GET", "GET", "", "", 401},
		{"unauthenticated POST", "POST", "", "", 401},
		{"invalid token", "POST", "invalid", "", 401},
		{"foreign origin", "POST", token, "https://other.example", 403},
		{"null origin", "POST", token, "null", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), tc.method, server.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
			if err != nil {
				t.Fatal(err)
			}
			req.Host = "auth.xeu.life"
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("MCP-Protocol-Version", "2025-11-25")
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			response, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != tc.status {
				t.Fatalf("proxied MCP status = %d, want %d", response.StatusCode, tc.status)
			}
			if tc.status == 401 && !strings.Contains(response.Header.Get("WWW-Authenticate"), f.srv.cfg.PublicURL+"/.well-known/oauth-protected-resource/mcp") {
				t.Fatal("proxied authentication lost resource discovery")
			}
		})
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "proxy-test", Version: "1.0.0"}, nil)
	transport := mcpTransport{token: token, host: "auth.xeu.life", base: http.DefaultTransport}
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: server.URL + "/mcp", HTTPClient: &http.Client{Transport: transport}}, nil)
	if err != nil {
		t.Fatal("proxied MCP initialization failed", err)
	}
	defer session.Close()
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_identity", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatal("proxied MCP tool call failed", result, err)
	}
}

func TestMCPExpiredTokenAndInsufficientScope(t *testing.T) {
	f := newFlows(t)
	client, _ := createClient(t, f, true)
	user, err := f.srv.store.UserByEmail(t.Context(), "admin@localhost")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, scope string
		expiry      time.Time
		status      int
	}{{"expired", mcpScope, time.Now().Add(-time.Hour), 401}, {"scope", "profile", time.Now().Add(time.Hour), 403}} {
		t.Run(tc.name, func(t *testing.T) {
			req := store.AuthorizationRequest{ID: tc.name, ClientID: client, UserID: user.ID, RedirectURI: "http://localhost:3000/callback", ExpiresAt: time.Now().Add(time.Hour)}
			grant := store.OAuthGrant{ID: tc.name, ClientID: client, UserID: user.ID, Scope: tc.scope, Resource: f.srv.mcpResource()}
			code := store.AuthorizationCode{Digest: tc.name, GrantID: grant.ID, ClientID: client, RedirectURI: req.RedirectURI, ExpiresAt: time.Now().Add(time.Hour)}
			if err := f.srv.store.CreateAuthorizationRequest(t.Context(), req); err != nil {
				t.Fatal(err)
			}
			if err := f.srv.store.DecideAuthorization(t.Context(), req, grant, code, true); err != nil {
				t.Fatal(err)
			}
			if _, err := f.srv.store.RedeemAuthorizationCode(t.Context(), code.Digest, client, req.RedirectURI, "", grant.Resource, store.OAuthToken{AccessHash: digest(tc.name), ExpiresAt: tc.expiry}); err != nil {
				t.Fatal(err)
			}
			if got := mcpRequest(t, f.router, tc.name, "tools/list", nil); got.Code != tc.status {
				t.Fatal(got.Code, got.Body.String())
			}
		})
	}
}

func TestMCPEmailChangeBindsGrantAndRevokesCredentials(t *testing.T) {
	f, emailCode := localFlows(t)
	registerLocal(t, f, emailCode)
	_, firstTokens := mcpTokens(t, f)
	_, otherTokens := mcpTokens(t, f)
	first, other := firstTokens["access_token"].(string), otherTokens["access_token"].(string)
	_, key := createKey(t, f, []string{"account:read"})
	started := mcpCall(t, f, first, "start_email_change", map[string]any{"email": "mcp-new@example.com", "password": "correct horse battery"})
	if toolStatus(t, started) != 200 {
		t.Fatal(started)
	}
	body := started["structuredContent"].(map[string]any)["body"].(map[string]any)
	args := map[string]any{"request": body["request"], "code": *emailCode}
	if toolStatus(t, mcpCall(t, f, other, "verify_email_change", args)) != 401 {
		t.Fatal("email change crossed OAuth grants")
	}
	if f.request("POST", "/api/v1/account/email/verify", args).Code != 401 {
		t.Fatal("email change crossed to browser session")
	}
	if toolStatus(t, mcpCall(t, f, first, "verify_email_change", args)) != 204 {
		t.Fatal("same grant rejected")
	}
	for _, token := range []string{first, other} {
		if mcpRequest(t, f.router, token, "tools/list", nil).Code != 401 {
			t.Fatal("account change left OAuth token valid")
		}
	}
	if keyRequest(t, f, key, "GET", "/api/v1/me", nil).Code != 401 {
		t.Fatal("account change left API key valid")
	}
}

func TestMCPTransportValidation(t *testing.T) {
	f := newFlows(t)
	_, tokens := mcpTokens(t, f)
	token := tokens["access_token"].(string)
	for _, method := range []string{"GET", "DELETE"} {
		req := httptest.NewRequest(method, "/mcp", http.NoBody)
		req.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		f.router.ServeHTTP(response, req)
		if response.Code != 405 {
			t.Fatal(method, response.Code)
		}
	}
	for _, tc := range []struct{ name, version, body string }{
		{"invalid-json", "2025-11-25", "{"},
		{"oversized", "2025-11-25", strings.Repeat("x", (1<<20)+1)},
		{"unknown-version", "2099-01-01", `{"jsonrpc":"2.0","id":1,"method":"ping"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/mcp", strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("MCP-Protocol-Version", tc.version)
			response := httptest.NewRecorder()
			f.router.ServeHTTP(response, req)
			if response.Code < 400 {
				t.Fatal(response.Code, response.Body.String())
			}
		})
	}
}

func TestMCPKeyManagement(t *testing.T) {
	f := newFlows(t)
	_, tokens := mcpTokens(t, f)
	token := tokens["access_token"].(string)
	result := mcpCall(t, f, token, "create_key", map[string]any{"name": "MCP created", "scopes": []string{"clients:read"}})
	if toolStatus(t, result) != 201 {
		t.Fatal(result)
	}
	body := result["structuredContent"].(map[string]any)["body"].(map[string]any)
	id := fmt.Sprint(body["key"].(map[string]any)["id"])
	key := body["token"].(string)
	if got := keyRequest(t, f, key, "POST", "/mcp", nil); got.Code != 401 {
		t.Fatal("MCP accepted API key")
	}
	if keyRequest(t, f, key, "GET", "/api/v1/clients", nil).Code != 200 {
		t.Fatal("MCP key unusable")
	}
	if toolStatus(t, mcpCall(t, f, token, "rotate_key", map[string]any{"id": id})) != 200 || keyRequest(t, f, key, "GET", "/api/v1/clients", nil).Code != 401 {
		t.Fatal("MCP rotation failed")
	}
	if toolStatus(t, mcpCall(t, f, token, "delete_key", map[string]any{"id": id})) != 204 {
		t.Fatal("MCP deletion failed")
	}
}
