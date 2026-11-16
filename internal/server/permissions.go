package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/xeu/configcenter/internal/apikey"
	"github.com/xeu/configcenter/internal/store"
)

func (s *Server) allowProject(c *gin.Context, project store.Project) bool {
	me, ok := s.me(c)
	if !ok || (me.Role != store.RoleAdmin && project.OwnerID != me.ID) {
		c.AbortWithStatus(http.StatusForbidden)
		return false
	}
	return true
}

func (s *Server) apiDeleteUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	me, _ := s.me(c)
	if err != nil || id == 0 || uint(id) == me.ID {
		c.Status(http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteUser(c.Request.Context(), uint(id), me.ID); err != nil {
		s.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) apiRotateKey(c *gin.Context, project store.Project) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.Status(http.StatusBadRequest)
		return
	}
	token, digest, err := apikey.Generate()
	if err != nil {
		s.fail(c, err)
		return
	}
	if err := s.store.RotateAPIKey(c.Request.Context(), project.ID, uint(id), digest[:], apikey.Display(token)); err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
}
