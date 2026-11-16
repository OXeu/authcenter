package server

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"

	"github.com/xeu/configcenter/internal/auth"
	"github.com/xeu/configcenter/internal/config"
	"github.com/xeu/configcenter/internal/i18n"
	"github.com/xeu/configcenter/internal/store"
)

//go:embed templates/shell/vue.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// Server wires storage, authentication and HTTP handlers together.
type Server struct {
	cfg           config.Config
	store         *store.Store
	authenticator *auth.Authenticator
	oauth         auth.Provider
	groupPolicy   auth.GroupPolicy
	catalog       *i18n.Catalog
	log           *slog.Logger
	funcs         template.FuncMap
	templates     map[string]*template.Template
}

// Engine builds the gin engine for the given configuration.
func Engine(ctx context.Context, cfg config.Config, log *slog.Logger) (*gin.Engine, error) {
	s, err := newServer(ctx, cfg, log)
	if err != nil {
		return nil, fmt.Errorf("start server: %w", err)
	}
	return s.router(), nil
}

func newServer(ctx context.Context, cfg config.Config, log *slog.Logger) (*Server, error) {
	dsn, err := sqliteDSN(cfg.DatabaseDSN)
	if err != nil {
		return nil, err
	}
	st, err := store.Open(ctx, sqlite.Open(dsn), cfg.Bootstrap.AdminEmail, cfg.Bootstrap.AdminName)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	provider, err := auth.NewProvider(ctx, cfg.OAuth)
	if err != nil {
		return nil, fmt.Errorf("configure oauth: %w", err)
	}
	catalog, err := i18n.Shared()
	if err != nil {
		return nil, fmt.Errorf("load messages: %w", err)
	}
	if len(catalog.Languages(cfg.Languages)) == 0 {
		return nil, fmt.Errorf("configure languages: %w", errNoLanguage)
	}
	s := &Server{
		cfg:           cfg,
		store:         st,
		authenticator: auth.New(st, st, cfg.SessionTTL),
		oauth:         provider,
		groupPolicy:   cfg.Groups,
		catalog:       catalog,
		log:           log,
		funcs: template.FuncMap{
			"derefTime": derefTime,
			"shortTime": func(t time.Time) string { return t.UTC().Format("2006-01-02 15:04") },
		},
	}
	if err := s.loadTemplates(); err != nil {
		return nil, err
	}
	return s, nil
}

func sqliteDSN(dsn string) (string, error) {
	if !strings.HasPrefix(dsn, "file:") {
		return "", errors.New("database dsn must start with file")
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", nil
}

var errNoLanguage = errors.New("no enabled language has a locale file")

// message is the plain translation of an id.
func message(tr *i18n.Translator, id string) string {
	return tr.T(id)
}

// renderMessage substitutes {{.Value}} in a message with one string.
func renderMessage(msg string, value any) (string, bool) {
	tpl, err := template.New("msg").Parse(msg)
	if err != nil {
		return "", false
	}
	var out strings.Builder
	if err := tpl.Execute(&out, map[string]any{"Value": fmt.Sprint(value)}); err != nil {
		return "", false
	}
	return out.String(), true
}

// loadTemplates parses the shell once per enabled language, binding `t` to
// that language's translator, so rendering never clones or re-parses.
func (s *Server) loadTemplates() error {
	s.templates = make(map[string]*template.Template, len(s.cfg.Languages))
	for _, language := range s.catalog.Languages(s.cfg.Languages) {
		tr := s.catalog.Translator(language.Tag)
		funcs := template.FuncMap{}
		for name, fn := range s.funcs {
			funcs[name] = fn
		}
		funcs["roleLabel"] = func(role string) template.HTML {
			return template.HTML(template.HTMLEscapeString(tr.T("role." + role))) //nolint:gosec // escaped above
		}
		funcs["msg"] = func(id string) string { return tr.T(id) }
		funcs["t"] = func(id string, data ...any) template.HTML {
			msg := message(tr, id)
			if len(data) > 1 {
				// `{{t "id" . extra}}` passes the trailing value explicitly.
				msg = strings.ReplaceAll(msg, "{{.ProviderName}}", "{{.Value}}")
				if rendered, ok := renderMessage(msg, data[1]); ok {
					return template.HTML(template.HTMLEscapeString(rendered)) //nolint:gosec // escaped above
				}
			}
			return template.HTML(template.HTMLEscapeString(msg)) //nolint:gosec // escaped above
		}
		tpl, err := template.New("shell").Funcs(funcs).ParseFS(templateFS, "templates/shell/*.html")
		if err != nil {
			return fmt.Errorf("parse templates for %s: %w", language.Tag, err)
		}
		s.templates[language.Tag] = tpl
	}
	return nil
}

// viewData returns the language-independent values available to templates.
func (s *Server) viewData(_ *gin.Context, extra gin.H) gin.H {
	data := gin.H{
		"ProviderName": s.providerName(),
		"PublicURL":    s.cfg.PublicURL,
	}
	for key, value := range extra {
		data[key] = value
	}
	return data
}

func (s *Server) render(c *gin.Context, status int, data gin.H) {
	id, loggedIn := auth.FromContext(c.Request.Context())
	language := s.catalog.Languages(s.cfg.Languages)
	label := "English"
	for _, l := range language {
		if l.Tag == s.translator(c).Lang() {
			label = l.Label
		}
	}
	withCommon := s.viewData(c, gin.H{
		"OAuthEnabled":  s.oauth != nil,
		"LoggedIn":      loggedIn && id.User != nil,
		"IsAdmin":       loggedIn && id.User != nil && id.User.Role == store.RoleAdmin,
		"RoleLabel":     roleKey(id.Role),
		"CSRF":          auth.CSRF(c),
		"Lang":          s.translator(c).Lang(),
		"LanguageLabel": label,
		"Languages":     language,
	})
	for key, value := range data {
		withCommon[key] = value
	}
	payload := gin.H{
		"page": "login", "lang": withCommon["Lang"], "title": withCommon["Title"],
		"publicUrl": withCommon["PublicURL"], "providerName": withCommon["ProviderName"],
		"csrf": withCommon["CSRF"], "loggedIn": withCommon["LoggedIn"],
		"isAdmin": withCommon["IsAdmin"], "oauthEnabled": withCommon["OAuthEnabled"],
		"me": withCommon["Me"], "project": withCommon["Project"], "env": "",
		"canWrite": false, "languageLabel": label,
		"messages": s.catalog.Messages(s.translator(c).Lang()), "flash": nil,
	}
	for _, key := range []string{"Page", "Env", "CanWrite"} {
		if value, ok := withCommon[key]; ok {
			payload[strings.ToLower(key[:1])+key[1:]] = value
		}
	}
	languages := make([]gin.H, 0, len(language))
	for _, item := range language {
		languages = append(languages, gin.H{"tag": item.Tag, "label": item.Label})
	}
	payload["languages"] = languages
	if message, ok := withCommon["LoginFailed"].(string); ok && message != "" {
		payload["flash"] = gin.H{"message": message, "level": "error"}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		s.log.Error("encode page data", "error", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	withCommon["PageJSON"] = template.JS(encoded) //nolint:gosec // json.Marshal escapes HTML delimiters before insertion into the inert script.
	tpl := s.templates[s.translator(c).Lang()]
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(status)
	if err := tpl.ExecuteTemplate(c.Writer, "vue.html", withCommon); err != nil {
		s.log.Error("render template", "error", err)
	}
}

func (s *Server) router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), s.requestLogger())
	r.Use(securityHeaders())
	r.Use(s.authenticator.BrowserMiddleware())
	r.Use(s.languageMiddleware())

	assets, err := fs.Sub(staticFS, "static")
	if err != nil {
		s.log.Error("mount static assets", "error", err)
	}
	r.StaticFS("/static", http.FS(assets))

	s.MountOAuth(r)
	s.MountLanguage(r)
	r.GET("/", func(c *gin.Context) { c.Redirect(http.StatusFound, "/projects") })

	s.MountUI(r)
	s.MountAPI(r)
	s.MountAgent(r)
	s.MountUIAPI(r)
	return r
}

func (s *Server) requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		s.log.Info("request", "method", c.Request.Method, "path", c.Request.URL.Path, "status", c.Writer.Status(), "duration", time.Since(start))
	}
}

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; connect-src 'self'; form-action 'self'")
		c.Next()
	}
}

// roleKey is the message id of a role label.
func roleKey(role store.Role) string {
	switch role {
	case store.RoleAdmin, store.RoleEditor, store.RoleViewer:
		return "role." + string(role)
	default:
		return string(role)
	}
}

func derefTime(p *time.Time) string {
	if p == nil {
		return ""
	}
	return p.UTC().Format("2006-01-02 15:04")
}
