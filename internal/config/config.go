package config

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/OXeu/configcenter/sdk/configcenter"

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
	SessionTTL     time.Duration
	// Languages lists the enabled UI languages; the first supported one is
	// the default. Tags must match the locale files in internal/i18n.
	Languages         []string
	OAuth             auth.Config
	Groups            auth.GroupPolicy
	Bootstrap         Bootstrap
	LocalLogin        bool
	AllowRegistration bool
	ResendKey         string
	ResendFrom        string
	EncryptionKey     []byte
}

// Bootstrap is the first-run administrator created when no user exists yet.
type Bootstrap struct {
	AdminEmail string
	AdminName  string
}

// DefaultEnvFile is read by the binary unless AC_ENV_FILE points elsewhere.
const DefaultEnvFile = ".env.local"

// Load reads configcenter connection settings from the environment and all
// runtime settings through the configcenter SDK. Exported bootstrap settings
// take precedence over the optional local env file.
func Load(ctx context.Context, lookup func(string) (string, bool)) (Config, error) {
	envFile := lookupOr(lookup, "AC_ENV_FILE", DefaultEnvFile)
	if err := godotenv.Load(envFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("load env file %s: %w", envFile, err)
	}

	client, err := configcenter.New(configcenter.Config{
		BaseURL:     lookupOr(lookup, "AC_CONFIGCENTER_URL", ""),
		APIKey:      lookupOr(lookup, "AC_CONFIGCENTER_KEY", ""),
		Environment: lookupOr(lookup, "AC_CONFIGCENTER_ENVIRONMENT", "production"),
		DotenvFiles: []string{},
	})
	if err != nil {
		return Config{}, fmt.Errorf("configure configcenter SDK: %w", err)
	}
	if client.IsLocal() {
		return Config{}, errors.New("AC_CONFIGCENTER_URL and AC_CONFIGCENTER_KEY are required")
	}
	if err := client.Load(ctx); err != nil {
		return Config{}, fmt.Errorf("load configcenter snapshot: %w", err)
	}
	return fromSnapshot(client.GetDefault)
}

func fromSnapshot(env func(string, string) string) (Config, error) {
	cfg := Config{
		Addr:           env("AC_ADDR", "127.0.0.1:8081"),
		DatabaseDriver: env("AC_DB_DRIVER", "sqlite"),
		DatabaseDSN:    env("AC_DB", "file:.tmp/authcenter.db"),
		PublicURL:      strings.TrimRight(env("AC_PUBLIC_URL", "http://127.0.0.1:8081"), "/"),

		SessionTTL:        24 * time.Hour,
		LocalLogin:        env("AC_LOCAL_LOGIN", "true") == "true",
		AllowRegistration: env("AC_ALLOW_REGISTRATION", "true") == "true",
		ResendKey:         env("AC_RESEND_API_KEY", ""),
		ResendFrom:        env("AC_RESEND_FROM", ""),
		Languages:         splitList(env("AC_LANGUAGES", "en,zh-CN")),
		OAuth: auth.Config{
			IssuerURL:    strings.TrimRight(env("AC_OAUTH_ISSUER", "https://github.com/login/oauth"), "/"),
			ClientID:     env("AC_OAUTH_CLIENT_ID", ""),
			ClientSecret: env("AC_OAUTH_CLIENT_SECRET", ""),
			Scopes:       strings.Fields(env("AC_OAUTH_SCOPES", "read:user user:email")),
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
	public, parseErr := url.Parse(cfg.PublicURL)
	if parseErr != nil || !validPublicURL(public) {
		errs = append(errs, errors.New("AC_PUBLIC_URL must be an HTTPS origin, or HTTP loopback origin"))
	}

	if cfg.LocalLogin {
		key, err := base64.StdEncoding.DecodeString(env("AC_ENCRYPTION_KEY", ""))
		if err != nil || len(key) != 32 {
			errs = append(errs, errors.New("AC_ENCRYPTION_KEY must be a base64-encoded 32-byte key"))
		} else {
			cfg.EncryptionKey = key
		}
	}
	if env("AC_ENV", "") == "production" && public != nil && public.Scheme != "https" {
		errs = append(errs, errors.New("production requires HTTPS"))
	}
	if _, port, err := net.SplitHostPort(cfg.Addr); err != nil || port == "" {
		errs = append(errs, fmt.Errorf("authcenter addr %q is not host:port", cfg.Addr))
	}
	if cfg.DatabaseDriver != "sqlite" && cfg.DatabaseDriver != "mysql" {
		errs = append(errs, errors.New("authcenter db driver must be sqlite or mysql"))
	}
	if cfg.DatabaseDriver == "mysql" && (cfg.DatabaseDSN == "" || strings.HasPrefix(cfg.DatabaseDSN, "file:")) {
		errs = append(errs, errors.New("authcenter db requires a MySQL DSN when using mysql"))
	}
	if cfg.DatabaseDriver == "mysql" {
		parsed, err := mysql.ParseDSN(cfg.DatabaseDSN)
		if err != nil || parsed.DBName == "" {
			errs = append(errs, errors.New("authcenter db must be a valid MySQL DSN with a database name"))
		} else {
			parsed.ParseTime = true
			cfg.DatabaseDSN = parsed.FormatDSN()
		}
	}
	if cfg.DatabaseDriver == "sqlite" && !strings.HasPrefix(cfg.DatabaseDSN, "file:") {
		errs = append(errs, errors.New("authcenter db must be a sqlite dsn starting with file"))
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

func validPublicURL(u *url.URL) bool {
	if u == nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return false
	}
	return u.Scheme == "https" || u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
}
