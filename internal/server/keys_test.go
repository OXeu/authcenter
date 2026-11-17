package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xeu/configcenter/internal/apikey"
	"github.com/xeu/configcenter/internal/store"
)

func keyRequest(t *testing.T, router http.Handler, token, method, path string, body any, want int) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(string(encoded)))
	req.Header.Set("Authorization", "ApiKey "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("%s %s: status %d, want %d", method, path, rec.Code, want)
	}
	return rec
}

func issueKey(t *testing.T, f *flows, path string, body map[string]any) (token string, keyID uint) {
	t.Helper()
	status, response := f.do(http.MethodPost, path, body)
	if status != http.StatusCreated {
		t.Fatalf("issue key: status %d", status)
	}
	token, ok := response["token"].(string)
	if !ok || !strings.HasPrefix(token, "cc_") {
		t.Fatal("missing issued credential")
	}
	key, ok := response["key"].(map[string]any)
	if !ok {
		t.Fatal("missing key metadata")
	}
	id, ok := key["id"].(float64)
	if !ok {
		t.Fatal("missing key ID")
	}
	return token, uint(id)
}

func TestGlobalKeyLifecycleAndProjectManagement(t *testing.T) {
	f := newFlows(t)
	token, id := issueKey(t, f, "/api/v1/keys", map[string]any{"name": "automation"})
	for _, project := range []string{"billing", "shipping"} {
		base := "/api/v1/projects/" + project
		keyRequest(t, f.router, token, http.MethodPost, "/api/v1/projects", map[string]any{"slug": project, "name": project}, http.StatusCreated)
		keyRequest(t, f.router, token, http.MethodPatch, base, map[string]any{"name": "Updated", "description": "Managed"}, http.StatusOK)
		keyRequest(t, f.router, token, http.MethodPost, base+"/environments", map[string]any{"slug": "staging", "name": "Staging"}, http.StatusCreated)
		keyRequest(t, f.router, token, http.MethodPut, base+"/environments/staging/items/db.pool", map[string]any{"value": "20"}, http.StatusOK)
		rec := keyRequest(t, f.router, token, http.MethodGet, base+"/environments/staging/items", nil, http.StatusOK)
		if !strings.Contains(rec.Body.String(), `"value":"20"`) {
			t.Fatal("global key could not read its configuration")
		}
		keyRequest(t, f.router, token, http.MethodGet, "/api/v1/agent/project?project="+project, nil, http.StatusOK)
		keyRequest(t, f.router, token, http.MethodGet, "/api/v1/agent/environments/staging/values?project="+project, nil, http.StatusOK)
		mcp := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{
			"name": "set_config", "arguments": map[string]any{"environment": "staging", "key": "mcp.value", "value": project},
		}}
		rec = keyRequest(t, f.router, token, http.MethodPost, "/mcp/"+project, mcp, http.StatusOK)
		if !strings.Contains(rec.Body.String(), `"isError":false`) {
			t.Fatal("global MCP write failed")
		}
		keyRequest(t, f.router, token, http.MethodDelete, base+"/environments/staging/items/db.pool", nil, http.StatusNoContent)
		keyRequest(t, f.router, token, http.MethodDelete, base+"/environments/staging", nil, http.StatusNoContent)
	}
	listed := keyRequest(t, f.router, token, http.MethodGet, "/api/v1/projects", nil, http.StatusOK)
	for _, slug := range []string{"billing", "shipping", "configcenter"} {
		if !strings.Contains(listed.Body.String(), `"slug":"`+slug+`"`) {
			t.Fatalf("global project list omits %s", slug)
		}
	}
	keyRequest(t, f.router, token, http.MethodGet, "/api/v1/agent/project", nil, http.StatusBadRequest)
	keyRequest(t, f.router, token, http.MethodDelete, "/api/v1/projects/shipping", nil, http.StatusNoContent)
	keyRequest(t, f.router, token, http.MethodGet, "/api/v1/projects", nil, http.StatusOK)
	for _, path := range []string{"/api/v1/keys", "/api/v1/users", "/api/v1/settings"} {
		keyRequest(t, f.router, token, http.MethodGet, path, nil, http.StatusUnauthorized)
	}
	status, keys := f.do(http.MethodGet, "/api/v1/keys", nil)
	if status != http.StatusOK {
		t.Fatal("cannot list global keys")
	}
	encoded, err := json.Marshal(keys)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), token) || strings.Contains(string(encoded), "digest") || !strings.Contains(string(encoded), `"scope":"global"`) {
		t.Fatal("key list must contain scope metadata without credentials")
	}
	path := fmt.Sprintf("/api/v1/keys/%d", id)
	status, rotated := f.do(http.MethodPost, path+"/rotate", nil)
	if status != http.StatusOK {
		t.Fatalf("rotate global key: status %d", status)
	}
	newToken, ok := rotated["token"].(string)
	if !ok || newToken == token {
		t.Fatal("rotation did not issue a new credential")
	}
	keyRequest(t, f.router, token, http.MethodGet, "/api/v1/projects", nil, http.StatusUnauthorized)
	keyRequest(t, f.router, newToken, http.MethodGet, "/api/v1/projects", nil, http.StatusOK)
	status, _ = f.do(http.MethodDelete, path, nil)
	if status != http.StatusNoContent {
		t.Fatalf("revoke global key: status %d", status)
	}
	keyRequest(t, f.router, newToken, http.MethodGet, "/api/v1/projects", nil, http.StatusUnauthorized)
}

func TestGlobalKeyScopeAndExpiry(t *testing.T) {
	f := newFlows(t)
	for _, slug := range []string{"alpha", "beta"} {
		status, _ := f.do(http.MethodPost, "/api/v1/projects", map[string]any{"slug": slug, "name": slug})
		if status != http.StatusCreated {
			t.Fatalf("create project: %d", status)
		}
	}
	global, globalID := issueKey(t, f, "/api/v1/keys", map[string]any{"name": "global"})
	for _, role := range []string{"viewer", "editor", "admin"} {
		token, keyID := issueKey(t, f, "/api/v1/projects/alpha/keys", map[string]any{"name": role, "role": role})
		keyRequest(t, f.router, token, http.MethodGet, "/api/v1/projects", nil, http.StatusForbidden)
		keyRequest(t, f.router, token, http.MethodPut, "/api/v1/projects/beta/environments/production/items/key", map[string]any{"value": "bad"}, http.StatusForbidden)
		keyRequest(t, f.router, token, http.MethodPost, "/mcp/beta", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}, http.StatusForbidden)
		keyRequest(t, f.router, token, http.MethodPost, "/mcp/alpha", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}, http.StatusOK)
		status, _ := f.do(http.MethodPost, fmt.Sprintf("/api/v1/keys/%d/rotate", keyID), nil)
		if status != http.StatusNotFound {
			t.Fatalf("global rotation reached project key: %d", status)
		}
	}
	keyRequest(t, f.router, global, http.MethodDelete, fmt.Sprintf("/api/v1/projects/alpha/keys/%d", globalID), nil, http.StatusNotFound)
	keyRequest(t, f.router, global, http.MethodPost, fmt.Sprintf("/api/v1/projects/alpha/keys/%d/rotate", globalID), nil, http.StatusNotFound)
	expired, _ := issueKey(t, f, "/api/v1/keys", map[string]any{"name": "expired", "expires_at": time.Now().Add(-time.Hour)})
	keyRequest(t, f.router, expired, http.MethodGet, "/api/v1/projects", nil, http.StatusUnauthorized)
	keyRequest(t, f.router, expired, http.MethodPost, "/mcp/alpha", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}, http.StatusUnauthorized)
	for _, body := range []map[string]any{{"name": "bad", "role": "viewer"}, {"name": " "}} {
		status, _ := f.do(http.MethodPost, "/api/v1/keys", body)
		if status != http.StatusBadRequest {
			t.Fatalf("invalid global key accepted: %d", status)
		}
	}
	savedCSRF := f.csrf
	f.csrf = ""
	status, _ := f.do(http.MethodPost, "/api/v1/keys", map[string]any{"name": "no-csrf"})
	if status != http.StatusForbidden {
		t.Fatalf("global issuance without CSRF: %d", status)
	}
	f.csrf = savedCSRF
	status, _ = f.do(http.MethodPost, "/api/v1/keys", map[string]any{"name": "valid-csrf"})
	if status != http.StatusCreated {
		t.Fatalf("valid CSRF rejected: %d", status)
	}
}

func TestGlobalKeysRequireAdministratorSession(t *testing.T) {
	s, router, _ := testServer(t, false)
	user, err := s.store.CreateUser(t.Context(), store.User{Email: "user@example.com", Name: "User", Role: store.RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.store.CreateSession(t.Context(), store.Session{ID: "regular-session", UserID: user.ID, CSRFToken: "csrf", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	f := &flows{t: t, router: router, csrf: "csrf", cookies: map[string]*http.Cookie{"cc_session": {Name: "cc_session", Value: "regular-session"}}}
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/keys"},
		{http.MethodGet, "/api/v1/keys"},
		{http.MethodPost, "/api/v1/keys"},
		{http.MethodPost, "/api/v1/keys/1/rotate"},
		{http.MethodDelete, "/api/v1/keys/1"},
	} {
		status, _ := f.do(route.method, route.path, map[string]any{"name": "forbidden"})
		if status != http.StatusForbidden {
			t.Fatalf("non-admin %s %s: status %d", route.method, route.path, status)
		}
	}
}

func TestGlobalKeyAttributionAndExplicitAuthentication(t *testing.T) {
	s, router, _ := testServer(t, false)
	admin, err := s.store.UserByEmail(t.Context(), "admin@localhost")
	if err != nil {
		t.Fatal(err)
	}
	token, digest, err := apikey.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.CreateAPIKey(t.Context(), store.APIKey{Name: "global", Digest: digest[:], Prefix: apikey.Display(token), Role: store.RoleAdmin, CreatedBy: admin.ID}); err != nil {
		t.Fatal(err)
	}
	keyRequest(t, router, token, http.MethodPost, "/api/v1/projects", map[string]any{"slug": "audit", "name": "Audit"}, http.StatusCreated)
	project, err := s.store.ProjectBySlug(t.Context(), "audit")
	if err != nil || project.OwnerID != admin.ID {
		t.Fatal("global-created project lost its owner")
	}
	env, err := s.store.EnvironmentBySlug(t.Context(), project.ID, "production")
	if err != nil {
		t.Fatal(err)
	}
	keyRequest(t, router, token, http.MethodPut, "/api/v1/projects/audit/environments/production/items/value", map[string]any{"value": "20"}, http.StatusOK)
	item, err := s.store.GetItem(t.Context(), project.ID, env.ID, "value")
	if err != nil || item.UpdatedBy != admin.ID {
		t.Fatal("global write lost its actor")
	}
	keyRequest(t, router, token, http.MethodPost, "/api/v1/projects/audit/keys", map[string]any{"name": "child", "role": "editor"}, http.StatusCreated)
	keys, err := s.store.ListAPIKeys(t.Context(), project.ID)
	if err != nil || len(keys) != 1 || keys[0].CreatedBy != admin.ID || keys[0].IsGlobal() {
		t.Fatal("global-issued project key has incorrect scope or creator")
	}
	if err := s.store.CreateSession(t.Context(), store.Session{ID: "admin-session", UserID: admin.ID, CSRFToken: "csrf", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		credential string
		status     int
	}{{token, http.StatusOK}, {"cc_invalid", http.StatusUnauthorized}} {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/audit/environments/production/items/value", strings.NewReader(`{"value":"30"}`))
		req.AddCookie(&http.Cookie{Name: "cc_session", Value: "admin-session"})
		req.Header.Set("Authorization", "ApiKey "+tc.credential)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("explicit API authentication with a browser cookie: status %d, want %d", rec.Code, tc.status)
		}
	}
}
