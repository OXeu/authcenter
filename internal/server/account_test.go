package server

import (
	"crypto/rand"
	"net/http"
	"testing"
	"time"

	"github.com/OXeu/authcenter/internal/credentials"
	"github.com/OXeu/authcenter/internal/store"
)

func signedInUser(t *testing.T, f *flows, user store.User) *flows {
	t.Helper()
	session := store.Session{ID: rand.Text(), UserID: user.ID, CSRFToken: rand.Text(), ExpiresAt: time.Now().Add(time.Hour)}
	if err := f.srv.store.CreateSession(t.Context(), session); err != nil {
		t.Fatal(err)
	}
	return &flows{t: t, srv: f.srv, router: f.router, csrf: session.CSRFToken, cookies: map[string]*http.Cookie{"ac_session": {Name: "ac_session", Value: session.ID}}}
}

func TestAccountPasswordAndSessionRevocation(t *testing.T) {
	f, code := localFlows(t)
	registerLocal(t, f, code)
	user, err := f.srv.store.UserByEmail(t.Context(), "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	other := signedInUser(t, f, user)
	if got := f.request("PUT", "/api/v1/account/password", map[string]string{"current_password": "wrong", "password": "a new secure password"}); got.Code != 401 {
		t.Fatal(got.Code)
	}
	if got := f.request("PUT", "/api/v1/account/password", map[string]string{"current_password": "correct horse battery", "password": "a new secure password"}); got.Code != 204 {
		t.Fatal(got.Code, got.Body.String())
	}
	if f.request("GET", "/api/v1/me", nil).Code != 200 || other.request("GET", "/api/v1/me", nil).Code != 401 {
		t.Fatal("session revocation failed")
	}
	user, err = f.srv.store.UserByID(t.Context(), user.ID)
	if err != nil || !credentials.CheckPassword(user.PasswordHash, "a new secure password") {
		t.Fatal("password not updated", err)
	}
	f.csrf = ""
	if f.request("PUT", "/api/v1/account/password", map[string]string{"password": "another secure password"}).Code != 403 {
		t.Fatal("CSRF accepted")
	}
}

func TestEmailChangeRequiresSessionBoundCode(t *testing.T) {
	f, code := localFlows(t)
	registerLocal(t, f, code)
	user, err := f.srv.store.UserByEmail(t.Context(), "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	other := signedInUser(t, f, user)
	response := f.request("POST", "/api/v1/account/email", map[string]string{"email": "new@example.com", "password": "correct horse battery"})
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	request := responseJSON(t, response)["request"].(string)
	payload := map[string]string{"request": request, "code": *code}
	if other.request("POST", "/api/v1/account/email/verify", payload).Code != 401 {
		t.Fatal("different session accepted")
	}
	if f.request("POST", "/api/v1/account/email/verify", map[string]string{"request": request, "code": "wrong"}).Code != 401 {
		t.Fatal("invalid code accepted")
	}
	f.cookies[loginChallengeCookie] = &http.Cookie{Name: loginChallengeCookie, Value: request}
	if f.request("POST", "/auth/local/verify", map[string]string{"method": "email", "code": *code}).Code != 401 {
		t.Fatal("email change accepted as login")
	}
	if got := f.request("POST", "/api/v1/account/email/verify", payload); got.Code != 204 {
		t.Fatal(got.Code, got.Body.String())
	}
	if f.request("POST", "/api/v1/account/email/verify", payload).Code != 401 {
		t.Fatal("email code replay accepted")
	}
	if other.request("GET", "/api/v1/me", nil).Code != 401 {
		t.Fatal("old session retained")
	}
	updated, err := f.srv.store.UserByID(t.Context(), user.ID)
	if err != nil || updated.Email != "new@example.com" || !updated.EmailVerified {
		t.Fatal("email not updated", err)
	}
}

func TestOAuthAccountCanSetPasswordAndManageAuthenticator(t *testing.T) {
	f, _ := localFlows(t)
	user, err := f.srv.store.CreateUser(t.Context(), store.User{Email: "oauth@example.com", Name: "OAuth", Role: store.RoleUser, OAuthIssuer: "https://idp.example", OAuthSub: "subject"})
	if err != nil {
		t.Fatal(err)
	}
	f = signedInUser(t, f, user)
	if got := f.request("PUT", "/api/v1/account/password", map[string]string{"password": "a new secure password"}); got.Code != 204 {
		t.Fatal(got.Code, got.Body.String())
	}
	response := f.request("POST", "/auth/local/totp", map[string]string{"password": "a new secure password"})
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	setup := responseJSON(t, response)
	secret := setup["secret"].(string)
	code, err := credentials.TOTP(secret, time.Now().Unix()/30-1)
	if err != nil {
		t.Fatal(err)
	}
	if f.request("POST", "/auth/local/totp/verify", map[string]any{"request": setup["request"], "code": code}).Code != 204 {
		t.Fatal("enrollment failed")
	}
	if f.request("DELETE", "/api/v1/account/totp", map[string]string{"password": "a new secure password", "code": code}).Code != 401 {
		t.Fatal("reused TOTP accepted")
	}
	code, err = credentials.TOTP(secret, time.Now().Unix()/30)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.request("DELETE", "/api/v1/account/totp", map[string]string{"password": "a new secure password", "code": code}); got.Code != 204 {
		t.Fatal(got.Code, got.Body.String())
	}
	settings := responseJSON(t, f.request("GET", "/api/v1/account/security", nil))
	if settings["totp_enabled"] != false || settings["password_enabled"] != true {
		t.Fatal(settings)
	}
}
