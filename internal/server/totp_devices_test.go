package server

import (
	"fmt"
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

func TestRenameTOTPDeviceValidatesOwnershipAndPreservesCredentials(t *testing.T) {
	f, code := localFlows(t)
	registerLocal(t, f, code)
	enrollDevice(t, f, "Phone")
	enrollDevice(t, f, "Tablet")
	user, err := f.srv.store.UserByEmail(t.Context(), "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	before, err := f.srv.store.TOTPDevices(t.Context(), user.ID)
	if err != nil || len(before) != 2 {
		t.Fatal("missing devices", err)
	}
	path := fmt.Sprintf("/api/v1/account/totp/%d", before[1].ID)
	otherSession := signedInUser(t, f, user)
	stranger, err := f.srv.store.CreateUser(t.Context(), store.User{Email: "stranger@example.com", Name: "Stranger", Role: store.RoleUser, OAuthSub: "stranger"})
	if err != nil {
		t.Fatal(err)
	}
	other := signedInUser(t, f, stranger)
	if other.request("PATCH", path, map[string]string{"name": "Stolen"}).Code != 404 {
		t.Fatal("device rename crossed accounts")
	}
	anonymous := &flows{t: t, srv: f.srv, router: f.router, cookies: map[string]*http.Cookie{}}
	if anonymous.request("PATCH", path, map[string]string{"name": "Anonymous"}).Code != 401 {
		t.Fatal("anonymous device rename accepted")
	}
	csrf := f.csrf
	f.csrf = ""
	if f.request("PATCH", path, map[string]string{"name": "No CSRF"}).Code != 403 {
		t.Fatal("rename accepted without CSRF")
	}
	f.csrf = csrf
	for _, name := range []string{"", " \t\n ", strings.Repeat("a", 101), strings.Repeat("验", 34)} {
		if response := f.request("PATCH", path, map[string]string{"name": name}); response.Code != 400 {
			t.Fatal("invalid device name accepted", response.Code)
		}
	}
	if f.request("PATCH", "/api/v1/account/totp/0", map[string]string{"name": "New"}).Code != 400 {
		t.Fatal("invalid device ID accepted")
	}
	if f.request("PATCH", "/api/v1/account/totp/99999", map[string]string{"name": "New"}).Code != 404 {
		t.Fatal("missing device rename accepted")
	}
	name := strings.Repeat("验", 33) + "A"
	for range 2 {
		if response := f.request("PATCH", path, map[string]string{"name": "  " + name + "  "}); response.Code != 204 {
			t.Fatal("rename failed", response.Code, response.Body.String())
		}
	}
	after, err := f.srv.store.TOTPDevices(t.Context(), user.ID)
	if err != nil || len(after) != 2 {
		t.Fatal("devices changed", err)
	}
	before[1].Name = name
	if after[0] != before[0] || after[1] != before[1] {
		t.Fatal("rename changed another device or its credentials")
	}
	if otherSession.request("GET", "/api/v1/me", nil).Code != 200 {
		t.Fatal("rename revoked another session")
	}
	settings := responseJSON(t, f.request("GET", "/api/v1/account/security", nil))
	devices := settings["totp_devices"].([]any)
	if devices[1].(map[string]any)["name"] != name {
		t.Fatal("new name missing from security settings")
	}
}

func TestTOTPDeviceRenameRequiresWriteScopeAndSupportsMCP(t *testing.T) {
	f, code := localFlows(t)
	registerLocal(t, f, code)
	enrollDevice(t, f, "Phone")
	user, err := f.srv.store.UserByEmail(t.Context(), "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	devices, err := f.srv.store.TOTPDevices(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/v1/account/totp/%d", devices[0].ID)
	_, readKey := createKey(t, f, []string{"account:read"})
	if keyRequest(t, f, readKey, "PATCH", path, map[string]string{"name": "Read key"}).Code != 403 {
		t.Fatal("read-only key renamed device")
	}
	_, writeKey := createKey(t, f, []string{"account:write"})
	if response := keyRequest(t, f, writeKey, "PATCH", path, map[string]string{"name": "Write key"}); response.Code != 204 {
		t.Fatal("write key could not rename device", response.Code)
	}
	_, tokens := mcpTokens(t, f)
	access := tokens["access_token"].(string)
	result := mcpCall(t, f, access, "rename_totp", map[string]any{"id": fmt.Sprint(devices[0].ID), "name": "MCP name"})
	if toolStatus(t, result) != 204 {
		t.Fatal(result)
	}
	if mcpRequest(t, f.router, access, "tools/list", nil).Code != 200 {
		t.Fatal("rename revoked MCP authorization")
	}
	devices, err = f.srv.store.TOTPDevices(t.Context(), user.ID)
	if err != nil || devices[0].Name != "MCP name" {
		t.Fatal("MCP rename not saved", err)
	}
}
