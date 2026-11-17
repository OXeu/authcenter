package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/OXeu/authcenter/internal/store"
)

func TestBatchConfigurationPermissionsAndIsolation(t *testing.T) {
	f := newFlows(t)
	for _, name := range []string{"batch", "other"} {
		if status, _ := f.do(http.MethodPost, "/api/v1/projects", map[string]string{"slug": name, "name": name}); status != http.StatusCreated {
			t.Fatal(status)
		}
	}
	base := "/api/v1/projects/batch/environments/production/items"
	if status, _ := f.do(http.MethodPut, base+"/keep", map[string]string{"value": "original"}); status != 200 {
		t.Fatal(status)
	}
	_, created := f.do(http.MethodPost, "/api/v1/keys", scopeBody("writer", keyScopeJSON{Project: "batch", Permissions: []store.Permission{store.ConfigWrite}}))
	writer := created["token"].(string)
	body := map[string]any{"items": []map[string]string{{"key": "keep", "value": "updated", "description": "new description"}, {"key": "db/pool", "value": ""}}}
	keyRequest(t, f.router, writer, http.MethodPut, base, body, http.StatusNoContent)
	keyRequest(t, f.router, writer, http.MethodGet, base, nil, http.StatusForbidden)
	keyRequest(t, f.router, writer, http.MethodPut, "/api/v1/projects/other/environments/production/items", body, http.StatusForbidden)
	_, created = f.do(http.MethodPost, "/api/v1/keys", scopeBody("reader", keyScopeJSON{Project: "batch", Permissions: []store.Permission{store.ConfigRead}}))
	reader := created["token"].(string)
	keyRequest(t, f.router, reader, http.MethodPut, base, body, http.StatusForbidden)
	query := "?" + url.Values{"keys": {"keep", "db/pool", "missing"}}.Encode()
	for _, path := range []string{base, "/api/v1/agent/environments/production/values"} {
		response := keyRequest(t, f.router, reader, http.MethodGet, path+query, nil, http.StatusOK)
		var selected struct {
			Items   []itemJSON `json:"items"`
			Missing []string   `json:"missing_keys"`
		}
		json.Unmarshal(response.Body.Bytes(), &selected)
		if len(selected.Items) != 2 || len(selected.Missing) != 1 || selected.Missing[0] != "missing" {
			t.Fatal("bad batch selection", response.Body.String())
		}
		if selected.Items[0].Key != "db/pool" || selected.Items[0].Value != "" || selected.Items[1].Value != "updated" || selected.Items[1].Description != "new description" {
			t.Fatal("batch upsert failed", selected)
		}
	}
	keyRequest(t, f.router, reader, http.MethodGet, "/api/v1/projects/other/environments/production/items"+query, nil, http.StatusForbidden)
	if status, _ := f.do(http.MethodPost, "/api/v1/projects/batch/environments", map[string]string{"slug": "staging", "name": "Staging"}); status != 201 {
		t.Fatal(status)
	}
	response := keyRequest(t, f.router, reader, http.MethodGet, "/api/v1/projects/batch/environments/staging/items"+query, nil, http.StatusOK)
	var empty struct {
		Items   []itemJSON `json:"items"`
		Missing []string   `json:"missing_keys"`
	}
	json.Unmarshal(response.Body.Bytes(), &empty)
	if len(empty.Items) != 0 || len(empty.Missing) != 3 {
		t.Fatal("environment leaked", response.Body.String())
	}
	f.csrf = ""
	if status, _ := f.do(http.MethodPut, base, body); status != http.StatusForbidden {
		t.Fatal("missing CSRF accepted", status)
	}
}

func TestBatchValidationPreventsPartialWrites(t *testing.T) {
	f := newFlows(t)
	if status, _ := f.do(http.MethodPost, "/api/v1/projects", map[string]string{"slug": "batch", "name": "Batch"}); status != 201 {
		t.Fatal(status)
	}
	base := "/api/v1/projects/batch/environments/production/items"
	for _, items := range [][]map[string]any{
		nil,
		{{"key": "valid", "value": "value"}, {"key": "invalid key", "value": "secret"}},
		{{"key": "valid", "value": "value"}, {"key": "valid", "value": "second"}},
		{{"key": "valid", "value": "value"}, {"key": "missing"}},
		{{"key": "valid", "value": nil}},
		{{"key": "valid", "value": false}},
		{{"key": "valid", "value": strings.Repeat("x", 1<<20)}},
		make([]map[string]any, 501),
	} {
		status, _ := f.do(http.MethodPut, base, map[string]any{"items": items})
		if status != http.StatusBadRequest {
			t.Fatal("invalid batch accepted", status)
		}
		if status, _ := f.do(http.MethodGet, base+"/valid", nil); status != http.StatusNotFound {
			t.Fatal("partial write", status)
		}
	}
	for _, query := range []string{"keys=", "keys=bad+key", "keys=a&keys=a", "keys=a&prefix=b", strings.Repeat("keys=a&", 501)} {
		if status, _ := f.do(http.MethodGet, base+"?"+query, nil); status != http.StatusBadRequest {
			t.Fatal("invalid selection accepted", query, status)
		}
	}
	if status, _ := f.do(http.MethodPost, "/api/v1/projects/batch/environments/production/defaults", map[string]any{"defaults": map[string]string{"key": "value"}}); status != http.StatusNotFound {
		t.Fatal("initialize API still mounted", status)
	}
}
