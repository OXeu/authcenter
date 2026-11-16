package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadReadsEnvFileWithoutOverridingExportedValues(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, ".env.local")
	contents := "CC_ADDR=0.0.0.0:9999\nCC_PUBLIC_URL=http://files.example/\nCC_OAUTH_ISSUER=https://github.com/login/oauth/\n"
	if err := os.WriteFile(file, []byte(contents), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	t.Setenv("CC_ENV_FILE", file)
	t.Setenv("CC_ADDR", "127.0.0.1:7777")

	cfg, err := Load(os.LookupEnv)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Addr != "127.0.0.1:7777" {
		t.Fatalf("exported CC_ADDR was overridden: %q", cfg.Addr)
	}
	if cfg.PublicURL != "http://files.example" {
		t.Fatalf("file value not used or not trimmed: %q", cfg.PublicURL)
	}
	if cfg.OAuth.IssuerURL != "https://github.com/login/oauth" {
		t.Fatalf("issuer trailing slash not trimmed: %q", cfg.OAuth.IssuerURL)
	}
	if cfg.OAuth.RedirectURL != "http://files.example/oauth/callback" {
		t.Fatalf("redirect not derived: %q", cfg.OAuth.RedirectURL)
	}
}

func TestLoadToleratesMissingEnvFile(t *testing.T) {
	t.Setenv("CC_ENV_FILE", filepath.Join(t.TempDir(), "absent.env"))
	cfg, err := Load(os.LookupEnv)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Addr == "" {
		t.Fatal("defaults were not applied")
	}
}

func TestDatabaseDrivers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		driver string
		dsn    string
		valid  bool
	}{
		{"default", "", "", true},
		{"mysql", "mysql", "user:password@tcp(localhost:3306)/configcenter", true},
		{"mysql missing dsn", "mysql", "", false},
		{"mysql missing database", "mysql", "user:password@tcp(localhost:3306)/", false},
		{"unknown", "postgres", "", false},
		{"sqlite invalid", "sqlite", "invalid", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{"CC_DB_DRIVER": tc.driver, "CC_DB": tc.dsn, "CC_ENV_FILE": "missing-test-env"}
			cfg, err := Load(func(key string) (string, bool) { value, ok := values[key]; return value, ok })
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
			if tc.name == "default" && cfg.DatabaseDriver != "sqlite" {
				t.Fatal("default must remain sqlite")
			}
			if tc.name == "mysql" && !strings.Contains(cfg.DatabaseDSN, "parseTime=true") {
				t.Fatal("mysql must parse timestamps")
			}
		})
	}
}
