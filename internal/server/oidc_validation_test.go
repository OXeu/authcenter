package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOpenIDRedirectPolicy(t *testing.T) {
	for _, tc := range []struct {
		uri    string
		public bool
		want   bool
	}{
		{"https://app.example/callback", false, true},
		{"http://localhost:3000/callback", false, true},
		{"http://127.0.0.1:3000/callback", true, true},
		{"cursor://app/callback", true, true},
		{"cursor:/callback", true, true},
		{"cursor:/callback", false, false},
		{"https://app.example/callback#fragment", false, false},
		{"http://remote.example/callback", true, false},
		{"https://user:password@app.example/callback", true, false},
		{"javascript:alert(1)", true, false},
		{"data:text/html,hello", true, false},
		{"file:///callback", true, false},
		{"/callback", true, false},
	} {
		if got := validOpenIDRedirect(tc.uri, tc.public); got != tc.want {
			t.Errorf("redirect %q public=%v: got %v, want %v", tc.uri, tc.public, got, tc.want)
		}
	}
}

func TestOpenIDAuthorizationValidation(t *testing.T) {
	f := newFlows(t)
	enableOpenID(t, f)
	id, _ := createClient(t, f, false)
	cases := []struct{ key, value, want string }{
		{"response_type", "", "invalid_request"},
		{"response_type", "id_token", "unsupported_response_type"},
		{"redirect_uri", "", "invalid_request"},
		{"nonce", "", "invalid_request"},
		{"nonce", strings.Repeat("n", 1025), "invalid_request"},
		{"response_mode", "fragment", "invalid_request"},
		{"scope", "openid mcp", "invalid_scope"},
		{"resource", f.srv.mcpResource(), "invalid_scope"},
		{"request", "unsigned.jwt.object", "request_not_supported"},
		{"request_uri", "https://client.example/request", "request_uri_not_supported"},
		{"max_age", "0", "invalid_request"},
		{"claims", `{}`, "invalid_request"},
		{"id_token_hint", "unverified", "invalid_request"},
		{"prompt", "login", "invalid_request"},
		{"prompt", "none consent", "invalid_request"},
		{"prompt", "none", "consent_required"},
	}
	for _, tc := range cases {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			query := openIDQuery(id, "openid", "nonce")
			query.Set(tc.key, tc.value)
			response := f.request("GET", "/oauth/authorize?"+query.Encode(), nil)
			location, err := url.Parse(response.Header().Get("Location"))
			if err != nil || response.Code != http.StatusFound || location.Query().Get("error") != tc.want || location.Query().Get("state") != query.Get("state") {
				t.Fatal("invalid request accepted or wrong error", response.Code, response.Header().Get("Location"))
			}
		})
	}
	query := openIDQuery(id, "openid", "nonce")
	query.Set("prompt", "none")
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, httptest.NewRequest("GET", "/oauth/authorize?"+query.Encode(), http.NoBody))
	if !strings.Contains(response.Header().Get("Location"), "error=login_required") {
		t.Fatal("prompt=none attempted interactive login")
	}
	query.Del("prompt")
	response = f.request("GET", "/oauth/authorize?"+query.Encode()+"&nonce=duplicate", nil)
	if response.Code != http.StatusBadRequest {
		t.Fatal("duplicate nonce accepted")
	}
	response = f.request("POST", "/oauth/register", map[string]any{"redirect_uris": []string{"https://client.example/callback"}, "scope": "mcp openid"})
	if response.Code != http.StatusBadRequest {
		t.Fatal("MCP registration granted OpenID access")
	}
}

func TestOpenIDCodeBindingsAndReplay(t *testing.T) {
	f := newFlows(t)
	enableOpenID(t, f)
	id, secret := createClient(t, f, false)
	other, otherSecret := createClient(t, f, false)
	verifier := strings.Repeat("a", 43)
	query := openIDQuery(id, "openid", "stored-nonce")
	query.Set("code_challenge", challengeFor(verifier))
	query.Set("code_challenge_method", "S256")
	code := approveOpenID(t, f, query)
	values := url.Values{"client_id": {id}, "client_secret": {secret}, "grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier}, "redirect_uri": {query.Get("redirect_uri")}, "nonce": {"forged-token-nonce"}, "scope": {"openid email"}}
	for _, mutate := range []func(url.Values){
		func(v url.Values) { v.Del("redirect_uri") },
		func(v url.Values) { v.Set("redirect_uri", "https://app.example/callback") },
		func(v url.Values) { v.Set("code_verifier", strings.Repeat("b", 43)) },
		func(v url.Values) { v.Set("client_id", other); v.Set("client_secret", otherSecret) },
	} {
		invalid, err := url.ParseQuery(values.Encode())
		if err != nil {
			t.Fatal(err)
		}
		mutate(invalid)
		if response := tokenRequest(f.router, invalid); response.Code != http.StatusBadRequest || responseJSON(t, response)["id_token"] != nil {
			t.Fatal("invalid code redemption returned an ID Token", response.Code)
		}
	}
	response := tokenRequest(f.router, values)
	if response.Code != http.StatusOK {
		t.Fatal("valid redemption rejected", response.Body.String())
	}
	tokens := responseJSON(t, response)
	claims := verifiedIDClaims(t, f, tokens["id_token"].(string))
	if claims["nonce"] != "stored-nonce" || claims["email"] != nil {
		t.Fatal("token request replaced consented scope or nonce")
	}
	if response := tokenRequest(f.router, values); response.Code != http.StatusBadRequest {
		t.Fatal("authorization code replay issued another ID Token")
	}
	if userinfoStatus(f.router, tokens["access_token"].(string)) != http.StatusUnauthorized {
		t.Fatal("replayed grant retained access")
	}
}

func TestOpenIDOptionalNonceAndPlainOAuthCompatibility(t *testing.T) {
	f := newFlows(t)
	enableOpenID(t, f)
	id, secret := createClient(t, f, false)
	code := approveOpenID(t, f, openIDQuery(id, "openid", ""))
	tokens := exchange(t, f, id, secret, code, "")
	claims := verifiedIDClaims(t, f, tokens["id_token"].(string))
	if _, ok := claims["nonce"]; ok {
		t.Fatal("unsolicited nonce in ID Token")
	}
	code = issueCode(t, f, id, "")
	tokens = exchange(t, f, id, secret, code, "")
	if _, ok := tokens["id_token"]; ok {
		t.Fatal("plain OAuth flow issued an ID Token")
	}
}

func TestOpenIDFormAuthorizationRetainsConsentCSRF(t *testing.T) {
	f := newFlows(t)
	enableOpenID(t, f)
	id, _ := createClient(t, f, false)
	query := openIDQuery(id, "openid profile email", "nonce")
	request := httptest.NewRequest("POST", "/oauth/authorize", strings.NewReader(query.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range f.cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	if response.Code != http.StatusFound || !strings.HasPrefix(response.Header().Get("Location"), "/oauth/authorize?request=") {
		t.Fatal("form authorization failed", response.Code, response.Body.String())
	}
	for _, cookie := range response.Result().Cookies() {
		f.cookies[cookie.Name] = cookie
	}
	f.request("GET", response.Header().Get("Location"), nil)
	f.csrf = ""
	if response := f.request("POST", "/oauth/authorize", map[string]any{"request": f.cookies[authorizationCookie].Value, "approve": true}); response.Code != http.StatusForbidden {
		t.Fatal("OpenID consent bypassed CSRF")
	}
	request = httptest.NewRequest("POST", "/oauth/authorize", strings.NewReader(url.Values{"request": {f.cookies[authorizationCookie].Value}, "approve": {"true"}}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range f.cookies {
		request.AddCookie(cookie)
	}
	response = httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatal("form content type bypassed consent")
	}
}
