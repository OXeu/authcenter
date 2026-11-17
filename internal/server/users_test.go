package server

import (
	"errors"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/OXeu/authcenter/internal/store"
)

func userManagementFlows(t *testing.T) (administrator, account *flows, target store.User) {
	t.Helper()
	userFlow, code := localFlows(t)
	registerLocal(t, userFlow, code)
	user, err := userFlow.srv.store.UserByEmail(t.Context(), "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := userFlow.srv.store.UserByEmail(t.Context(), "admin@localhost")
	if err != nil {
		t.Fatal(err)
	}
	return signedInUser(t, userFlow, admin), userFlow, user
}

func TestUserBanRevokesSessionsTokensAndChallenges(t *testing.T) {
	admin, userFlow, user := userManagementFlows(t)
	client, secret := createClient(t, userFlow, false)
	code := issueCode(t, userFlow, client, "")
	tokens := exchange(t, userFlow, client, secret, code, "")
	response := admin.request("PUT", fmt.Sprintf("/api/v1/users/%d/ban", user.ID), map[string]any{"until": time.Now().Add(time.Hour)})
	if response.Code != 204 {
		t.Fatal(response.Code, response.Body.String())
	}
	if err := admin.srv.store.CreateLoginChallenge(t.Context(), store.LoginChallenge{ID: "late-challenge", UserID: user.ID, Email: user.Email, Purpose: "login", ExpiresAt: time.Now().Add(time.Minute)}); !errors.Is(err, store.ErrInvalidGrant) {
		t.Fatal("banned account created a new challenge", err)
	}
	if userFlow.request("GET", "/api/v1/me", nil).Code != 401 {
		t.Fatal("banned session accepted")
	}
	if userinfoStatus(admin.router, tokens["access_token"].(string)) != 401 {
		t.Fatal("banned access token accepted")
	}
	refresh := tokenRequest(admin.router, url.Values{"client_id": {client}, "client_secret": {secret}, "grant_type": {"refresh_token"}, "refresh_token": {tokens["refresh_token"].(string)}})
	if refresh.Code == 200 {
		t.Fatal("banned refresh accepted")
	}
	if userFlow.request("POST", "/auth/local/password", map[string]string{"email": user.Email, "password": "correct horse battery"}).Code != 401 {
		t.Fatal("banned password accepted")
	}
	if got := admin.request("DELETE", fmt.Sprintf("/api/v1/users/%d/ban", user.ID), nil); got.Code != 204 {
		t.Fatal(got.Code)
	}
	if userFlow.request("GET", "/api/v1/me", nil).Code != 401 {
		t.Fatal("unban restored revoked session")
	}
	if signedInUser(t, userFlow, user).request("GET", "/api/v1/me", nil).Code != 200 {
		t.Fatal("unbanned user cannot log in")
	}
}

func TestUserManagementPermissionsAndExpiry(t *testing.T) {
	admin, userFlow, user := userManagementFlows(t)
	path := fmt.Sprintf("/api/v1/users/%d", user.ID)
	if userFlow.request("DELETE", path, nil).Code != 403 {
		t.Fatal("nonadmin deletion accepted")
	}
	if userFlow.request("PUT", path+"/ban", map[string]any{"until": time.Now().Add(time.Hour)}).Code != 403 {
		t.Fatal("nonadmin ban accepted")
	}
	if admin.request("PUT", path+"/ban", map[string]any{"until": time.Now().Add(-time.Hour)}).Code != 400 {
		t.Fatal("past expiry accepted")
	}
	actor, err := admin.srv.store.UserByEmail(t.Context(), "admin@localhost")
	if err != nil {
		t.Fatal(err)
	}
	self := fmt.Sprintf("/api/v1/users/%d", actor.ID)
	if admin.request("DELETE", self, nil).Code != 403 || admin.request("PUT", self+"/ban", map[string]any{"until": time.Now().Add(time.Hour)}).Code != 403 {
		t.Fatal("self destruction accepted")
	}
	past := time.Now().Add(-time.Minute)
	if err := admin.srv.store.SetUserBan(t.Context(), actor.ID, user.ID, &past); err != nil {
		t.Fatal(err)
	}
	if signedInUser(t, userFlow, user).request("GET", "/api/v1/me", nil).Code != 200 {
		t.Fatal("expired ban blocked login")
	}
	admin.csrf = ""
	if admin.request("DELETE", path, nil).Code != 403 {
		t.Fatal("missing CSRF accepted")
	}
}

func TestDeletingUserRemovesOwnedApplicationsAndTokens(t *testing.T) {
	admin, userFlow, user := userManagementFlows(t)
	client, secret := createClient(t, userFlow, false)
	code := issueCode(t, userFlow, client, "")
	tokens := exchange(t, userFlow, client, secret, code, "")
	if got := admin.request("DELETE", fmt.Sprintf("/api/v1/users/%d", user.ID), nil); got.Code != 204 {
		t.Fatal(got.Code, got.Body.String())
	}
	if _, err := admin.srv.store.UserByID(t.Context(), user.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("user retained", err)
	}
	if userFlow.request("GET", "/api/v1/me", nil).Code != 401 || userinfoStatus(admin.router, tokens["access_token"].(string)) != 401 {
		t.Fatal("deleted account remains authenticated")
	}
	if admin.request("GET", "/api/v1/users", nil).Code != 200 {
		t.Fatal("administrator affected")
	}
	if got := admin.request("DELETE", fmt.Sprintf("/api/v1/users/%d", user.ID), nil); got.Code != 404 {
		t.Fatal("missing account deletion", got.Code)
	}
}
