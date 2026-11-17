package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/OXeu/authcenter/internal/auth"
	"github.com/OXeu/authcenter/internal/store"
	"github.com/gin-gonic/gin"
)

// MountAPI registers session-authenticated management endpoints.
func (s *Server) MountAPI(r *gin.Engine) {
	api := r.Group("/api/v1", auth.RequireUser())
	api.GET("/me", s.apiMe)
	api.GET("/account/security", s.securitySettings)
	api.PUT("/account/password", s.setPassword)
	api.POST("/account/email", s.startEmailChange)
	api.POST("/account/email/verify", s.verifyEmailChange)
	api.DELETE("/account/totp", s.disableTOTP)
	api.GET("/grants", s.apiGrants)
	api.DELETE("/grants/:id", s.apiRevokeGrant)
	admin := api.Group("", auth.RequireAdmin())
	api.GET("/clients", s.apiClients)
	api.POST("/clients", s.apiCreateClient)
	api.PATCH("/clients/:id", s.apiUpdateClient)
	api.DELETE("/clients/:id", s.apiDeleteClient)
	admin.GET("/users", s.apiListUsers)
	admin.PATCH("/users/:id", s.apiSetRole)
	admin.PUT("/users/:id/ban", s.apiBanUser)
	admin.DELETE("/users/:id/ban", s.apiUnbanUser)
	admin.DELETE("/users/:id", s.apiDeleteUser)
	admin.GET("/settings", s.apiSettings)
	admin.PUT("/settings", s.apiSaveSettings)
	admin.POST("/settings/refresh", s.apiRefreshSettings)
}

func (s *Server) apiMe(c *gin.Context) {
	id, _ := auth.FromContext(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"email": id.User.Email, "name": id.User.Name, "avatar_url": id.User.AvatarURL, "role": id.Role, "csrf": auth.CSRF(c)})
}

func (s *Server) fail(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, store.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, errBadRequest):
		status = http.StatusBadRequest
	case errors.Is(err, store.ErrProtectedUser):
		status = http.StatusForbidden
	}
	if status == http.StatusInternalServerError {
		s.log.Error("request failed", "path", c.Request.URL.Path, "error", err)
		c.JSON(status, gin.H{"error": "internal error"})
		return
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

var errBadRequest = errors.New("bad request")

func bindJSON(c *gin.Context, dst any) error {
	if err := c.ShouldBindJSON(dst); err != nil {
		return fmt.Errorf("%w: %w", errBadRequest, err)
	}
	return nil
}

type userJSON struct {
	AvatarURL   string     `json:"avatar_url"`
	ID          uint       `json:"id"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	Role        store.Role `json:"role"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
	BannedUntil *time.Time `json:"banned_until"`
	Banned      bool       `json:"banned"`
}

func (s *Server) apiListUsers(c *gin.Context) {
	users, err := s.store.ListUsers(c.Request.Context())
	if err != nil {
		s.fail(c, err)
		return
	}
	out := make([]userJSON, 0, len(users))
	for _, u := range users {
		out = append(out, userJSON{ID: u.ID, Name: u.Name, AvatarURL: u.AvatarURL, Email: u.Email, Role: u.Role, CreatedAt: u.CreatedAt, LastLoginAt: u.LastLoginAt, BannedUntil: u.BannedUntil, Banned: !u.Active()})
	}
	c.JSON(http.StatusOK, gin.H{"users": out})
}

func (s *Server) apiSetRole(c *gin.Context) {
	var in struct {
		Role store.Role `json:"role"`
	}
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	if in.Role != store.RoleAdmin && in.Role != store.RoleUser {
		s.fail(c, fmt.Errorf("%w: invalid role", errBadRequest))
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		s.fail(c, fmt.Errorf("%w: invalid user id", errBadRequest))
		return
	}
	actor, _ := auth.FromContext(c.Request.Context())
	if uint(id) == actor.User.ID && in.Role != store.RoleAdmin {
		c.Status(http.StatusBadRequest)
		return
	}
	if uint(id) == actor.User.ID {
		c.Status(204)
		return
	}
	if err := s.store.ManageUserRole(c.Request.Context(), actor.User.ID, uint(id), in.Role); err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "role": in.Role})
}
