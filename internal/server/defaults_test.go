package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/OXeu/configcenter/internal/store"
)

func TestDefaultsPreserveValuesAndEnforcePermissions(t *testing.T) {
	f := newFlows(t)
	for _, name := range []string{"defaults", "other"} {
		if status, _ := f.do(http.MethodPost, "/api/v1/projects", map[string]string{"slug": name, "name": name}); status != http.StatusCreated {
			t.Fatal("create project", status)
		}
	}
	base := "/api/v1/projects/defaults/environments/production"
	status, original := f.do(http.MethodPut, base+"/items/keep", map[string]string{"value": "original", "description": "retained"})
	if status != 200 {
		t.Fatal(status)
	}
	_, created := f.do(http.MethodPost, "/api/v1/keys", scopeBody("initializer", keyScopeJSON{Project: "defaults", Permissions: []store.Permission{store.ConfigWrite, store.ConfigRead}}))
	token := created["token"].(string)
	body := map[string]any{"defaults": map[string]string{"keep": "replace", "created": "default-value"}}
	keyRequest(t, f.router, token, http.MethodPost, base+"/defaults", body, http.StatusNoContent)
	keyRequest(t, f.router, token, http.MethodPost, base+"/defaults", body, http.StatusNoContent)
	response := keyRequest(t, f.router, token, http.MethodGet, base+"/items/keep", nil, http.StatusOK)
	var kept map[string]any
	json.Unmarshal(response.Body.Bytes(), &kept)
	for _, field := range []string{"value", "description", "updated_at"} {
		if kept[field] != original[field] {
			t.Fatal("initialization overwrote existing field", field)
		}
	}
	response = keyRequest(t, f.router, token, http.MethodGet, base+"/items/created", nil, http.StatusOK)
	json.Unmarshal(response.Body.Bytes(), &kept)
	if kept["value"] != "default-value" {
		t.Fatal("missing default")
	}
	keyRequest(t, f.router, token, http.MethodPost, "/api/v1/projects/other/environments/production/defaults", body, http.StatusForbidden)
	_, created = f.do(http.MethodPost, "/api/v1/keys", scopeBody("reader", keyScopeJSON{Project: "defaults", Permissions: []store.Permission{store.ConfigRead}}))
	keyRequest(t, f.router, created["token"].(string), http.MethodPost, base+"/defaults", body, http.StatusForbidden)
	f.csrf = ""
	if status, _ := f.do(http.MethodPost, base+"/defaults", body); status != http.StatusForbidden {
		t.Fatal("missing CSRF accepted", status)
	}
	keyRequest(t, f.router, token, http.MethodPost, base+"/defaults", map[string]any{"defaults": map[string]string{"valid": "value", "invalid key": "bad"}}, http.StatusBadRequest)
	keyRequest(t, f.router, token, http.MethodGet, base+"/items/valid", nil, http.StatusNotFound)
}
