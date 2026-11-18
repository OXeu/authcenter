package server

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/OXeu/authcenter/internal/credentials"
	"github.com/OXeu/authcenter/internal/store"
)

func deviceCode(t *testing.T, secret string, offset int64) string {
	t.Helper()
	code, err := credentials.TOTP(secret, time.Now().Unix()/30+offset)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func enrollDevice(t *testing.T, f *flows, name string) string {
	t.Helper()
	response := f.request("POST", "/auth/local/totp", map[string]string{"name": name, "password": "correct horse battery"})
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	setup := responseJSON(t, response)
	secret := setup["secret"].(string)
	response = f.request("POST", "/auth/local/totp/verify", map[string]any{"request": setup["request"], "code": deviceCode(t, secret, -1)})
	if response.Code != 204 {
		t.Fatal(response.Code, response.Body.String())
	}
	return secret
}

func TestMultipleTOTPDevicesLoginAndRemoval(t *testing.T) {
	f, code := localFlows(t)
	registerLocal(t, f, code)
	first := enrollDevice(t, f, "Phone")
	second := enrollDevice(t, f, "Tablet")
	user, err := f.srv.store.UserByEmail(t.Context(), "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	devices, err := f.srv.store.TOTPDevices(t.Context(), user.ID)
	if err != nil || len(devices) != 2 {
		t.Fatal("devices missing", err)
	}
	settings := f.request("GET", "/api/v1/account/security", nil)
	if strings.Contains(settings.Body.String(), first) || strings.Contains(settings.Body.String(), "secret") || !strings.Contains(settings.Body.String(), "Tablet") {
		t.Fatal("unsafe device metadata")
	}
	for _, secret := range []string{first, second} {
		login := &flows{t: t, srv: f.srv, router: f.router, cookies: map[string]*http.Cookie{}}
		for attempt := range 2 {
			login.cookies = map[string]*http.Cookie{}
			if response := login.request("POST", "/auth/local/password", map[string]string{"email": user.Email, "password": "correct horse battery"}); response.Code != 200 {
				t.Fatal(response.Code, response.Body.String())
			}
			response := login.request("POST", "/auth/local/verify", map[string]string{"method": "totp", "code": deviceCode(t, secret, 0)})
			want := 200
			if attempt == 1 {
				want = 401
			}
			if response.Code != want {
				t.Fatal("device login/replay", response.Code, want)
			}
		}
	}
	f.srv.cfg.ResendKey = ""
	remove := map[string]any{"id": devices[0].ID, "password": "correct horse battery", "code": deviceCode(t, second, 1)}
	if f.request("DELETE", "/api/v1/account/totp", remove).Code != 401 {
		t.Fatal("another device code accepted")
	}
	remove["code"] = deviceCode(t, first, 1)
	if response := f.request("DELETE", "/api/v1/account/totp", remove); response.Code != 204 {
		t.Fatal(response.Code, response.Body.String())
	}
	remove["id"], remove["code"] = devices[1].ID, deviceCode(t, second, 1)
	if f.request("DELETE", "/api/v1/account/totp", remove).Code != 400 {
		t.Fatal("last second factor removed")
	}
	devices, err = f.srv.store.TOTPDevices(t.Context(), user.ID)
	if err != nil || len(devices) != 1 || devices[0].Name != "Tablet" {
		t.Fatal("wrong device removed", err)
	}
}

func TestTOTPEnrollmentBoundToSessionAndPassword(t *testing.T) {
	f, code := localFlows(t)
	registerLocal(t, f, code)
	user, err := f.srv.store.UserByEmail(t.Context(), "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	other := signedInUser(t, f, user)
	response := f.request("POST", "/auth/local/totp", map[string]string{"password": "correct horse battery", "name": "Phone"})
	setup := responseJSON(t, response)
	payload := map[string]any{"request": setup["request"], "code": deviceCode(t, setup["secret"].(string), -1)}
	if other.request("POST", "/auth/local/totp/verify", payload).Code != 401 {
		t.Fatal("enrollment crossed sessions")
	}
	if f.request("PUT", "/api/v1/account/password", map[string]string{"current_password": "correct horse battery", "password": "another secure password"}).Code != 204 {
		t.Fatal("password change failed")
	}
	if f.request("POST", "/auth/local/totp/verify", payload).Code != 401 {
		t.Fatal("stale enrollment accepted")
	}
}

func TestTOTPDeviceAccountIsolation(t *testing.T) {
	f, code := localFlows(t)
	registerLocal(t, f, code)
	secret := enrollDevice(t, f, "Phone")
	user, err := f.srv.store.UserByEmail(t.Context(), "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	devices, err := f.srv.store.TOTPDevices(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	stranger, err := f.srv.store.CreateUser(t.Context(), store.User{Email: "stranger@example.com", Name: "Stranger", Role: store.RoleUser, OAuthSub: "stranger"})
	if err != nil {
		t.Fatal(err)
	}
	other := signedInUser(t, f, stranger)
	if other.request("DELETE", "/api/v1/account/totp", map[string]any{"id": devices[0].ID, "code": deviceCode(t, secret, 0)}).Code != 401 {
		t.Fatal("device removal crossed accounts")
	}
}

func TestMCPRemovesSelectedTOTPDevice(t *testing.T) {
	f, code := localFlows(t)
	registerLocal(t, f, code)
	enrollDevice(t, f, "Phone")
	secret := enrollDevice(t, f, "Tablet")
	user, err := f.srv.store.UserByEmail(t.Context(), "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	devices, err := f.srv.store.TOTPDevices(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, tokens := mcpTokens(t, f)
	result := mcpCall(t, f, tokens["access_token"].(string), "disable_totp", map[string]any{"id": devices[1].ID, "password": "correct horse battery", "code": deviceCode(t, secret, 0)})
	if toolStatus(t, result) != 204 {
		t.Fatal(result)
	}
	devices, err = f.srv.store.TOTPDevices(t.Context(), user.ID)
	if err != nil || len(devices) != 1 || devices[0].Name != "Phone" {
		t.Fatal("wrong device removed by MCP", err)
	}
}
