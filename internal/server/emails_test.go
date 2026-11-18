package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OXeu/authcenter/internal/store"
)

func bindAccountEmail(t *testing.T, f *flows, code *string, address string) {
	t.Helper()
	response := f.request("POST", "/api/v1/account/emails", map[string]string{"email": address, "password": "correct horse battery"})
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	request := responseJSON(t, response)["request"]
	if response := f.request("POST", "/api/v1/account/emails/verify", map[string]any{"request": request, "code": *code}); response.Code != 204 {
		t.Fatal(response.Code, response.Body.String())
	}
}

func assertOAuthEmail(t *testing.T, f *flows, id, secret, email string) string {
	t.Helper()
	code := approveOpenID(t, f, openIDQuery(id, "openid email", "email-selection"))
	tokens := exchange(t, f, id, secret, code, "")
	claims := verifiedIDClaims(t, f, tokens["id_token"].(string))
	if claims["email"] != email || claims["email_verified"] != true || claims["emails"] != nil {
		t.Fatal("wrong ID Token email", claims)
	}
	access := tokens["access_token"].(string)
	request := httptest.NewRequest("GET", "/oauth/userinfo", http.NoBody)
	request.Header.Set("Authorization", "Bearer "+access)
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	info := responseJSON(t, response)
	if response.Code != 200 || info["email"] != email || info["email_verified"] != true || info["emails"] != nil {
		t.Fatal("wrong userinfo email", info)
	}
	return access
}

func TestMultipleEmailsPrimaryClaimsAndLogin(t *testing.T) {
	f, code := localFlows(t)
	registerLocal(t, f, code)
	secret := enrollDevice(t, f, "Phone")
	enableOpenID(t, f)
	id, clientSecret := createClient(t, f, false)
	bindAccountEmail(t, f, code, " Second@Example.com ")
	user, err := f.srv.store.UserByEmail(t.Context(), "second@example.com")
	if err != nil || user.Email != "alice@example.com" {
		t.Fatal("binding changed primary or lost login alias", err)
	}
	emails, err := f.srv.store.AccountEmails(t.Context(), user.ID)
	if err != nil || len(emails) != 2 || !emails[0].Primary || emails[1].Primary || !emails[1].Verified {
		t.Fatal("incorrect binding metadata", emails, err)
	}
	access := assertOAuthEmail(t, f, id, clientSecret, user.Email)
	other := signedInUser(t, f, user)
	primaryPath := fmt.Sprintf("/api/v1/account/emails/%d/primary", emails[1].ID)
	if f.request("PUT", primaryPath, map[string]string{"password": "wrong"}).Code != 401 {
		t.Fatal("primary change did not verify password")
	}
	if response := f.request("PUT", primaryPath, map[string]string{"password": "correct horse battery"}); response.Code != 204 {
		t.Fatal(response.Code, response.Body.String())
	}
	if other.request("GET", "/api/v1/me", nil).Code != 401 || userinfoStatus(f.router, access) != 401 {
		t.Fatal("primary change did not revoke other credentials")
	}
	assertOAuthEmail(t, f, id, clientSecret, "second@example.com")
	if f.request("DELETE", fmt.Sprintf("/api/v1/account/emails/%d", emails[1].ID), map[string]string{"password": "correct horse battery"}).Code != 400 {
		t.Fatal("primary email removed")
	}
	login := &flows{t: t, srv: f.srv, router: f.router, cookies: map[string]*http.Cookie{}}
	if response := login.request("POST", "/auth/local/password", map[string]string{"email": " ALICE@example.com ", "password": "correct horse battery"}); response.Code != 200 {
		t.Fatal("secondary email cannot log in", response.Code)
	}
	if response := login.request("POST", "/auth/local/verify", map[string]string{"method": "totp", "code": deviceCode(t, secret, 0)}); response.Code != 200 {
		t.Fatal("secondary email login failed", response.Code)
	}
	if response := f.request("DELETE", fmt.Sprintf("/api/v1/account/emails/%d", emails[0].ID), map[string]string{"password": "correct horse battery"}); response.Code != 204 {
		t.Fatal(response.Code, response.Body.String())
	}
	if _, err := f.srv.store.UserByEmail(t.Context(), "alice@example.com"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("removed email still resolves", err)
	}
	login.cookies = map[string]*http.Cookie{}
	if login.request("POST", "/auth/local/password", map[string]string{"email": "alice@example.com", "password": "correct horse battery"}).Code != 401 {
		t.Fatal("removed email still logs in")
	}
}

func TestEmailBindingVerificationIsolationAndReplay(t *testing.T) {
	f, code := localFlows(t)
	registerLocal(t, f, code)
	user, err := f.srv.store.UserByEmail(t.Context(), "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	other := signedInUser(t, f, user)
	response := f.request("POST", "/api/v1/account/emails", map[string]string{"email": "bound@example.com", "password": "correct horse battery"})
	request := responseJSON(t, response)["request"]
	payload := map[string]any{"request": request, "code": *code}
	if _, err := f.srv.store.UserByEmail(t.Context(), "bound@example.com"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("unverified address was bound")
	}
	for _, client := range []*flows{other, f} {
		path := "/api/v1/account/emails/verify"
		if client == f {
			path = "/api/v1/account/email/verify"
		}
		if client.request("POST", path, payload).Code != 401 {
			t.Fatal("binding verification crossed session or purpose")
		}
	}
	if f.request("POST", "/api/v1/account/emails/verify", payload).Code != 204 {
		t.Fatal("valid binding failed")
	}
	if f.request("POST", "/api/v1/account/emails/verify", payload).Code != 401 {
		t.Fatal("binding replay accepted")
	}
	if f.request("POST", "/api/v1/account/emails", map[string]string{"email": "BOUND@example.com", "password": "correct horse battery"}).Code != 409 {
		t.Fatal("duplicate address accepted")
	}
}

func TestEmailBindingAttemptLimit(t *testing.T) {
	f, code := localFlows(t)
	registerLocal(t, f, code)
	response := f.request("POST", "/api/v1/account/emails", map[string]string{"email": "unverified@example.com", "password": "correct horse battery"})
	request := responseJSON(t, response)["request"]
	for range 5 {
		if f.request("POST", "/api/v1/account/emails/verify", map[string]any{"request": request, "code": "wrong"}).Code != 401 {
			t.Fatal("invalid verification accepted")
		}
	}
	if f.request("POST", "/api/v1/account/emails/verify", map[string]any{"request": request, "code": *code}).Code != 401 {
		t.Fatal("attempt limit bypassed")
	}
	if _, err := f.srv.store.UserByEmail(t.Context(), "unverified@example.com"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("unverified address was bound")
	}
}

func TestEmailBindingsCannotCrossAccounts(t *testing.T) {
	f, code := localFlows(t)
	registerLocal(t, f, code)
	bindAccountEmail(t, f, code, "bound@example.com")
	user, err := f.srv.store.UserByEmail(t.Context(), "bound@example.com")
	if err != nil {
		t.Fatal(err)
	}
	emails, err := f.srv.store.AccountEmails(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	stranger, err := f.srv.store.CreateUser(t.Context(), store.User{Email: "stranger@example.com", Name: "Stranger", Role: store.RoleUser, OAuthSub: "stranger"})
	if err != nil {
		t.Fatal(err)
	}
	other := signedInUser(t, f, stranger)
	if other.request("PUT", fmt.Sprintf("/api/v1/account/emails/%d/primary", emails[1].ID), map[string]string{}).Code != 404 {
		t.Fatal("primary email crossed accounts")
	}
	if other.request("DELETE", fmt.Sprintf("/api/v1/account/emails/%d", emails[1].ID), map[string]string{}).Code != 404 {
		t.Fatal("email removal crossed accounts")
	}
	if other.request("POST", "/api/v1/account/emails", map[string]string{"email": "bound@example.com"}).Code != 409 {
		t.Fatal("email binding crossed accounts")
	}
	anonymous := &flows{t: t, srv: f.srv, router: f.router, cookies: map[string]*http.Cookie{}}
	if anonymous.request("POST", "/auth/local/password", map[string]any{"email": "bound@example.com", "name": "Hijacker", "password": "correct horse battery", "register": true}).Code != 400 {
		t.Fatal("secondary address re-registered")
	}
}

func TestMCPEmailBindingIsBoundToAuthorization(t *testing.T) {
	f, code := localFlows(t)
	registerLocal(t, f, code)
	_, firstTokens := mcpTokens(t, f)
	_, otherTokens := mcpTokens(t, f)
	first, other := firstTokens["access_token"].(string), otherTokens["access_token"].(string)
	started := mcpCall(t, f, first, "start_email_binding", map[string]any{"email": "mcp-bound@example.com", "password": "correct horse battery"})
	if toolStatus(t, started) != 200 {
		t.Fatal(started)
	}
	body := started["structuredContent"].(map[string]any)["body"].(map[string]any)
	args := map[string]any{"request": body["request"], "code": *code}
	if toolStatus(t, mcpCall(t, f, other, "verify_email_binding", args)) != 401 {
		t.Fatal("binding crossed OAuth grants")
	}
	if f.request("POST", "/api/v1/account/emails/verify", args).Code != 401 {
		t.Fatal("binding crossed to browser session")
	}
	if toolStatus(t, mcpCall(t, f, first, "verify_email_binding", args)) != 204 {
		t.Fatal("same grant rejected")
	}
	if mcpRequest(t, f.router, first, "tools/list", nil).Code != 401 {
		t.Fatal("binding retained grant")
	}
	user, err := f.srv.store.UserByEmail(t.Context(), "mcp-bound@example.com")
	if err != nil || user.Email != "alice@example.com" {
		t.Fatal("binding changed primary", err)
	}
}
