package config

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/OXeu/configcenter/sdk/configcenter"
)

// ErrInvalidSetting identifies a rejected configuration edit.
var ErrInvalidSetting = errors.New("invalid setting")

// Setting describes a supported runtime setting without exposing stored secrets.
type Setting struct {
	Key        string   `json:"key"`
	Value      string   `json:"value"`
	Secret     bool     `json:"secret"`
	Configured bool     `json:"configured"`
	Restart    bool     `json:"restart"`
	Pending    bool     `json:"pending"`
	Options    []string `json:"options,omitempty"`
}

var definitions = []Setting{
	{Key: "AC_ADDR", Value: "127.0.0.1:8081", Restart: true},
	{Key: "AC_PUBLIC_URL", Value: "http://127.0.0.1:8081"},
	{Key: "AC_ENV"},
	{Key: "AC_DB_DRIVER", Value: "sqlite", Restart: true, Options: []string{"sqlite", "mysql"}},
	{Key: "AC_DB", Value: "file:.tmp/authcenter.db", Secret: true, Restart: true},
	{Key: "AC_LANGUAGES", Value: "en,zh-CN"},
	{Key: "AC_LOCAL_LOGIN", Value: "true", Options: []string{"true", "false"}},
	{Key: "AC_ALLOW_REGISTRATION", Value: "true", Options: []string{"true", "false"}},
	{Key: "AC_ENCRYPTION_KEY", Secret: true, Restart: true},
	{Key: "AC_RESEND_API_KEY", Secret: true},
	{Key: "AC_RESEND_FROM"},
	{Key: "AC_OAUTH_ISSUER", Value: "https://github.com/login/oauth"},
	{Key: "AC_OAUTH_CLIENT_ID"},
	{Key: "AC_OAUTH_CLIENT_SECRET", Secret: true},
	{Key: "AC_OAUTH_SCOPES", Value: "read:user user:email"},
	{Key: "AC_OAUTH_REDIRECT_URL"},
	{Key: "AC_OAUTH_GROUPS_CLAIM", Value: "groups"},
	{Key: "AC_OAUTH_ADMIN_GROUPS", Value: "authcenter-admin"},
	{Key: "AC_OAUTH_GROUP_MAP"},
	{Key: "AC_BOOTSTRAP_ADMIN_EMAIL", Value: "admin@localhost", Restart: true},
	{Key: "AC_BOOTSTRAP_ADMIN_NAME", Value: "admin", Restart: true},
}

// Manager reads and writes the configured project's settings through the SDK.
type Manager struct {
	client *configcenter.Client
	mu     sync.Mutex
}

// NewManager retains the configured SDK client for administrative operations.
func NewManager(client *configcenter.Client) *Manager { return &Manager{client: client} }

// Read bypasses the SDK cache and validates a fresh configuration snapshot.
func (m *Manager) Read(ctx context.Context) (Config, error) {
	values, err := m.readValues(ctx)
	if err != nil {
		return Config{}, err
	}
	return m.parse(values)
}

func (m *Manager) readValues(ctx context.Context) (map[string]string, error) {
	keys := make([]string, 0, len(definitions))
	for _, item := range definitions {
		keys = append(keys, item.Key)
	}
	values, err := m.client.ReadMany(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("read configuration: %w", err)
	}
	for _, item := range definitions {
		if _, ok := values[item.Key]; !ok {
			values[item.Key] = item.Value
		}
	}
	return values, nil
}

func (m *Manager) parse(values map[string]string) (Config, error) {
	full := make(map[string]string, len(definitions))
	for _, item := range definitions {
		value, ok := values[item.Key]
		if !ok {
			value = item.Value
		}
		if len(item.Options) > 0 && !slices.Contains(item.Options, value) {
			return Config{}, fmt.Errorf("%w: %s", ErrInvalidSetting, item.Key)
		}
		full[item.Key] = value
	}
	cfg, err := fromSnapshot(func(key, fallback string) string {
		if value, ok := full[key]; ok {
			return value
		}
		return fallback
	})
	if err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalidSetting, err)
	}
	cfg.Source, cfg.Values = m, full
	return cfg, nil
}

// Save validates the merged snapshot before atomically writing the supplied keys.
func (m *Manager) Save(ctx context.Context, values map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(values) == 0 {
		return ErrInvalidSetting
	}
	current, err := m.readValues(ctx)
	if err != nil {
		return err
	}
	merged := maps.Clone(current)
	for key, value := range values {
		if _, ok := merged[key]; !ok || len(value) > 65536 {
			return fmt.Errorf("%w: unknown or oversized key", ErrInvalidSetting)
		}
		merged[key] = value
	}
	if _, err := m.parse(merged); err != nil {
		return err
	}
	if err := m.client.PutMany(ctx, values); err != nil {
		return fmt.Errorf("save configuration: %w", err)
	}
	return nil
}

// Items returns redacted settings and their differences from the active snapshot.
func (m *Manager) Items(ctx context.Context, active Config) ([]Setting, error) {
	values, err := m.readValues(ctx)
	if err != nil {
		return nil, err
	}
	items := slices.Clone(definitions)
	for i := range items {
		item := &items[i]
		item.Value = values[item.Key]
		item.Configured = item.Value != ""
		item.Pending = item.Value != active.Values[item.Key]
		if item.Secret {
			item.Value = ""
		}
	}
	return items, nil
}

// Live preserves settings whose resources cannot be replaced without a restart.
func (c Config) Live(previous Config) (live Config, restart []string) {
	c.Values = maps.Clone(c.Values)
	for _, item := range definitions {
		if item.Restart && c.Values[item.Key] != previous.Values[item.Key] {
			restart = append(restart, item.Key)
			c.Values[item.Key] = previous.Values[item.Key]
		}
	}
	c.Addr, c.DatabaseDriver, c.DatabaseDSN, c.Bootstrap, c.EncryptionKey = previous.Addr, previous.DatabaseDriver, previous.DatabaseDSN, previous.Bootstrap, previous.EncryptionKey
	if c.LocalLogin && len(c.EncryptionKey) != 32 {
		c.LocalLogin = previous.LocalLogin
		c.Values["AC_LOCAL_LOGIN"] = previous.Values["AC_LOCAL_LOGIN"]
		restart = append(restart, "AC_LOCAL_LOGIN")
	}
	return c, restart
}
