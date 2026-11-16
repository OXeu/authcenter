package server

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/fs"
	"mime"
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

func gzipBytes(data []byte) ([]byte, error) {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(data); err != nil {
		return nil, fmt.Errorf("compress asset: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("finish compressed asset: %w", err)
	}
	return buffer.Bytes(), nil
}

func serveStaticFile(c *gin.Context, filesystem fs.FS, name string) {
	data, err := fs.ReadFile(filesystem, name)
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.Status(http.StatusNotFound)
		return
	}
	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	c.Header("Content-Type", contentType)
	switch path.Ext(name) {
	case ".js", ".css", ".svg", ".json":
		c.Writer.Header().Add("Vary", "Accept-Encoding")
		if acceptsGzip(c.GetHeader("Accept-Encoding")) && c.GetHeader("Range") == "" {
			compressed, err := gzipBytes(data)
			if err != nil {
				c.Header("Cache-Control", "no-store")
				c.Status(http.StatusInternalServerError)
				return
			}
			c.Header("Content-Encoding", "gzip")
			data = compressed
		}
	}
	http.ServeContent(c.Writer, c.Request, name, time.Time{}, bytes.NewReader(data))
}
