package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/xeu/configcenter/internal/store"
)

func (s *Server) globalKeyHandler(handler func(*gin.Context, store.Project)) gin.HandlerFunc {
	return func(c *gin.Context) {
		handler(c, store.Project{})
	}
}

func (s *Server) globalKeysPage(c *gin.Context) {
	s.render(c, http.StatusOK, gin.H{"Page": "globalKeys", "Title": "title.globalKeys"})
}
