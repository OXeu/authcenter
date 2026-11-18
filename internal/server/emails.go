package server

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/OXeu/authcenter/internal/store"
)

func (s *Server) setPrimaryEmail(c *gin.Context)    { s.manageAccountEmail(c, true) }
func (s *Server) removeAccountEmail(c *gin.Context) { s.manageAccountEmail(c, false) }

func (s *Server) manageAccountEmail(c *gin.Context, primary bool) {
	id, err := userID(c)
	if err != nil {
		s.fail(c, err)
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	user, _ := s.me(c)
	if !s.verifyAccountPassword(c, user, in.Password) {
		return
	}
	if primary {
		err = s.store.SetPrimaryEmail(c.Request.Context(), user, id, sessionID(c))
	} else {
		err = s.store.RemoveAccountEmail(c.Request.Context(), user, id, sessionID(c))
	}
	switch {
	case errors.Is(err, store.ErrPrimaryEmail):
		oauthError(c, 400, "primary_email_required")
	case errors.Is(err, store.ErrInvalidGrant):
		oauthError(c, 401, "invalid_credentials")
	case err != nil:
		s.fail(c, err)
	default:
		c.Status(204)
	}
}
