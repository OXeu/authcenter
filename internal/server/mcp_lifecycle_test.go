package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/OXeu/authcenter/internal/store"
)

func registeredTokens(t *testing.T, f *flows, client, callback string) map[string]any {
	t.Helper()
	query := registeredAuthorizationQuery(f, client, callback)
	query.Set("scope", "mcp offline_access")
	code := registrationCode(t, f, query, true)
	response := tokenRequest(f.router, url.Values{"client_id": {client}, "grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callback}, "resource": {f.srv.mcpResource()}, "code_verifier": {strings.Repeat("v", 43)}})
	if response.Code != http.StatusOK {
		t.Fatal("registered token exchange failed", response.Code, response.Body.String())
	}
	return responseJSON(t, response)
}

func TestMCPRegistrationSharedGrantIsolation(t *testing.T) {
	f := newFlows(t)
	callback := "cursor:/callback"
	client := registerMCPClient(t, f, []string{callback})["client_id"].(string)
	first := registeredTokens(t, f, client, callback)
	user, err := f.srv.store.CreateUser(t.Context(), store.User{Email: "other@example.com", Name: "Other", Role: store.RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	other := signedInUser(t, f, user)
	second := registeredTokens(t, other, client, callback)
	unrelated := registerMCPClient(t, f, []string{callback})["client_id"].(string)
	unrelatedTokens := registeredTokens(t, f, unrelated, callback)
	firstGrant, err := f.srv.store.AccessTokenGrant(t.Context(), digest(first["access_token"].(string)))
	if err != nil {
		t.Fatal(err)
	}
	secondGrant, err := f.srv.store.AccessTokenGrant(t.Context(), digest(second["access_token"].(string)))
	if err != nil {
		t.Fatal(err)
	}
	if response := f.request("DELETE", "/api/v1/grants/"+secondGrant.ID, nil); response.Code != http.StatusNotFound {
		t.Fatal("another user's grant can be revoked", response.Code)
	}
	for _, token := range []string{"unknown", second["access_token"].(string)} {
		form := url.Values{"client_id": {unrelated}, "token": {token}}
		request := httptest.NewRequest(http.MethodPost, "/oauth/revoke", strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		f.router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatal("unknown or foreign token revocation failed", response.Code)
		}
	}
	query := registeredAuthorizationQuery(f, client, callback)
	pending := f.request("GET", "/oauth/authorize?"+query.Encode(), nil)
	if pending.Code != http.StatusFound || f.request("GET", pending.Header().Get("Location"), nil).Code != http.StatusOK {
		t.Fatal("pending authorization failed")
	}
	requestID := f.cookies[authorizationCookie].Value
	if response := f.request("DELETE", "/api/v1/grants/"+firstGrant.ID, nil); response.Code != http.StatusNoContent {
		t.Fatal("own grant revocation failed", response.Code)
	}
	if _, err := f.srv.store.AuthorizationClientByID(t.Context(), client); err != nil {
		t.Fatal("registration with another user's grant was deleted", err)
	}
	if mcpRequest(t, f.router, first["access_token"].(string), "tools/list", nil).Code != http.StatusUnauthorized {
		t.Fatal("revoked user's token remains usable")
	}
	if mcpRequest(t, f.router, second["access_token"].(string), "tools/list", nil).Code != http.StatusOK {
		t.Fatal("another user's token was revoked")
	}
	if response := other.request("DELETE", "/api/v1/grants/"+secondGrant.ID, nil); response.Code != http.StatusNoContent {
		t.Fatal("last grant revocation failed", response.Code)
	}
	if _, err := f.srv.store.AuthorizationClientByID(t.Context(), client); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("last grant revocation retained registration", err)
	}
	if response := f.request("POST", "/oauth/authorize", map[string]any{"request": requestID, "approve": true}); response.Code != http.StatusBadRequest {
		t.Fatal("pending consent restored a revoked registration", response.Code)
	}
	if response := f.request("GET", "/oauth/authorize?"+query.Encode(), nil); response.Code != http.StatusBadRequest {
		t.Fatal("deleted registration can authorize again", response.Code)
	}
	if mcpRequest(t, f.router, unrelatedTokens["access_token"].(string), "tools/list", nil).Code != http.StatusOK {
		t.Fatal("revocation affected another registration")
	}
}

func TestMCPTokenRevocationDeletesRegistration(t *testing.T) {
	f := newFlows(t)
	callback := "cursor:/callback"
	client := registerMCPClient(t, f, []string{callback})["client_id"].(string)
	tokens := registeredTokens(t, f, client, callback)
	form := url.Values{"client_id": {client}, "token": {tokens["refresh_token"].(string)}}
	request := httptest.NewRequest(http.MethodPost, "/oauth/revoke", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatal("token revocation failed", response.Code)
	}
	if _, err := f.srv.store.AuthorizationClientByID(t.Context(), client); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("token revocation retained registration", err)
	}
	if mcpRequest(t, f.router, tokens["access_token"].(string), "tools/list", nil).Code != http.StatusUnauthorized {
		t.Fatal("token revocation left MCP access usable")
	}
}
