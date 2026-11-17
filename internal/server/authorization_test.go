package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OXeu/authcenter/internal/store"
)

func (f *flows) request(method, path string, body any) *httptest.ResponseRecorder {
	f.t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			f.t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Authcenter-Request", "1")
	req.Header.Set("X-CSRF-Token", f.csrf)
	for _, cookie := range f.cookies {
		req.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, req)
	for _, cookie := range response.Result().Cookies() {
		if cookie.MaxAge < 0 {
			delete(f.cookies, cookie.Name)
		} else {
			f.cookies[cookie.Name] = cookie
		}
	}
	return response
}

func responseJSON(t *testing.T, r *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %d: %v", r.Code, err)
	}
	return body
}

func clientBody(t *testing.T) map[string]any {
	t.Helper()
	var logo bytes.Buffer
	if err := png.Encode(&logo, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	return map[string]any{"name": "Example", "homepage": "https://app.example", "redirect_uris": []string{"https://app.example/callback", "http://localhost:3000/callback"}, "logo": base64.StdEncoding.EncodeToString(logo.Bytes()), "public": false}
}

func createClient(t *testing.T, f *flows, public bool) (id, secret string) {
	t.Helper()
	body := clientBody(t)
	body["public"] = public
	response := f.request("POST", "/api/v1/clients", body)
	if response.Code != 201 {
		t.Fatalf("create client: %d %s", response.Code, response.Body.String())
	}
	result := responseJSON(t, response)
	return result["client"].(map[string]any)["id"].(string), result["client_secret"].(string)
}

func issueCode(t *testing.T, f *flows, clientID, verifier string) string {
	t.Helper()
	params := url.Values{"client_id": {clientID}, "redirect_uri": {"http://localhost:3000/callback"}, "scope": {"profile email offline_access"}, "state": {"state-with-special+chars"}}
	if verifier != "" {
		params.Set("code_challenge", challengeFor(verifier))
		params.Set("code_challenge_method", "S256")
	}
	response := f.request("GET", "/oauth/authorize?"+params.Encode(), nil)
	if response.Code != 302 {
		t.Fatalf("authorize: %d %s", response.Code, response.Body.String())
	}
	consent := f.request("GET", response.Header().Get("Location"), nil)
	if consent.Code != 200 || !strings.Contains(consent.Body.String(), `"page":"consent"`) {
		t.Fatalf("consent page: %d", consent.Code)
	}
	request := f.cookies[authorizationCookie].Value
	response = f.request("POST", "/oauth/authorize", map[string]any{"request": request, "approve": true})
	if response.Code != 200 {
		t.Fatalf("consent: %d %s", response.Code, response.Body.String())
	}
	redirect, err := url.Parse(responseJSON(t, response)["redirect"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if redirect.Query().Get("state") != "state-with-special+chars" || redirect.Query().Get("code") == "" || redirect.Query().Has("iss") {
		t.Fatal("unexpected plain OAuth response")
	}
	return redirect.Query().Get("code")
}

func tokenRequest(router http.Handler, values url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func exchange(t *testing.T, f *flows, id, secret, code, verifier string) map[string]any {
	t.Helper()
	response := tokenRequest(f.router, url.Values{"client_id": {id}, "client_secret": {secret}, "grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {"http://localhost:3000/callback"}, "code_verifier": {verifier}})
	if response.Code != 200 {
		t.Fatalf("exchange: %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("token is cacheable")
	}
	return responseJSON(t, response)
}

func userinfoStatus(router http.Handler, token string) int {
	req := httptest.NewRequest("GET", "/oauth/userinfo", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response.Code
}

func TestOAuthProviderRoundTripAndRefreshReplay(t *testing.T) {
	f := newFlows(t)
	id, secret := createClient(t, f, false)
	code := issueCode(t, f, id, "")
	tokens := exchange(t, f, id, secret, code, "")
	if userinfoStatus(f.router, tokens["access_token"].(string)) != 200 {
		t.Fatal("access token rejected")
	}
	values := url.Values{"client_id": {id}, "client_secret": {secret}, "grant_type": {"refresh_token"}, "refresh_token": {tokens["refresh_token"].(string)}}
	response := tokenRequest(f.router, values)
	if response.Code != 200 {
		t.Fatalf("refresh: %d %s", response.Code, response.Body.String())
	}
	rotated := responseJSON(t, response)
	if rotated["refresh_token"] == tokens["refresh_token"] {
		t.Fatal("refresh token was not rotated")
	}
	if tokenRequest(f.router, values).Code != 400 {
		t.Fatal("refresh replay accepted")
	}
	if userinfoStatus(f.router, rotated["access_token"].(string)) != 401 {
		t.Fatal("replayed family remains active")
	}
}

func TestPKCEClientIsolationAndConcurrentCodeUse(t *testing.T) {
	f := newFlows(t)
	id, secret := createClient(t, f, true)
	other, otherSecret := createClient(t, f, false)
	verifier := strings.Repeat("a", 43)
	code := issueCode(t, f, id, verifier)
	values := url.Values{"client_id": {other}, "client_secret": {otherSecret}, "grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {"http://localhost:3000/callback"}, "code_verifier": {verifier}}
	if tokenRequest(f.router, values).Code != 400 {
		t.Fatal("code crossed client boundary")
	}
	values.Set("client_id", id)
	values.Set("client_secret", secret)
	values.Set("code_verifier", strings.Repeat("b", 43))
	if tokenRequest(f.router, values).Code != 400 {
		t.Fatal("wrong PKCE accepted")
	}
	values.Set("code_verifier", verifier)
	var wait sync.WaitGroup
	results := make(chan int, 2)
	for range 2 {
		wait.Go(func() { results <- tokenRequest(f.router, values).Code })
	}
	wait.Wait()
	close(results)
	successes := 0
	for status := range results {
		if status == 200 {
			successes++
		} else if status != 400 {
			t.Fatalf("concurrent exchange status %d", status)
		}
	}
	if successes != 1 {
		t.Fatalf("code redeemed %d times", successes)
	}
}

func TestCallbacksConsentAndEditableApplications(t *testing.T) {
	f := newFlows(t)
	id, _ := createClient(t, f, false)
	bad := f.request("GET", "/oauth/authorize?client_id="+id+"&redirect_uri=https://evil.example", nil)
	if bad.Code != 400 || bad.Header().Get("Location") != "" {
		t.Fatal("unregistered redirect followed")
	}
	response := f.request("GET", "/oauth/authorize?client_id="+id, nil)
	request := f.cookies[authorizationCookie].Value
	f.request("GET", response.Header().Get("Location"), nil)
	csrf := f.csrf
	f.csrf = ""
	if f.request("POST", "/oauth/authorize", map[string]any{"request": request, "approve": true}).Code != 403 {
		t.Fatal("consent bypassed CSRF")
	}
	f.csrf = csrf
	denied := f.request("POST", "/oauth/authorize", map[string]any{"request": request, "approve": false})
	if denied.Code != 200 || !strings.Contains(responseJSON(t, denied)["redirect"].(string), "error=access_denied") {
		t.Fatal("denial did not redirect with error")
	}
	edited := clientBody(t)
	edited["name"] = "Edited"
	edited["homepage"] = "https://new.example"
	edited["redirect_uris"] = []string{"https://new.example/callback"}
	edited["logo"] = ""
	if response := f.request("PATCH", "/api/v1/clients/"+id, edited); response.Code != 200 {
		t.Fatalf("edit: %d %s", response.Code, response.Body.String())
	}
	logo := f.request("GET", "/applications/"+id+"/logo", nil)
	if logo.Code != 200 || logo.Header().Get("Content-Type") != "image/png" {
		t.Fatal("logo missing")
	}
	client, err := f.srv.store.OAuthClientByID(context.Background(), id)
	if err != nil || client.Name != "Edited" || len(client.RedirectURIs) != 1 {
		t.Fatal("metadata not saved")
	}
	old := f.request("GET", "/oauth/authorize?client_id="+id+"&redirect_uri=https://app.example/callback", nil)
	if old.Code != 400 {
		t.Fatal("removed callback accepted")
	}
}

func TestClientRegistrationPreservesCallbackValues(t *testing.T) {
	f := newFlows(t)
	redirects := []string{
		"different-app://unrestricted-host/custom/path?context=one#section",
		"http://remote.example/callback", "relative/callback", "not a URI", "",
		"https://example.com/callback?code=old&error=old&state=old&iss=old",
	}
	body := clientBody(t)
	body["redirect_uris"] = redirects
	response := f.request("POST", "/api/v1/clients", body)
	if response.Code != 201 {
		t.Fatal("manual registration restricted callback values", response.Code, response.Body.String())
	}
	clientID := responseJSON(t, response)["client"].(map[string]any)["id"].(string)
	client, err := f.srv.store.OAuthClientByID(t.Context(), clientID)
	if err != nil || !slices.Equal(client.RedirectURIs, redirects) {
		t.Fatal("manual registration changed callbacks", err)
	}
	redirects = append(redirects, "another-app:/callback")
	body["redirect_uris"] = redirects
	if response := f.request("PATCH", "/api/v1/clients/"+clientID, body); response.Code != 200 {
		t.Fatal("callback update rejected", response.Code, response.Body.String())
	}
	client, err = f.srv.store.OAuthClientByID(t.Context(), clientID)
	if err != nil || !slices.Equal(client.RedirectURIs, redirects) {
		t.Fatal("callback update changed values", err)
	}
}

func TestUserOwnershipAndClientRevocation(t *testing.T) {
	f := newFlows(t)
	id, secret := createClient(t, f, false)
	code := issueCode(t, f, id, "")
	tokens := exchange(t, f, id, secret, code, "")
	user, err := f.srv.store.CreateUser(context.Background(), store.User{Email: "other@example.com", Name: "Other", Role: store.RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.srv.store.CreateSession(context.Background(), store.Session{ID: "other-session", UserID: user.ID, CSRFToken: "other-csrf", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	other := &flows{t: t, router: f.router, srv: f.srv, csrf: "other-csrf", cookies: map[string]*http.Cookie{"ac_session": {Name: "ac_session", Value: "other-session"}}}
	if other.request("PATCH", "/api/v1/clients/"+id, clientBody(t)).Code != 403 {
		t.Fatal("client ownership bypass")
	}
	if other.request("DELETE", "/api/v1/clients/"+id, nil).Code != 403 {
		t.Fatal("client deletion crossed accounts")
	}
	createClient(t, other, false)
	grants := responseJSON(t, f.request("GET", "/api/v1/grants", nil))["grants"].([]any)
	grant := grants[0].(map[string]any)["id"].(string)
	if other.request("DELETE", "/api/v1/grants/"+grant, nil).Code != 404 {
		t.Fatal("grant ownership bypass")
	}
	if f.request("DELETE", "/api/v1/clients/"+id, nil).Code != 204 {
		t.Fatal("delete client failed")
	}
	if userinfoStatus(f.router, tokens["access_token"].(string)) != 401 {
		t.Fatal("disabled client token accepted")
	}
}
