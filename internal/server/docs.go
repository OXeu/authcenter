package server

import (
	"bytes"
	"html/template"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"github.com/xeu/configcenter"
)

func (s *Server) mountDocs(r *gin.Engine) {
	r.GET("/docs", func(c *gin.Context) { c.Redirect(http.StatusMovedPermanently, "/docs/") })
	r.GET("/docs/*page", s.serveDocs)
}

func (s *Server) serveDocs(c *gin.Context) {
	name := strings.TrimPrefix(c.Param("page"), "/")
	raw := strings.HasSuffix(name, ".md")
	name = strings.TrimSuffix(name, ".md")
	if name == "" {
		name = "index"
	}
	switch name {
	case "index", "api", "sdk":
	default:
		c.Status(http.StatusNotFound)
		return
	}
	source, err := configcenter.Documentation.ReadFile("docs/" + name + ".md")
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	if raw {
		c.Data(http.StatusOK, "text/markdown; charset=utf-8", source)
		return
	}
	var body bytes.Buffer
	if err := goldmark.New(goldmark.WithExtensions(extension.Table)).Convert(source, &body); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	s.render(c, http.StatusOK, gin.H{
		"Page": "docs", "Title": "nav.docs", "DocPage": name,
		"DocumentHTML": template.HTML(body.String()), //nolint:gosec // Only embedded documentation is rendered; Goldmark disables raw HTML.
	})
}
