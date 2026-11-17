package server

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/OXeu/configcenter/internal/auth"
	"github.com/OXeu/configcenter/internal/store"
)

// MountAgent serves the MCP endpoint and the API-key REST endpoints. Agents
// authenticate with a project or global API key; the web UI uses a browser
// session and CSRF token. The path selects the project within the key scope.
func (s *Server) MountAgent(r *gin.Engine) {
	r.POST("/mcp/:project", s.authenticator.AgentMiddleware(s), s.mcpHandle)
}

// MountUIAPI serves the reads and deletes the web UI needs for items, which
// the MCP tool set does not cover for humans.
func (s *Server) MountUIAPI(r *gin.Engine) {
	ui := r.Group("/api/v1/ui", auth.RequireUser())
	ui.GET("/:project/items/:env", s.withProject(s.apiListItems))
	ui.DELETE("/:project/items/:env/:key", auth.RequireWriter(), s.withProject(s.apiDeleteItem))
}

// SessionUser lets the MCP middleware resolve browser sessions.
func (s *Server) SessionUser(ctx context.Context, id string) (store.Session, store.User, error) {
	sess, user, err := s.store.SessionUser(ctx, id)
	if err != nil {
		return store.Session{}, store.User{}, fmt.Errorf("resolve session: %w", err)
	}
	return sess, user, nil
}
