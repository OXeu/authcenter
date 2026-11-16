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

func mountDocs(r *gin.Engine) {
	r.GET("/docs", func(c *gin.Context) { c.Redirect(http.StatusMovedPermanently, "/docs/") })
	r.GET("/docs/*page", serveDocs)
}

func serveDocs(c *gin.Context) {
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
	page := template.Must(template.New("docs").Parse(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Configcenter 接入文档</title><link rel="stylesheet" href="/static/docs.css"></head><body><header><a href="/docs/">Configcenter 文档</a><nav aria-label="文档导航"><a href="/docs/api">HTTP API</a><a href="/docs/sdk">Go SDK</a><a href="/projects">控制台</a></nav></header><main>{{.}}</main></body></html>`))
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := page.Execute(c.Writer, template.HTML(body.String())); err != nil { //nolint:gosec // Only embedded documentation is rendered; Goldmark disables raw HTML.
		c.Abort()
	}
}
