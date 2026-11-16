package config

import (
	"os"
	"path/filepath"
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
