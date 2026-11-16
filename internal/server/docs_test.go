package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDocumentation(t *testing.T) {
	r := gin.New()
	mountDocs(r)
	for _, name := range []string{"index", "api", "sdk"} {
		for _, suffix := range []string{"", ".md"} {
			path := "/docs/" + name + suffix
			t.Run(path, func(t *testing.T) {
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, http.NoBody))
				contentType := "text/html"
				marker := "<h1>"
				if suffix == ".md" {
					contentType = "text/markdown"
					marker = "# "
				}
				if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), contentType) || !strings.Contains(rec.Body.String(), marker) {
					t.Fatalf("unexpected documentation response: %d %s", rec.Code, rec.Header().Get("Content-Type"))
				}
			})
		}
	}
	for _, path := range []string{"/docs/commits.md", "/docs/missing", "/docs/../README.md"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, http.NoBody))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status %d", path, rec.Code)
		}
	}
}
