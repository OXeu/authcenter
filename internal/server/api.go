package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xeu/configcenter/internal/apikey"
	"github.com/xeu/configcenter/internal/auth"
	"github.com/xeu/configcenter/internal/slug"
	"github.com/xeu/configcenter/internal/store"
)

// MountAPI registers the JSON API used by the web UI and by operators.
func (s *Server) MountAPI(r *gin.Engine) {
	ui := r.Group("/api/v1", s.authenticator.BrowserMiddleware())
	ui.GET("/me", s.apiMe)
	ui.GET("/projects", s.apiListProjects)
	ui.POST("/projects", auth.RequireWriter(), s.apiCreateProject)
	ui.PATCH("/projects/:project", auth.RequireWriter(), s.apiUpdateProject)
	ui.DELETE("/projects/:project", auth.RequireWriter(), s.apiDeleteProject)
	ui.GET("/projects/:project/environments", s.withProject(s.apiListEnvironments))
	ui.POST("/projects/:project/environments", auth.RequireWriter(), s.withProject(s.apiCreateEnvironment))
	ui.DELETE("/projects/:project/environments/:env", auth.RequireWriter(), s.withProject(s.apiDeleteEnvironment))
	ui.GET("/projects/:project/environments/:env/items", s.withProject(s.apiListItems))
	ui.PUT("/projects/:project/environments/:env/items/:key", auth.RequireWriter(), s.withProject(s.apiPutItem))
	ui.DELETE("/projects/:project/environments/:env/items/:key", auth.RequireWriter(), s.withProject(s.apiDeleteItem))
	ui.GET("/projects/:project/keys", s.withProject(s.apiListKeys))
	ui.POST("/projects/:project/keys", auth.RequireWriter(), s.withProject(s.apiCreateKey))
	ui.DELETE("/projects/:project/keys/:id", auth.RequireWriter(), s.withProject(s.apiDeleteKey))
	ui.GET("/projects/:project/proposals", s.withProject(s.apiListProposals))
	ui.POST("/projects/:project/proposals/:id/review", auth.RequireWriter(), s.withProject(s.apiReviewProposal))
	ui.GET("/users", auth.RequireAdmin(), s.apiListUsers)
	ui.PATCH("/users/:id", auth.RequireAdmin(), s.apiSetRole)

	key := r.Group("/api/v1/agent", s.authenticator.BearerMiddleware())
	key.GET("/project", s.apiProgrammaticProject)
	key.GET("/environments/:env/values", s.apiProgrammaticItems)
}

// withProject resolves the :project route parameter before the handler runs.
func (s *Server) withProject(fn func(*gin.Context, store.Project)) gin.HandlerFunc {
	return func(c *gin.Context) {
		project, err := s.store.ProjectBySlug(c.Request.Context(), c.Param("project"))
		if err != nil {
			s.fail(c, err)
			return
		}
		c.Set("cc_project", project)
		fn(c, project)
	}
}

func (s *Server) fail(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, store.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, slug.ErrInvalid), errors.Is(err, slug.ErrInvalidKey), errors.Is(err, errBadRequest):
		status = http.StatusBadRequest
	}
	if status == http.StatusInternalServerError {
		s.log.Error("request failed", "path", c.Request.URL.Path, "error", err)
		c.JSON(status, gin.H{"error": "internal error"})
		return
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

var errBadRequest = errors.New("bad request")

func bindJSON(c *gin.Context, dst any) error {
	if err := c.ShouldBindJSON(dst); err != nil {
		return fmt.Errorf("%w: %w", errBadRequest, err)
	}
	return nil
}

type projectJSON struct {
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

func toProjectJSON(p store.Project) projectJSON {
	return projectJSON{Slug: p.Slug, Name: p.Name, Description: p.Description, CreatedAt: p.CreatedAt}
}

type envJSON struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

func toEnvJSON(e store.Environment) envJSON {
	return envJSON{Slug: e.Slug, Name: e.Name}
}

type itemJSON struct {
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	Description string    `json:"description"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func toItemJSON(i store.ConfigItem) itemJSON {
	return itemJSON{Key: i.Key, Value: i.Value, Description: i.Description, UpdatedAt: i.UpdatedAt}
}

type keyJSON struct {
	ID         uint       `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Role       store.Role `json:"role"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

func toKeyJSON(k store.APIKey) keyJSON {
	return keyJSON{ID: k.ID, Name: k.Name, Prefix: k.Prefix, Role: k.Role, CreatedAt: k.CreatedAt, LastUsedAt: k.LastUsedAt, ExpiresAt: k.ExpiresAt}
}

// apiMe returns the signed-in user and the CSRF token of the session.
func (s *Server) apiMe(c *gin.Context) {
	id, ok := auth.FromContext(c.Request.Context())
	if !ok || id.User == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"email": id.User.Email, "name": id.User.Name, "avatar_url": id.User.AvatarURL, "role": id.Role, "csrf": auth.CSRF(c)})
}

func (s *Server) apiListProjects(c *gin.Context) {
	projects, err := s.store.ListProjects(c.Request.Context())
	if err != nil {
		s.fail(c, err)
		return
	}
	out := make([]projectJSON, 0, len(projects))
	for _, p := range projects {
		out = append(out, toProjectJSON(p))
	}
	c.JSON(http.StatusOK, gin.H{"projects": out})
}

type projectInput struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Server) apiCreateProject(c *gin.Context) {
	var in projectInput
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	if err := slug.Validate(in.Slug); err != nil {
		s.fail(c, err)
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 128 {
		s.fail(c, fmt.Errorf("%w: name is required", errBadRequest))
		return
	}
	now := time.Now().UTC()
	created, err := s.store.CreateProject(
		c.Request.Context(),
		store.Project{Slug: in.Slug, Name: name, Description: in.Description, CreatedAt: now, UpdatedAt: now},
		store.Environment{Slug: "production", Name: "Production", CreatedAt: now},
	)
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, toProjectJSON(created))
}

func (s *Server) apiUpdateProject(c *gin.Context) {
	var in projectInput
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	project, err := s.store.ProjectBySlug(c.Request.Context(), c.Param("project"))
	if err != nil {
		s.fail(c, err)
		return
	}
	if err := s.store.UpdateProject(c.Request.Context(), project.ID, in.Name, in.Description); err != nil {
		s.fail(c, err)
		return
	}
	project.Name, project.Description = in.Name, in.Description
	c.JSON(http.StatusOK, toProjectJSON(project))
}

func (s *Server) apiDeleteProject(c *gin.Context) {
	project, err := s.store.ProjectBySlug(c.Request.Context(), c.Param("project"))
	if err != nil {
		s.fail(c, err)
		return
	}
	if err := s.store.DeleteProject(c.Request.Context(), project.ID); err != nil {
		s.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) apiListEnvironments(c *gin.Context, project store.Project) {
	envs, err := s.store.ListEnvironments(c.Request.Context(), project.ID)
	if err != nil {
		s.fail(c, err)
		return
	}
	out := make([]envJSON, 0, len(envs))
	for _, e := range envs {
		out = append(out, toEnvJSON(e))
	}
	c.JSON(http.StatusOK, gin.H{"environments": out})
}

func (s *Server) apiCreateEnvironment(c *gin.Context, project store.Project) {
	var in envJSON
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	if err := slug.Validate(in.Slug); err != nil {
		s.fail(c, err)
		return
	}
	created, err := s.store.CreateEnvironment(c.Request.Context(), store.Environment{
		ProjectID: project.ID, Slug: in.Slug, Name: in.Name, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, toEnvJSON(created))
}

func (s *Server) apiDeleteEnvironment(c *gin.Context, project store.Project) {
	env, err := s.store.EnvironmentBySlug(c.Request.Context(), project.ID, c.Param("env"))
	if err != nil {
		s.fail(c, err)
		return
	}
	if err := s.store.DeleteEnvironment(c.Request.Context(), project.ID, env.ID); err != nil {
		s.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) resolveEnv(c *gin.Context, project store.Project) (store.Environment, bool) {
	env, err := s.store.EnvironmentBySlug(c.Request.Context(), project.ID, c.Param("env"))
	if err != nil {
		s.fail(c, err)
		return store.Environment{}, false
	}
	return env, true
}

func (s *Server) itemsResponse(c *gin.Context, project store.Project, env store.Environment, items []store.ConfigItem) {
	out := make([]itemJSON, 0, len(items))
	for _, item := range items {
		out = append(out, toItemJSON(item))
	}
	c.JSON(http.StatusOK, gin.H{"project": project.Slug, "environment": env.Slug, "items": out})
}

func (s *Server) apiListItems(c *gin.Context, project store.Project) {
	env, ok := s.resolveEnv(c, project)
	if !ok {
		return
	}
	items, err := s.store.ListItems(c.Request.Context(), project.ID, env.ID, c.Query("prefix"))
	if err != nil {
		s.fail(c, err)
		return
	}
	s.itemsResponse(c, project, env, items)
}

type itemInput struct {
	Key         string  `json:"key"`
	Value       *string `json:"value"`
	Description string  `json:"description"`
}

func (s *Server) apiPutItem(c *gin.Context, project store.Project) {
	env, ok := s.resolveEnv(c, project)
	if !ok {
		return
	}
	var in itemInput
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	key := c.Param("key")
	if err := slug.ValidateKey(key); err != nil {
		s.fail(c, err)
		return
	}
	if in.Key != "" && in.Key != key {
		if err := s.store.RenameItem(c.Request.Context(), project.ID, env.ID, key, in.Key); err != nil && !errors.Is(err, store.ErrNotFound) {
			s.fail(c, err)
			return
		}
		key = in.Key
	}
	if in.Value == nil {
		s.fail(c, fmt.Errorf("%w: value is required", errBadRequest))
		return
	}
	id, _ := auth.FromContext(c.Request.Context())
	var userID uint
	if id.User != nil {
		userID = id.User.ID
	}
	item, err := s.store.PutItem(c.Request.Context(), store.ConfigItem{
		ProjectID: project.ID, EnvironmentID: env.ID, Key: key, Value: *in.Value,
		Description: in.Description, UpdatedBy: userID,
	})
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, toItemJSON(item))
}

func (s *Server) apiDeleteItem(c *gin.Context, project store.Project) {
	env, ok := s.resolveEnv(c, project)
	if !ok {
		return
	}
	if err := s.store.DeleteItem(c.Request.Context(), project.ID, env.ID, c.Param("key")); err != nil {
		s.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) apiListKeys(c *gin.Context, project store.Project) {
	keys, err := s.store.ListAPIKeys(c.Request.Context(), project.ID)
	if err != nil {
		s.fail(c, err)
		return
	}
	out := make([]keyJSON, 0, len(keys))
	for _, k := range keys {
		out = append(out, toKeyJSON(k))
	}
	c.JSON(http.StatusOK, gin.H{"keys": out})
}

type keyInput struct {
	Name      string     `json:"name"`
	Role      store.Role `json:"role"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func (s *Server) apiCreateKey(c *gin.Context, project store.Project) {
	var in keyInput
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	if in.Role == "" {
		in.Role = store.RoleViewer
	}
	if !in.Role.Valid() {
		s.fail(c, fmt.Errorf("%w: invalid role", errBadRequest))
		return
	}
	token, digest, err := apikey.Generate()
	if err != nil {
		s.fail(c, err)
		return
	}
	id, _ := auth.FromContext(c.Request.Context())
	var userID uint
	if id.User != nil {
		userID = id.User.ID
	}
	key, err := s.store.CreateAPIKey(c.Request.Context(), store.APIKey{
		ProjectID: project.ID, Name: in.Name, Digest: digest[:], Prefix: apikey.Display(token),
		Role: in.Role, CreatedBy: userID, CreatedAt: time.Now().UTC(), ExpiresAt: in.ExpiresAt,
	})
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"key": toKeyJSON(key), "token": token})
}

func (s *Server) apiDeleteKey(c *gin.Context, project store.Project) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		s.fail(c, fmt.Errorf("%w: invalid key id", errBadRequest))
		return
	}
	if err := s.store.DeleteAPIKey(c.Request.Context(), project.ID, uint(id)); err != nil {
		s.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// apiProgrammaticProject lists the environments reachable with this API key.
func (s *Server) apiProgrammaticProject(c *gin.Context) {
	id, _ := auth.FromContext(c.Request.Context())
	if id.Project == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "api key required"})
		return
	}
	envs, err := s.store.ListEnvironments(c.Request.Context(), id.Project.ID)
	if err != nil {
		s.fail(c, fmt.Errorf("list environments: %w", err))
		return
	}
	out := make([]envJSON, 0, len(envs))
	for _, e := range envs {
		out = append(out, toEnvJSON(e))
	}
	c.JSON(http.StatusOK, gin.H{"project": toProjectJSON(*id.Project), "environments": out})
}

func (s *Server) apiProgrammaticItems(c *gin.Context) {
	id, _ := auth.FromContext(c.Request.Context())
	env, ok := s.resolveEnv(c, *id.Project)
	if !ok {
		return
	}
	items, err := s.store.ListItems(c.Request.Context(), id.Project.ID, env.ID, c.Query("prefix"))
	if err != nil {
		s.fail(c, err)
		return
	}
	s.itemsResponse(c, *id.Project, env, items)
}

type userJSON struct {
	AvatarURL   string     `json:"avatar_url"`
	ID          uint       `json:"id"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	Role        store.Role `json:"role"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

func (s *Server) apiListUsers(c *gin.Context) {
	users, err := s.store.ListUsers(c.Request.Context())
	if err != nil {
		s.fail(c, err)
		return
	}
	out := make([]userJSON, 0, len(users))
	for _, u := range users {
		out = append(out, userJSON{ID: u.ID, Name: u.Name, AvatarURL: u.AvatarURL, Email: u.Email, Role: u.Role, CreatedAt: u.CreatedAt, LastLoginAt: u.LastLoginAt})
	}
	c.JSON(http.StatusOK, gin.H{"users": out})
}

func (s *Server) apiSetRole(c *gin.Context) {
	var in struct {
		Role store.Role `json:"role"`
	}
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	if !in.Role.Valid() {
		s.fail(c, fmt.Errorf("%w: invalid role", errBadRequest))
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		s.fail(c, fmt.Errorf("%w: invalid user id", errBadRequest))
		return
	}
	if err := s.store.SetUserRole(c.Request.Context(), uint(id), in.Role); err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "role": in.Role})
}

type proposalJSON struct {
	ID          uint       `json:"id"`
	Environment string     `json:"environment"`
	Key         string     `json:"key"`
	Action      string     `json:"action"`
	Value       *string    `json:"value"`
	Reason      string     `json:"reason"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	ReviewedAt  *time.Time `json:"reviewed_at"`
}

func (s *Server) proposalViews(c *gin.Context, project store.Project, ps []store.MCPProposal) ([]proposalJSON, error) {
	envs, err := s.store.ListEnvironments(c.Request.Context(), project.ID)
	if err != nil {
		return nil, fmt.Errorf("list environments for proposals: %w", err)
	}
	byID := make(map[uint]string, len(envs))
	for _, e := range envs {
		byID[e.ID] = e.Slug
	}
	out := make([]proposalJSON, 0, len(ps))
	for _, p := range ps {
		out = append(out, proposalJSON{
			ID: p.ID, Environment: byID[p.EnvironmentID], Key: p.Key, Action: p.Action,
			Value: p.Value, Reason: p.Reason, Status: p.Status, CreatedAt: p.CreatedAt, ReviewedAt: p.ReviewedAt,
		})
	}
	return out, nil
}

func (s *Server) apiListProposals(c *gin.Context, project store.Project) {
	ps, err := s.store.ListProposals(c.Request.Context(), project.ID, c.Query("status"))
	if err != nil {
		s.fail(c, err)
		return
	}
	views, err := s.proposalViews(c, project, ps)
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"proposals": views})
}

func (s *Server) apiReviewProposal(c *gin.Context, project store.Project) {
	var in struct {
		Approve bool `json:"approve"`
	}
	if err := bindJSON(c, &in); err != nil {
		s.fail(c, err)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		s.fail(c, fmt.Errorf("%w: invalid proposal id", errBadRequest))
		return
	}
	me, _ := s.me(c)
	reviewed, err := s.store.ReviewProposal(c.Request.Context(), project.ID, uint(id), me.ID, in.Approve)
	if err != nil {
		s.fail(c, err)
		return
	}
	views, err := s.proposalViews(c, project, []store.MCPProposal{reviewed})
	if err != nil {
		s.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"proposal": views[0]})
}
