package server

import (
	"context"
	"crypto/rand"
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

	"github.com/xeu/configcenter"
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
	template      *template.Template
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

func (s *Server) loadTemplates() error {
	tpl, err := template.New("shell").ParseFS(templateFS, "templates/shell/*.html")
	if err != nil {
		return fmt.Errorf("parse Vue shell: %w", err)
	}
	s.template = tpl
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
		"CSRF":          auth.CSRF(c),
		"Lang":          s.translator(c).Lang(),
		"LanguageLabel": label,
		"Languages":     language,
	})
	for key, value := range data {
		withCommon[key] = value
	}
	payload := gin.H{
		"nonce": c.GetString("cspNonce"), "page": "login", "lang": withCommon["Lang"], "title": withCommon["Title"],
		"publicUrl": withCommon["PublicURL"], "providerName": withCommon["ProviderName"],
		"csrf": withCommon["CSRF"], "loggedIn": withCommon["LoggedIn"],
		"isAdmin": withCommon["IsAdmin"], "oauthEnabled": withCommon["OAuthEnabled"],
		"me": withCommon["Me"], "project": withCommon["Project"], "env": "",
		"canWrite": loggedIn && id.User != nil && id.User.Role.CanWrite(), "languageLabel": label,
		"messages": s.catalog.Messages(s.translator(c).Lang()), "flash": nil,
	}
	for _, key := range []string{"Page", "Env", "CanWrite"} {
		if value, ok := withCommon[key]; ok {
			payload[strings.ToLower(key[:1])+key[1:]] = value
		}
	}
	if loggedIn && id.User != nil {
		payload["me"] = userJSON{ID: id.User.ID, Name: id.User.Name, AvatarURL: id.User.AvatarURL, Email: id.User.Email, Role: id.User.Role, LastLoginAt: id.User.LastLoginAt}
	}
	if project, ok := withCommon["Project"].(*store.Project); ok {
		payload["project"] = toProjectJSON(*project)
	}
	withCommon["Title"] = s.translator(c).T(fmt.Sprint(withCommon["Title"]))
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
	tpl := s.template
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
	frontend, frontendErr := fs.Sub(configcenter.Frontend, ".tmp/ui")
	if frontendErr != nil {
		s.log.Error("mount frontend", "error", frontendErr)
	}
	r.GET("/static/*filepath", func(c *gin.Context) {
		if strings.HasPrefix(c.Param("filepath"), "/app/") {
			c.Header("Cache-Control", "no-cache")
			http.StripPrefix("/static/app/", http.FileServer(http.FS(frontend))).ServeHTTP(c.Writer, c.Request)
			return
		}
		http.StripPrefix("/static/", http.FileServer(http.FS(assets))).ServeHTTP(c.Writer, c.Request)
	})

	mountDocs(r)
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
		nonce := rand.Text()
		c.Set("cspNonce", nonce)
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'nonce-"+nonce+"'; script-src 'self'; img-src 'self' data: https:; connect-src 'self'; form-action 'self'")
		c.Next()
	}
}
