package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/OXeu/authcenter/internal/auth"
	"github.com/OXeu/authcenter/internal/store"
)

// MountUI registers the server-rendered pages.
func (s *Server) MountUI(r *gin.Engine) {
	ui := r.Group("")
	ui.GET("/applications", auth.RequireUser(), s.applicationsPage)
	ui.GET("/keys", auth.RequireUser(), func(c *gin.Context) { s.render(c, 200, gin.H{"Page": "keys", "Title": "title.keys"}) })
	ui.GET("/account", auth.RequireUser(), func(c *gin.Context) { s.render(c, 200, gin.H{"Page": "account", "Title": "title.account"}) })
	ui.GET("/users", auth.RequireAdmin(), s.usersPage)
	ui.GET("/settings", auth.RequireAdmin(), func(c *gin.Context) { s.render(c, 200, gin.H{"Page": "settings", "Title": "title.settings"}) })
}

func (s *Server) me(c *gin.Context) (store.User, bool) {
	id, ok := auth.FromContext(c.Request.Context())
	if !ok || id.User == nil {
		return store.User{}, false
	}
	return *id.User, true
}

func (s *Server) applicationsPage(c *gin.Context) {
	s.render(c, http.StatusOK, gin.H{"Page": "applications", "Title": "title.applications"})
}

func (s *Server) usersPage(c *gin.Context) {
	me, ok := s.me(c)
	if !ok {
		c.Redirect(http.StatusFound, "/login")
		return
	}
	s.render(c, http.StatusOK, gin.H{
		"Page":    "users",
		"Title":   "title.users",
		"Me":      me,
		"IsAdmin": me.Role == store.RoleAdmin,
	})
}
