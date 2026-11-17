package server

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/OXeu/authcenter/internal/config"
	"github.com/OXeu/authcenter/internal/store"
	"github.com/OXeu/configcenter/sdk/configcenter"
)

func settingsFlow(t *testing.T) (flow *flows, client *configcenter.Client) {
	t.Helper()
	srv, router, _ := testServer(t, false)
	client, err := configcenter.New(configcenter.Config{DotenvFiles: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.PutMany(t.Context(), map[string]string{"AC_LOCAL_LOGIN": "false", "AC_ADDR": srv.cfg.Addr, "AC_DB": srv.cfg.DatabaseDSN, "AC_PUBLIC_URL": srv.cfg.PublicURL}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.NewManager(client).Read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	srv.cfg = cfg
	admin, err := srv.store.UserByEmail(t.Context(), "admin@localhost")
	if err != nil {
		t.Fatal(err)
	}
	return signedInUser(t, &flows{t: t, srv: srv, router: router}, admin), client
}

func TestSettingsPermissionsRedactionAndRefresh(t *testing.T) {
	f, client := settingsFlow(t)
	if err := client.PutMany(t.Context(), map[string]string{"AC_RESEND_API_KEY": "private-key"}); err != nil {
		t.Fatal(err)
	}
	response := f.request("GET", "/api/v1/settings", nil)
	if response.Code != 200 || strings.Contains(response.Body.String(), "private-key") {
		t.Fatal("secret leaked", response.Code)
	}
	if f.request("GET", "/settings", nil).Code != 200 {
		t.Fatal("settings page unavailable")
	}
	user, err := f.srv.store.CreateUser(t.Context(), store.User{Email: "user@example.com", Name: "User", Role: store.RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	other := signedInUser(t, f, user)
	for _, path := range []string{"/settings", "/api/v1/settings"} {
		if other.request("GET", path, nil).Code != 403 {
			t.Fatal("nonadmin settings access", path)
		}
	}
	if other.request("POST", "/api/v1/settings/refresh", nil).Code != 403 {
		t.Fatal("nonadmin refreshed config")
	}
	if got := f.request("PUT", "/api/v1/settings", map[string]any{"values": map[string]string{"AC_ALLOW_REGISTRATION": "false", "AC_LANGUAGES": "zh-CN"}}); got.Code != 204 {
		t.Fatal(got.Code, got.Body.String())
	}
	if !f.srv.runtime.current.Load().cfg.AllowRegistration {
		t.Fatal("save applied without refresh")
	}
	if got := f.request("POST", "/api/v1/settings/refresh", nil); got.Code != 200 {
		t.Fatal(got.Code, got.Body.String())
	}
	if f.srv.runtime.current.Load().cfg.AllowRegistration {
		t.Fatal("refresh did not apply")
	}
	response = f.request("GET", "/login", nil)
	if response.Code != 302 {
		t.Fatal("existing session was lost")
	}
	public := httptest.NewRecorder()
	f.router.ServeHTTP(public, httptest.NewRequest("GET", "/login", http.NoBody))
	if !strings.Contains(public.Body.String(), `"lang":"zh-CN"`) || !strings.Contains(public.Body.String(), `"registration":false`) {
		t.Fatal("router retained old snapshot")
	}
	f.csrf = ""
	if f.request("POST", "/api/v1/settings/refresh", nil).Code != 403 {
		t.Fatal("CSRF bypass")
	}
}

func TestSettingsPreserveStartupResourcesAndRejectInvalidRefresh(t *testing.T) {
	f, client := settingsFlow(t)
	original := f.srv.cfg.DatabaseDSN
	if err := client.PutMany(t.Context(), map[string]string{"AC_ADDR": "127.0.0.1:9090", "AC_DB": "file:other.db", "AC_LOCAL_LOGIN": "true", "AC_ENCRYPTION_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32))}); err != nil {
		t.Fatal(err)
	}
	response := f.request("POST", "/api/v1/settings/refresh", nil)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "AC_ENCRYPTION_KEY") {
		t.Fatal(response.Code, response.Body.String())
	}
	active := f.srv.runtime.current.Load()
	if active.cfg.DatabaseDSN != original || active.cfg.LocalLogin || len(active.cfg.EncryptionKey) != 0 {
		t.Fatal("startup resource replaced")
	}
	if err := client.PutMany(t.Context(), map[string]string{"AC_LANGUAGES": "unknown"}); err != nil {
		t.Fatal(err)
	}
	if f.request("POST", "/api/v1/settings/refresh", nil).Code != 400 {
		t.Fatal("invalid refresh accepted")
	}
	if f.srv.runtime.current.Load() != active {
		t.Fatal("failed refresh changed active snapshot")
	}
}

func TestConfigurationRefreshWithConcurrentRequests(t *testing.T) {
	f, _ := settingsFlow(t)
	var group sync.WaitGroup
	for range 4 {
		group.Go(func() {
			for range 10 {
				response := httptest.NewRecorder()
				f.router.ServeHTTP(response, httptest.NewRequest("GET", "/login", http.NoBody))
				if response.Code != 200 {
					t.Errorf("concurrent page: %d", response.Code)
				}
			}
		})
	}
	for range 3 {
		if f.request("POST", "/api/v1/settings/refresh", nil).Code != 200 {
			t.Error("refresh failed")
		}
	}
	group.Wait()
}
