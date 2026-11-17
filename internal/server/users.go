package server

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func userID(c *gin.Context) (uint, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("%w: invalid user id", errBadRequest)
	}
	return uint(id), nil
}

func (s *Server) apiBanUser(c *gin.Context) {
	var in struct {
		Until time.Time `json:"until"`
	}
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	if !in.Until.After(time.Now().UTC()) {
		s.fail(c, fmt.Errorf("%w: expiry must be in the future", errBadRequest))
		return
	}
	s.changeBan(c, &in.Until)
}

func (s *Server) apiUnbanUser(c *gin.Context) { s.changeBan(c, nil) }

func (s *Server) changeBan(c *gin.Context, until *time.Time) {
	id, err := userID(c)
	if err != nil {
		s.fail(c, err)
		return
	}
	actor, _ := s.me(c)
	if err := s.store.SetUserBan(c.Request.Context(), actor.ID, id, until); err != nil {
		s.fail(c, err)
		return
	}
	c.Status(204)
}

func (s *Server) apiDeleteUser(c *gin.Context) {
	id, err := userID(c)
	if err != nil {
		s.fail(c, err)
		return
	}
	actor, _ := s.me(c)
	if err := s.store.DeleteUser(c.Request.Context(), actor.ID, id); err != nil {
		s.fail(c, err)
		return
	}
	c.Status(204)
}
