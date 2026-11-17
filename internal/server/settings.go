package server

import (
	"errors"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/gin-gonic/gin"

	"github.com/OXeu/authcenter/internal/auth"
	"github.com/OXeu/authcenter/internal/config"
	"github.com/OXeu/authcenter/internal/credentials"
)

type runtimeConfig struct {
	mu      sync.Mutex
	current atomic.Pointer[Server]
}

func (s *Server) routeSnapshot() gin.HandlerFunc {
	return func(c *gin.Context) {
		active := s.runtime.current.Load()
		if active != nil && active != s {
			active.routes.ServeHTTP(c.Writer, c.Request)
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *Server) configurationAvailable(c *gin.Context) bool {
	if s.cfg.Source == nil {
		oauthError(c, 503, "configuration_unavailable")
		return false
	}
	return true
}

func (s *Server) apiSettings(c *gin.Context) {
	if !s.configurationAvailable(c) {
		return
	}
	items, err := s.cfg.Source.Items(c.Request.Context(), s.cfg)
	if err != nil {
		s.configurationError(c, err)
		return
	}
	c.JSON(200, gin.H{"items": items})
}

func (s *Server) apiSaveSettings(c *gin.Context) {
	if !s.configurationAvailable(c) {
		return
	}
	var in struct {
		Values map[string]string `json:"values"`
	}
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	s.runtime.mu.Lock()
	defer s.runtime.mu.Unlock()
	if err := s.cfg.Source.Save(c.Request.Context(), in.Values); err != nil {
		s.configurationError(c, err)
		return
	}
	c.Status(204)
}

func (s *Server) apiRefreshSettings(c *gin.Context) {
	if !s.configurationAvailable(c) {
		return
	}
	s.runtime.mu.Lock()
	defer s.runtime.mu.Unlock()
	active := s.runtime.current.Load()
	cfg, err := active.cfg.Source.Read(c.Request.Context())
	if err != nil {
		s.configurationError(c, err)
		return
	}
	cfg, restart := cfg.Live(active.cfg)
	provider := active.oauth
	if !reflect.DeepEqual(cfg.OAuth, active.cfg.OAuth) {
		provider, err = auth.NewProvider(c.Request.Context(), cfg.OAuth)
		if err != nil {
			s.configurationError(c, err)
			return
		}
	}
	st := active.store.WithRegistration(cfg.AllowRegistration)
	next := &Server{
		cfg: cfg, store: st, authenticator: auth.New(st, cfg.SessionTTL), oauth: provider,
		groupPolicy: cfg.Groups, mailer: credentials.Resend{APIKey: cfg.ResendKey, From: cfg.ResendFrom},
		catalog: active.catalog, log: active.log, template: active.template, runtime: active.runtime,
	}
	next.router()
	s.runtime.current.Store(next)
	c.JSON(200, gin.H{"restart_required": restart})
}

func (s *Server) configurationError(c *gin.Context, err error) {
	if errors.Is(err, config.ErrInvalidSetting) {
		oauthError(c, 400, "configuration_invalid")
		return
	}
	s.log.Error("configuration operation failed", "error", err)
	oauthError(c, 502, "configuration_failed")
}
