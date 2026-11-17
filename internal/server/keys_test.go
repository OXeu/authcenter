package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OXeu/authcenter/internal/apikey"
	"github.com/OXeu/authcenter/internal/store"
)

func keyRequest(t *testing.T, f *flows, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	for _, cookie := range f.cookies {
		req.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, req)
	return response
}

func createKey(t *testing.T, f *flows, scopes []string) (id, token string) {
	t.Helper()
	response := f.request("POST", "/api/v1/keys", map[string]any{"name": "Automation", "scopes": scopes})
	if response.Code != 201 {
		t.Fatal(response.Code, response.Body.String())
	}
	body := responseJSON(t, response)
	return fmt.Sprint(body["key"].(map[string]any)["id"]), body["token"].(string)
}

func TestAPIKeyLifecycleAndScopeEnforcement(t *testing.T) {
	f := newFlows(t)
	id, token := createKey(t, f, []string{"clients:read"})
	if !strings.HasPrefix(token, "ac_") {
		t.Fatal("key prefix")
	}
	if keyRequest(t, f, token, "GET", "/api/v1/clients", nil).Code != 200 {
		t.Fatal("read permission rejected")
	}
	for _, target := range []struct{ method, path string }{{"GET", "/api/v1/me"}, {"GET", "/api/v1/users"}, {"POST", "/api/v1/clients"}, {"GET", "/api/v1/keys"}} {
		if got := keyRequest(t, f, token, target.method, target.path, nil); got.Code != 403 {
			t.Fatal("key escaped scope through browser session", target, got.Code)
		}
	}
	listed := f.request("GET", "/api/v1/keys", nil)
	if strings.Contains(listed.Body.String(), token) || strings.Contains(listed.Body.String(), "digest") {
		t.Fatal("key leaked")
	}
	if listed.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("credential metadata cached")
	}
	key := responseJSON(t, listed)["keys"].([]any)[0].(map[string]any)
	if key["last_used_at"] == nil {
		t.Fatal("last use not recorded")
	}
	if got := f.request("PATCH", "/api/v1/keys/"+id, map[string]any{"name": "Disabled", "scopes": []string{}}); got.Code != 200 {
		t.Fatal(got.Body.String())
	}
	if keyRequest(t, f, token, "GET", "/api/v1/clients", nil).Code != 403 {
		t.Fatal("empty scopes grant access")
	}
	if got := f.request("PATCH", "/api/v1/keys/"+id, map[string]any{"name": "Enabled", "scopes": []string{"clients:read"}}); got.Code != 200 {
		t.Fatal(got.Body.String())
	}
	rotated := responseJSON(t, f.request("POST", "/api/v1/keys/"+id+"/rotate", nil))["token"].(string)
	if keyRequest(t, f, token, "GET", "/api/v1/clients", nil).Code != 401 || keyRequest(t, f, rotated, "GET", "/api/v1/clients", nil).Code != 200 {
		t.Fatal("rotation ineffective")
	}
	if f.request("DELETE", "/api/v1/keys/"+id, nil).Code != 204 || keyRequest(t, f, rotated, "GET", "/api/v1/clients", nil).Code != 401 {
		t.Fatal("deletion ineffective")
	}
	f.csrf = ""
	if f.request("POST", "/api/v1/keys", map[string]any{"name": "CSRF", "scopes": []string{}}).Code != 403 {
		t.Fatal("browser CSRF bypass")
	}
	if keyRequest(t, f, "invalid", "GET", "/api/v1/clients", nil).Code != 401 {
		t.Fatal("invalid key fell back to session")
	}
}

func TestAPIKeyOwnershipDelegationAndExpiry(t *testing.T) {
	f := newFlows(t)
	user, err := f.srv.store.CreateUser(t.Context(), store.User{Email: "key-owner@example.com", Name: "Owner", Role: store.RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	owner := signedInUser(t, f, user)
	adminKey, _ := createKey(t, f, []string{"users:read"})
	if owner.request("DELETE", "/api/v1/keys/"+adminKey, nil).Code != 403 {
		t.Fatal("cross-owner key access")
	}
	if owner.request("POST", "/api/v1/keys", map[string]any{"name": "Escalation", "scopes": []string{"users:read"}}).Code != 403 {
		t.Fatal("user granted admin scope")
	}
	managerID, manager := createKey(t, owner, []string{"keys:read", "keys:write", "clients:read"})
	for _, scopes := range [][]string{{"clients:write"}, {"users:read"}, {"unknown"}, {"clients:read", "clients:read"}} {
		if response := keyRequest(t, owner, manager, "POST", "/api/v1/keys", map[string]any{"name": "Too broad", "scopes": scopes}); response.Code < 400 {
			t.Fatal("delegation expanded scope", scopes)
		}
	}
	child := keyRequest(t, owner, manager, "POST", "/api/v1/keys", map[string]any{"name": "Reader", "scopes": []string{"clients:read"}})
	if child.Code != 201 {
		t.Fatal(child.Code, child.Body.String())
	}
	childToken := responseJSON(t, child)["token"].(string)
	if keyRequest(t, owner, childToken, "GET", "/api/v1/clients", nil).Code != 200 {
		t.Fatal("delegated key invalid")
	}
	broad, _ := createKey(t, owner, []string{"clients:write"})
	for _, target := range []struct{ method, suffix string }{{"POST", "/rotate"}, {"PATCH", ""}, {"DELETE", ""}} {
		if keyRequest(t, owner, manager, target.method, "/api/v1/keys/"+broad+target.suffix, map[string]any{"name": "Escalation", "scopes": []string{"clients:read"}}).Code != 403 {
			t.Fatal("broader key manageable")
		}
	}
	if got := owner.request("POST", "/api/v1/keys", map[string]any{"name": "Expired", "scopes": []string{}, "expires_at": time.Now().Add(-time.Hour)}); got.Code != 400 {
		t.Fatal("past expiry accepted")
	}
	expiry := time.Now().Add(time.Hour)
	expiring := responseJSON(t, owner.request("POST", "/api/v1/keys", map[string]any{"name": "Temporary", "scopes": []string{"keys:write", "clients:read"}, "expires_at": expiry}))["token"].(string)
	if keyRequest(t, owner, expiring, "POST", "/api/v1/keys", map[string]any{"name": "Forever", "scopes": []string{"clients:read"}}).Code != 403 {
		t.Fatal("delegation escaped expiry")
	}
	childID := fmt.Sprint(responseJSON(t, child)["key"].(map[string]any)["id"])
	if keyRequest(t, owner, expiring, "POST", "/api/v1/keys/"+childID+"/rotate", nil).Code != 403 {
		t.Fatal("rotation escaped expiry")
	}
	past := time.Now().Add(-time.Second)
	expiredToken, hash, err := apikey.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.store.CreateAPIKey(t.Context(), store.APIKey{Name: "Expired", CreatedBy: user.ID, Scopes: []store.Permission{store.ClientsRead}, Digest: hash[:], ExpiresAt: &past}); err != nil {
		t.Fatal(err)
	}
	if keyRequest(t, owner, expiredToken, "GET", "/api/v1/clients", nil).Code != 401 {
		t.Fatal("expired key accepted")
	}
	stored, err := f.srv.store.APIKeyByDigest(t.Context(), apikey.Hash(manager))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.srv.store.UpdateAPIKey(t.Context(), stored); err != nil {
		t.Fatal(err)
	}
	if err := f.srv.store.DeleteAPIKey(t.Context(), stored); !errors.Is(err, store.ErrKeyChanged) {
		t.Fatal("stale delete accepted", err)
	}
	if err := f.srv.store.RotateAPIKey(t.Context(), stored, hash, "stale"); !errors.Is(err, store.ErrKeyChanged) {
		t.Fatal("stale rotate accepted", err)
	}
	if err := f.srv.store.UpdateAPIKey(t.Context(), stored); !errors.Is(err, store.ErrKeyChanged) {
		t.Fatal("stale update accepted", err)
	}
	if owner.request("DELETE", "/api/v1/keys/"+managerID, nil).Code != 204 {
		t.Fatal("owner cannot revoke manager")
	}
}

func TestAPIKeyLiveRolesAndCredentialInvalidation(t *testing.T) {
	f := newFlows(t)
	_, key := createKey(t, f, []string{"users:read"})
	stored, err := f.srv.store.APIKeyByDigest(t.Context(), apikey.Hash(key))
	if err != nil {
		t.Fatal(err)
	}
	if keyRequest(t, f, key, "GET", "/api/v1/users", nil).Code != 200 {
		t.Fatal("admin key denied")
	}
	if err := f.srv.store.SetUserRole(t.Context(), stored.CreatedBy, store.RoleUser); err != nil {
		t.Fatal(err)
	}
	if keyRequest(t, f, key, "GET", "/api/v1/users", nil).Code != 403 {
		t.Fatal("demoted key remains administrator")
	}
	if err := f.srv.store.SetUserRole(t.Context(), stored.CreatedBy, store.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	admin, err := f.srv.store.UserByEmail(t.Context(), "admin@localhost")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.srv.store.SetUserBan(t.Context(), admin.ID, stored.CreatedBy, new(time.Now().Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	if keyRequest(t, f, key, "GET", "/api/v1/users", nil).Code != 401 {
		t.Fatal("banned key remains valid")
	}
	if _, err := f.srv.store.APIKeyByDigest(t.Context(), apikey.Hash(key)); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("ban did not revoke key")
	}
}
