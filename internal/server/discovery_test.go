package server

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestAuthorizationMetadataDiscovery(t *testing.T) {
	f := newFlows(t)
	var previous map[string]any
	for _, path := range []string{"/.well-known/oauth-authorization-server", "/.well-known/openid-configuration"} {
		request := httptest.NewRequest(http.MethodGet, path, http.NoBody)
		request.Header.Set("Origin", "https://client.example")
		response := httptest.NewRecorder()
		f.router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Header().Get("Access-Control-Allow-Origin") != "*" || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("anonymous discovery failed: %s %d", path, response.Code)
		}
		metadata := responseJSON(t, response)
		if previous != nil && !reflect.DeepEqual(metadata, previous) {
			t.Fatal("discovery endpoints advertise different capabilities")
		}
		previous = metadata
		for field, suffix := range map[string]string{
			"issuer": "", "authorization_endpoint": "/oauth/authorize", "token_endpoint": "/oauth/token",
			"registration_endpoint": "/oauth/register", "revocation_endpoint": "/oauth/revoke", "userinfo_endpoint": "/oauth/userinfo",
		} {
			if metadata[field] != f.srv.cfg.PublicURL+suffix {
				t.Fatalf("incorrect %s: %v", field, metadata[field])
			}
		}
		for field, expected := range map[string][]any{
			"response_types_supported": {"code"}, "response_modes_supported": {"query"},
			"grant_types_supported": {"authorization_code", "refresh_token"}, "code_challenge_methods_supported": {"S256"},
			"token_endpoint_auth_methods_supported":      {"client_secret_basic", "client_secret_post", "none"},
			"revocation_endpoint_auth_methods_supported": {"client_secret_basic", "client_secret_post", "none"},
			"scopes_supported":                           {"profile", "email", "offline_access", "mcp"},
		} {
			if !reflect.DeepEqual(metadata[field], expected) {
				t.Fatalf("incorrect %s: %v", field, metadata[field])
			}
		}
		for _, field := range []string{"jwks_uri", "id_token_signing_alg_values_supported"} {
			if _, ok := metadata[field]; ok {
				t.Fatalf("discovery advertises unsupported %s", field)
			}
		}
		request = httptest.NewRequest(http.MethodOptions, path, http.NoBody)
		request.Header.Set("Origin", "https://client.example")
		request.Header.Set("Access-Control-Request-Method", "GET")
		response = httptest.NewRecorder()
		f.router.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") != "*" || response.Header().Get("Access-Control-Allow-Credentials") != "" {
			t.Fatalf("discovery preflight failed: %s %d", path, response.Code)
		}
	}
}
