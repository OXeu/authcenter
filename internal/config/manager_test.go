package config

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/OXeu/configcenter/sdk/configcenter"
)

func TestManagerReadsFreshValuesAndRedactsSecrets(t *testing.T) {
	var mu sync.Mutex
	values := map[string]string{"AC_LOCAL_LOGIN": "false", "AC_RESEND_API_KEY": "private-mail-key"}
	reads, writes := 0, 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.URL.Path == "/api/v1/agent/project":
			json.NewEncoder(w).Encode(map[string]any{"project": map[string]string{"slug": "auth"}})
		case r.Method == "PUT":
			var body struct {
				Items []configcenter.Item `json:"items"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			for _, item := range body.Items {
				values[item.Key] = item.Value
			}
			writes++
			w.WriteHeader(204)
		default:
			reads++
			items := []configcenter.Item{}
			for key, value := range values {
				items = append(items, configcenter.Item{Key: key, Value: value})
			}
			json.NewEncoder(w).Encode(map[string]any{"items": items})
		}
	}))
	defer remote.Close()
	client, err := configcenter.New(configcenter.Config{BaseURL: remote.URL, APIKey: "cc_test", DotenvFiles: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(client)
	active, err := manager.Read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	items, err := manager.Items(t.Context(), active)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(items)
	if err != nil || strings.Contains(string(encoded), "private-mail-key") {
		t.Fatal("secret exposed", err)
	}
	if err := manager.Save(t.Context(), map[string]string{"AC_ALLOW_REGISTRATION": "false"}); err != nil {
		t.Fatal(err)
	}
	current, err := manager.Read(t.Context())
	if err != nil || current.AllowRegistration {
		t.Fatal("snapshot not refreshed", err)
	}
	if err := manager.Save(t.Context(), map[string]string{"AC_LOCAL_LOGIN": "garbage"}); !errors.Is(err, ErrInvalidSetting) {
		t.Fatal("invalid option accepted", err)
	}
	if err := manager.Save(t.Context(), map[string]string{"AC_CONFIGCENTER_API_KEY": "cc_overwrite"}); !errors.Is(err, ErrInvalidSetting) {
		t.Fatal("bootstrap edit accepted", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if reads < 4 || writes != 1 {
		t.Fatal("cache or validation bypass", reads, writes)
	}
}

func TestManagerCanRepairInvalidRemoteSnapshot(t *testing.T) {
	client, err := configcenter.New(configcenter.Config{DotenvFiles: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.PutMany(t.Context(), map[string]string{"AC_LOCAL_LOGIN": "false", "AC_LANGUAGES": "unknown"}); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(client)
	if _, err := manager.Read(t.Context()); !errors.Is(err, ErrInvalidSetting) {
		t.Fatal(err)
	}
	if _, err := manager.Items(t.Context(), Config{}); err != nil {
		t.Fatal("invalid setting hidden from editor", err)
	}
	if err := manager.Save(t.Context(), map[string]string{"AC_LANGUAGES": "en"}); err != nil {
		t.Fatal(err)
	}
}
