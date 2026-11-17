package config

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSDKLoadsAllRuntimeConfiguration(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "ApiKey cc_test" || !strings.Contains(r.URL.Path, "/environments/staging/values") {
			t.Errorf("unexpected configcenter request %s", r.URL.Path)
		}
		items := []map[string]string{}
		for k, v := range map[string]string{"AC_ADDR": "127.0.0.1:9090", "AC_PUBLIC_URL": "https://auth.example", "AC_ENCRYPTION_KEY": key, "AC_RESEND_API_KEY": "test-mail-key", "AC_RESEND_FROM": "Auth <auth@example.com>", "AC_OAUTH_CLIENT_ID": "github-client", "AC_OAUTH_CLIENT_SECRET": "github-secret"} {
			items = append(items, map[string]string{"key": k, "value": v})
		}
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	}))
	defer remote.Close()
	values := map[string]string{"AC_CONFIGCENTER_URL": remote.URL, "AC_CONFIGCENTER_KEY": "cc_test", "AC_CONFIGCENTER_ENVIRONMENT": "staging", "AC_ENV_FILE": "missing-test-env", "AC_ADDR": "127.0.0.1:1"}
	cfg, err := Load(context.Background(), func(k string) (string, bool) { v, ok := values[k]; return v, ok })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:9090" || cfg.ResendKey != "test-mail-key" || cfg.OAuth.RedirectURL != "https://auth.example/oauth/callback" || len(cfg.EncryptionKey) != 32 {
		t.Fatal("runtime settings did not come from the SDK")
	}
}

func TestSDKFailureDoesNotFallBackToRuntimeEnvironment(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer remote.Close()
	for _, values := range []map[string]string{{}, {"AC_CONFIGCENTER_URL": remote.URL, "AC_CONFIGCENTER_KEY": "cc_test"}} {
		_, err := Load(context.Background(), func(k string) (string, bool) {
			if k == "AC_ENV_FILE" {
				return "missing-test-env", true
			}
			v, ok := values[k]
			return v, ok
		})
		if err == nil {
			t.Fatal("missing or unavailable remote must fail closed")
		}
	}
}

func TestConfigValidation(t *testing.T) {
	for _, values := range []map[string]string{
		{"AC_PUBLIC_URL": "https://auth.example/path"},
		{"AC_PUBLIC_URL": "http://untrusted.example"},
		{"AC_LOCAL_LOGIN": "true", "AC_ENCRYPTION_KEY": "bad"},
		{"AC_DB_DRIVER": "postgres"},
		{"AC_DB_DRIVER": "mysql", "AC_DB": "invalid"},
		{"AC_DB": "invalid"},
	} {
		_, err := fromSnapshot(func(key, fallback string) string {
			if v, ok := values[key]; ok {
				return v
			}
			if key == "AC_LOCAL_LOGIN" {
				return "false"
			}
			return fallback
		})
		if err == nil {
			t.Fatal("invalid config accepted")
		}
	}
}
