package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIssuerPathsFollowDiscoverySpec(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"https://github.com":                "https://github.com/.well-known/openid-configuration",
		"https://idp.example.com/tenant/1/": "https://idp.example.com/.well-known/openid-configuration/tenant/1",
	}
	for issuer, want := range cases {
		origin := strings.TrimRight(issuer, "/")
		got := discoveryURL(issuer)
		if got != want {
			t.Fatalf("%s -> %s, want %s (origin %s)", issuer, got, want, origin)
		}
	}
}

func TestGitHubDiscoveryFallbackAndUserinfo(t *testing.T) {
	t.Parallel()
	var sawBearer, sawFormSecret bool
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sawBearer = r.Header.Get("Authorization") == "Bearer secret"
		sawFormSecret = r.PostForm.Get("client_secret") != ""
		if r.PostForm.Get("code_verifier") != "" {
			http.Error(w, "pkce sent", http.StatusBadRequest)
			return
		}
		if r.PostForm.Get("code") != "code" {
			http.Error(w, "bad code", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "gho_token", "token_type": "bearer"})
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer gho_token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 42, "login": "octo", "name": "Octo Cat"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// The fallback only applies to github.com, so the test drives the pieces
	// that are host specific and points userinfo at the fake server.
	c := New(Config{IssuerURL: "https://github.com", ClientID: "id", ClientSecret: "secret", RedirectURL: "http://cc/cb", Scopes: []string{"openid", "groups", "read:user"}}, srv.Client())
	meta := githubDiscovery()
	meta.TokenEndpoint = srv.URL + "/login/oauth/access_token"
	meta.UserinfoEndpoint = srv.URL + "/user"
	c.remember(meta)

	if got := c.scopes(); len(got) != 1 || got[0] != "read:user" {
		t.Fatalf("scopes = %v; GitHub must not receive openid or groups", got)
	}
	accessToken, err := c.exchangeCode(context.Background(), meta, "code", "verifier")
	if err != nil || accessToken != "gho_token" {
		t.Fatalf("token exchange: %v (%q)", err, accessToken)
	}

	claims, err := c.userinfo(context.Background(), "gho_token")
	if err != nil {
		t.Fatalf("userinfo: %v", err)
	}
	if claims.Sub != "42" || claims.Name != "Octo Cat" {
		t.Fatalf("claims = %+v", claims)
	}
	if !sawBearer || sawFormSecret {
		t.Fatalf("token request: bearer=%v form-secret=%v", sawBearer, sawFormSecret)
	}
}

func TestSameIssuerAcceptsOriginAfterPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		discovered, configured string
		want                   bool
	}{
		{"https://github.com", "https://github.com/login/oauth", true},
		{"https://idp.example.com", "https://idp.example.com/", true},
		{"https://idp.example.com", "https://idp.example.com/tenant", true},
		{"https://evil.example.com", "https://github.com/login/oauth", false},
	}
	for _, c := range cases {
		if got := sameIssuer(c.discovered, c.configured); got != c.want {
			t.Fatalf("sameIssuer(%q, %q) = %v, want %v", c.discovered, c.configured, got, c.want)
		}
	}
}
