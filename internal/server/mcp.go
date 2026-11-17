package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/OXeu/authcenter/internal/auth"
	"github.com/OXeu/authcenter/internal/store"
)

const mcpScope = "mcp"

type mcpGrantKey struct{}

func (s *Server) mcpResource() string { return s.cfg.PublicURL + "/mcp" }

func (s *Server) mountMCP(r *gin.Engine) {
	metadata := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"resource": s.mcpResource(), "resource_name": "AuthCenter MCP", "authorization_servers": []string{s.cfg.PublicURL}, "scopes_supported": []string{mcpScope, "offline_access"}, "bearer_methods_supported": []string{"header"}})
	}
	public := r.Group("", oauthPublic)
	for _, path := range []string{"/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-protected-resource"} {
		public.GET(path, metadata)
		public.OPTIONS(path, func(c *gin.Context) { c.Status(http.StatusNoContent) })
	}
	api := gin.New()
	s.MountAPI(api)
	userTools := s.mcpServer(api, false)
	adminTools := s.mcpServer(api, true)
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		id, _ := auth.FromContext(r.Context())
		if id.Role == store.RoleAdmin {
			return adminTools
		}
		return userTools
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 1 << 20, PropagateRequestCancellation: true})
	r.Any("/mcp", s.mcpAuthenticate, gin.WrapH(handler))
}

func (s *Server) mcpAuthenticate(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if origins := c.Request.Header.Values("Origin"); len(origins) > 1 || len(origins) == 1 && origins[0] != s.cfg.PublicURL {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid origin"})
		return
	}
	parts := strings.Fields(c.GetHeader("Authorization"))
	if len(c.Request.Header.Values("Authorization")) != 1 || len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		s.mcpChallenge(c, http.StatusUnauthorized, "invalid_token")
		return
	}
	grant, err := s.store.AccessTokenGrant(c.Request.Context(), digest(parts[1]))
	if err != nil || grant.Resource != s.mcpResource() {
		s.mcpChallenge(c, http.StatusUnauthorized, "invalid_token")
		return
	}
	if !slices.Contains(strings.Fields(grant.Scope), mcpScope) {
		s.mcpChallenge(c, http.StatusForbidden, "insufficient_scope")
		return
	}
	user, err := s.store.UserByID(c.Request.Context(), grant.UserID)
	if err != nil || !user.Active() {
		s.mcpChallenge(c, http.StatusUnauthorized, "invalid_token")
		return
	}
	ctx := auth.WithIdentity(c.Request.Context(), auth.Identity{User: &user, Role: user.Role})
	c.Request = c.Request.WithContext(context.WithValue(ctx, mcpGrantKey{}, grant.ID))
	c.Next()
}

func (s *Server) mcpChallenge(c *gin.Context, status int, code string) {
	c.Header("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata=%q, scope=%q, error=%q`, s.cfg.PublicURL+"/.well-known/oauth-protected-resource/mcp", mcpScope, code))
	c.AbortWithStatusJSON(status, gin.H{"error": code})
}

func (s *Server) mcpServer(api http.Handler, admin bool) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "authcenter", Version: "1.0.0"}, nil)
	for _, op := range s.apiOperations() {
		if op.Permission.AdminOnly() && !admin {
			continue
		}
		mcp.AddTool(server, &mcp.Tool{Name: op.Name, Description: op.Description, InputSchema: op.Schema, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: op.Method == http.MethodGet}}, func(ctx context.Context, _ *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, apiToolResult, error) {
			result, err := executeAPITool(ctx, api, op, input)
			return &mcp.CallToolResult{IsError: result.Status >= 400}, result, err
		})
	}
	return server
}

type apiToolResult struct {
	Status int `json:"http_status"`
	Body   any `json:"body,omitempty"`
}

func executeAPITool(ctx context.Context, api http.Handler, op apiOperation, input map[string]any) (apiToolResult, error) {
	body := maps.Clone(input)
	path := "/api/v1" + op.Path
	if strings.Contains(path, ":id") {
		id, ok := input["id"].(string)
		if !ok {
			return apiToolResult{}, fmt.Errorf("id must be a string")
		}
		path = strings.Replace(path, ":id", id, 1)
		delete(body, "id")
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return apiToolResult{}, fmt.Errorf("encode tool arguments: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, op.Method, path, bytes.NewReader(payload))
	if err != nil {
		return apiToolResult{}, fmt.Errorf("create tool request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response := &apiResponse{header: make(http.Header), status: http.StatusOK}
	api.ServeHTTP(response, request)
	result := apiToolResult{Status: response.status}
	if len(response.body) > 0 {
		if err := json.Unmarshal(response.body, &result.Body); err != nil {
			return apiToolResult{}, fmt.Errorf("decode API response: %w", err)
		}
	}
	return result, nil
}

type apiResponse struct {
	header http.Header
	status int
	body   []byte
}

// Header returns buffered API response headers.
func (r *apiResponse) Header() http.Header { return r.header }

// WriteHeader records the API response status.
func (r *apiResponse) WriteHeader(status int) { r.status = status }

// Write buffers the API response body.
func (r *apiResponse) Write(body []byte) (int, error) {
	r.body = append(r.body, body...)
	return len(body), nil
}
