package server

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/OXeu/authcenter/internal/oidc"
	"github.com/OXeu/center-common/oauth"
)

func signingConfiguration(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

func enableOpenID(t *testing.T, f *flows) {
	t.Helper()
	signer, err := oidc.NewSigner(signingConfiguration(t))
	if err != nil {
		t.Fatal(err)
	}
	f.srv.cfg.OIDC = signer
}

func openIDQuery(id, scope, nonce string) url.Values {
	query := url.Values{"client_id": {id}, "response_type": {"code"}, "redirect_uri": {"http://localhost:3000/callback"}, "scope": {scope}, "state": {"original-state"}}
	if nonce != "" {
		query.Set("nonce", nonce)
	}
	return query
}

func approveOpenID(t *testing.T, f *flows, query url.Values) string {
	t.Helper()
	response := f.request("GET", "/oauth/authorize?"+query.Encode(), nil)
	if response.Code != http.StatusFound || !strings.HasPrefix(response.Header().Get("Location"), "/oauth/authorize?request=") {
		t.Fatal("authorization failed", response.Code, response.Header().Get("Location"))
	}
	if response := f.request("GET", response.Header().Get("Location"), nil); response.Code != http.StatusOK {
		t.Fatal("consent page failed", response.Code)
	}
	response = f.request("POST", "/oauth/authorize", map[string]any{"request": f.cookies[authorizationCookie].Value, "approve": true})
	if response.Code != http.StatusOK {
		t.Fatal("consent failed", response.Code, response.Body.String())
	}
	redirect, err := url.Parse(responseJSON(t, response)["redirect"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if redirect.Query().Get("state") != query.Get("state") || redirect.Query().Get("code") == "" {
		t.Fatal("authorization response lost code or state")
	}
	return redirect.Query().Get("code")
}

func verifiedIDClaims(t *testing.T, f *flows, raw string) map[string]any {
	t.Helper()
	response := f.request("GET", "/oauth/jwks", nil)
	if response.Code != http.StatusOK || response.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("public JWKS unavailable")
	}
	var jwks struct{ Keys []oidc.JWK }
	if err := json.Unmarshal(response.Body.Bytes(), &jwks); err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		t.Fatal("missing signed ID Token")
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]string
	json.Unmarshal(header, &fields)
	if fields["alg"] != "RS256" || fields["kid"] == "" {
		t.Fatal("invalid signing header")
	}
	var public *rsa.PublicKey
	for _, key := range jwks.Keys {
		if key.Kid == fields["kid"] {
			n, _ := base64.RawURLEncoding.DecodeString(key.N)
			e, _ := base64.RawURLEncoding.DecodeString(key.E)
			public = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
		}
	}
	if public == nil {
		t.Fatal("ID Token key not published")
	}
	signature, _ := base64.RawURLEncoding.DecodeString(parts[2])
	hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(public, crypto.SHA256, hash[:], signature); err != nil {
		t.Fatal("invalid ID Token signature", err)
	}
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	return claims
}

func TestOpenIDClaimsAndRefresh(t *testing.T) {
	for _, scope := range []string{"openid", "openid profile", "openid email", "openid profile email offline_access"} {
		t.Run(scope, func(t *testing.T) {
			f := newFlows(t)
			enableOpenID(t, f)
			id, secret := createClient(t, f, false)
			code := approveOpenID(t, f, openIDQuery(id, scope, "nonce+with/special=characters"))
			tokens := exchange(t, f, id, secret, code, "")
			claims := verifiedIDClaims(t, f, tokens["id_token"].(string))
			if claims["iss"] != f.srv.cfg.PublicURL || claims["aud"] != id || claims["sub"] == "" || claims["nonce"] != "nonce+with/special=characters" {
				t.Fatal("incorrect identity bindings", claims)
			}
			issued, expires := int64(claims["iat"].(float64)), int64(claims["exp"].(float64))
			if expires-issued != 300 || time.Now().Unix()-issued > 5 || issued > time.Now().Unix() {
				t.Fatal("incorrect ID Token lifetime")
			}
			access := tokens["access_token"].(string)
			hash := sha256.Sum256([]byte(access))
			if claims["at_hash"] != base64.RawURLEncoding.EncodeToString(hash[:16]) {
				t.Fatal("ID Token not bound to access token")
			}
			for _, method := range []string{"GET", "POST"} {
				request := httptest.NewRequest(method, "/oauth/userinfo", http.NoBody)
				request.Header.Set("Authorization", "Bearer "+access)
				response := httptest.NewRecorder()
				f.router.ServeHTTP(response, request)
				info := responseJSON(t, response)
				if response.Code != http.StatusOK || info["sub"] != claims["sub"] {
					t.Fatal("userinfo subject mismatch")
				}
				for _, field := range []string{"name", "picture", "email", "email_verified"} {
					if !reflect.DeepEqual(claims[field], info[field]) {
						t.Fatal("userinfo claims differ from ID Token", field)
					}
				}
			}
			for field, permitted := range map[string]bool{"name": strings.Contains(scope, "profile"), "email": strings.Contains(scope, "email"), "email_verified": strings.Contains(scope, "email")} {
				if _, present := claims[field]; present != permitted {
					t.Fatal("claims exceeded consented scopes", field)
				}
			}
			if strings.Contains(scope, "offline_access") {
				response := tokenRequest(f.router, url.Values{"client_id": {id}, "client_secret": {secret}, "grant_type": {"refresh_token"}, "refresh_token": {tokens["refresh_token"].(string)}})
				refreshed := responseJSON(t, response)
				if response.Code != http.StatusOK || refreshed["scope"] != scope || refreshed["id_token"] != nil || refreshed["refresh_token"] == tokens["refresh_token"] {
					t.Fatal("invalid OpenID refresh response", response.Code)
				}
			}
		})
	}
}

func TestOpenIDWorksWithSharedClient(t *testing.T) {
	f := newFlows(t)
	enableOpenID(t, f)
	server := httptest.NewServer(f.router)
	t.Cleanup(server.Close)
	f.srv.cfg.PublicURL = server.URL
	id, secret := createClient(t, f, true)
	client := oauth.New(oauth.Config{IssuerURL: server.URL, ClientID: id, ClientSecret: secret, RedirectURL: "http://localhost:3000/callback", Scopes: []string{"openid", "profile", "email"}}, server.Client())
	redirect, blob, err := client.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	location, err := url.Parse(redirect)
	if err != nil {
		t.Fatal(err)
	}
	code := approveOpenID(t, f, location.Query())
	claims, err := client.Finish(t.Context(), code, location.Query().Get("state"), blob)
	if err != nil {
		t.Fatal("shared client rejected AuthCenter ID Token", err)
	}
	if claims.Issuer != server.URL || claims.Sub == "" || claims.Email != "dana@example.com" || !claims.EmailVerified || claims.Name != "Dana" {
		t.Fatal("unexpected verified identity", claims)
	}
}

func TestOpenIDDiscoveryAndDisabledConfiguration(t *testing.T) {
	f := newFlows(t)
	id, _ := createClient(t, f, false)
	response := f.request("GET", "/oauth/authorize?"+openIDQuery(id, "openid", "nonce").Encode(), nil)
	if !strings.Contains(response.Header().Get("Location"), "error=invalid_scope") {
		t.Fatal("OpenID accepted without signing configuration")
	}
	enableOpenID(t, f)
	for _, path := range []string{"/.well-known/openid-configuration", "/.well-known/oauth-authorization-server"} {
		response := f.request("GET", path, nil)
		metadata := responseJSON(t, response)
		if metadata["jwks_uri"] != f.srv.cfg.PublicURL+"/oauth/jwks" || !reflect.DeepEqual(metadata["id_token_signing_alg_values_supported"], []any{"RS256"}) ||
			!reflect.DeepEqual(metadata["subject_types_supported"], []any{"public"}) || !slices.Contains(metadata["scopes_supported"].([]any), any("openid")) {
			t.Fatal("incomplete OpenID metadata", metadata)
		}
		if path == "/.well-known/openid-configuration" && metadata["registration_endpoint"] != nil {
			t.Fatal("MCP registration advertised as OpenID registration")
		}
	}
}
