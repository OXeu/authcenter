package server

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/OXeu/configcenter/internal/auth"
	"github.com/OXeu/configcenter/internal/slug"
	"github.com/OXeu/configcenter/internal/store"
	"github.com/gin-gonic/gin"
)

func (s *Server) apiInitializeItems(c *gin.Context, project store.Project) {
	env, ok := s.resolveEnv(c, project)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var input struct {
		Defaults map[string]string `json:"defaults"`
	}
	if err := bindJSON(c, &input); err != nil {
		s.fail(c, err)
		return
	}
	if len(input.Defaults) == 0 || len(input.Defaults) > 500 {
		s.fail(c, fmt.Errorf("%w: defaults must contain 1 to 500 keys", errBadRequest))
		return
	}
	keys := make([]string, 0, len(input.Defaults))
	for key := range input.Defaults {
		if err := slug.ValidateKey(key); err != nil {
			s.fail(c, fmt.Errorf("key %q: %w", key, err))
			return
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	identity, _ := auth.FromContext(c.Request.Context())
	items := make([]store.ConfigItem, 0, len(keys))
	for _, key := range keys {
		items = append(items, store.ConfigItem{ProjectID: project.ID, EnvironmentID: env.ID, Key: key, Value: input.Defaults[key], UpdatedBy: identity.ActorID()})
	}
	if err := s.store.InitializeItems(c.Request.Context(), items); err != nil {
		s.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
