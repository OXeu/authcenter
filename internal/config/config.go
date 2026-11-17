package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"

	"github.com/OXeu/authcenter/internal/auth"
)

// Config holds every runtime setting of the server.
type Config struct {
	Addr           string
	DatabaseDriver string
	DatabaseDSN    string
	PublicURL      string
	SessionSecret  string
	SessionTTL     time.Duration
	// Languages lists the enabled UI languages; the first supported one is
	// the default. Tags must match the locale files in internal/i18n.
	Languages []string
	OAuth     auth.Config
	Groups    auth.GroupPolicy
	Bootstrap Bootstrap
}

// Bootstrap is the first-run administrator created when no user exists yet.
type Bootstrap struct {
	AdminEmail string
	AdminName  string
}

// DefaultEnvFile is read by the binary unless AC_ENV_FILE points elsewhere.
const DefaultEnvFile = ".env.local"

// Load reads configuration from the environment. It first loads the env file
// (AC_ENV_FILE, default .env.local) with godotenv, which does not override
// variables that are already set, so `AC_ADDR=… ./authcenter` and exported
// variables still win. A missing file is not an error.
func Load(lookup func(string) (string, bool)) (Config, error) {
	envFile := lookupOr(lookup, "AC_ENV_FILE", DefaultEnvFile)
	if err := godotenv.Load(envFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("load env file %s: %w", envFile, err)
	}
	env := func(key, fallback string) string {
		return lookupOr(lookup, key, fallback)
	}

	cfg := Config{
		Addr:           env("AC_ADDR", "127.0.0.1:8080"),
		DatabaseDriver: env("AC_DB_DRIVER", "sqlite"),
		DatabaseDSN:    env("AC_DB", "file:authcenter.db"),
		PublicURL:      strings.TrimRight(env("AC_PUBLIC_URL", "http://127.0.0.1:8080"), "/"),
		SessionSecret:  env("AC_SESSION_SECRET", ""),
		SessionTTL:     24 * time.Hour,
		Languages:      splitList(env("AC_LANGUAGES", "en,zh-CN")),
		OAuth: auth.Config{
			IssuerURL:    strings.TrimRight(env("AC_OAUTH_ISSUER", ""), "/"),
			ClientID:     env("AC_OAUTH_CLIENT_ID", ""),
			ClientSecret: env("AC_OAUTH_CLIENT_SECRET", ""),
			Scopes:       strings.Fields(env("AC_OAUTH_SCOPES", "openid profile email groups")),
			RedirectURL:  env("AC_OAUTH_REDIRECT_URL", ""),
			GroupsClaim:  env("AC_OAUTH_GROUPS_CLAIM", "groups"),
		},
		Groups: auth.GroupPolicy{
			AdminGroups: splitList(env("AC_OAUTH_ADMIN_GROUPS", "authcenter-admin")),
			Mapping:     parseGroupMap(env("AC_OAUTH_GROUP_MAP", "")),
		},
		Bootstrap: Bootstrap{
			AdminEmail: env("AC_BOOTSTRAP_ADMIN_EMAIL", "admin@localhost"),
			AdminName:  env("AC_BOOTSTRAP_ADMIN_NAME", "admin"),
		},
	}

	if cfg.OAuth.RedirectURL == "" {
		cfg.OAuth.RedirectURL = cfg.PublicURL + "/oauth/callback"
	}

	var errs []error
	if _, port, err := net.SplitHostPort(cfg.Addr); err != nil || port == "" {
		errs = append(errs, fmt.Errorf("cc addr %q is not host:port", cfg.Addr))
	}
	if cfg.DatabaseDriver != "sqlite" && cfg.DatabaseDriver != "mysql" {
		errs = append(errs, errors.New("cc db driver must be sqlite or mysql"))
	}
	if cfg.DatabaseDriver == "mysql" && (cfg.DatabaseDSN == "" || strings.HasPrefix(cfg.DatabaseDSN, "file:")) {
		errs = append(errs, errors.New("cc db requires a MySQL DSN when using mysql"))
	}
	if cfg.DatabaseDriver == "mysql" {
		parsed, err := mysql.ParseDSN(cfg.DatabaseDSN)
		if err != nil || parsed.DBName == "" {
			errs = append(errs, errors.New("cc db must be a valid MySQL DSN with a database name"))
		} else {
			parsed.ParseTime = true
			cfg.DatabaseDSN = parsed.FormatDSN()
		}
	}
	if cfg.DatabaseDriver == "sqlite" && !strings.HasPrefix(cfg.DatabaseDSN, "file:") {
		errs = append(errs, errors.New("cc db must be a sqlite dsn starting with file"))
	}
	if cfg.SessionSecret == "" && env("AC_ENV", "") == "production" {
		errs = append(errs, errors.New("cc session secret is required in production mode"))
	}
	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}

// lookupOr returns the value of key when it is set and non-empty.
func lookupOr(lookup func(string) (string, bool), key, fallback string) string {
	if value, ok := lookup(key); ok && value != "" {
		return value
	}
	return fallback
}

func splitList(raw string) []string {
	out := make([]string, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// parseGroupMap parses "group:role,group:role".
func parseGroupMap(raw string) map[string]string {
	m := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		group, role, ok := strings.Cut(strings.TrimSpace(pair), ":")
		if !ok {
			continue
		}
		group, role = strings.TrimSpace(group), strings.TrimSpace(role)
		if group != "" && role != "" {
			m[group] = role
		}
	}
	return m
}
