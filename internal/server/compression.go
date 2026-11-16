package server

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func acceptsGzip(value string) bool {
	wildcard := false
	for _, item := range strings.Split(value, ",") {
		parts := strings.Split(item, ";")
		coding := strings.TrimSpace(parts[0])
		quality := 1.0
		for _, parameter := range parts[1:] {
			key, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if ok && strings.EqualFold(key, "q") {
				q, err := strconv.ParseFloat(value, 64)
				if err != nil || !(q >= 0 && q <= 1) {
					quality = 0
				} else {
					quality = q
				}
			}
		}
		if strings.EqualFold(coding, "gzip") {
			return quality > 0
		}
		if coding == "*" {
			wildcard = quality > 0
		}
	}
	return wildcard
}

func serveStaticFile(c *gin.Context, filesystem fs.FS, name string) {
	contentType := ""
	switch path.Ext(name) {
	case ".js":
		contentType = "text/javascript; charset=utf-8"
	case ".css":
		contentType = "text/css; charset=utf-8"
	case ".svg":
		contentType = "image/svg+xml"
	case ".json":
		contentType = "application/json"
	}
	filename := name
	compressed := false
	if contentType != "" {
		c.Writer.Header().Add("Vary", "Accept-Encoding")
		compressed = acceptsGzip(c.GetHeader("Accept-Encoding")) && c.GetHeader("Range") == ""
		if compressed {
			filename += ".gz"
		}
	}
	data, err := fs.ReadFile(filesystem, filename)
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.Status(http.StatusNotFound)
		return
	}
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	c.Header("Content-Type", contentType)
	if compressed {
		c.Header("Content-Encoding", "gzip")
	}
	http.ServeContent(c.Writer, c.Request, name, time.Time{}, bytes.NewReader(data))
}
