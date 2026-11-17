package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/OXeu/configcenter/internal/auth"
	"github.com/OXeu/configcenter/internal/slug"
	"github.com/OXeu/configcenter/internal/store"
)

const (
	jsonRPCVersion = "2.0"
	mcpProtocol    = "2025-06-18"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResult struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternal       = -32603
)

// MountMCP serves a Streamable-HTTP MCP endpoint per project.
func (s *Server) MountMCP(r *gin.Engine) {
	r.POST("/mcp/:project", s.authenticator.BearerMiddleware(), s.mcpHandle)
}

func (s *Server) mcpHandle(c *gin.Context) {
	var req rpcRequest
	if err := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)).Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, rpcResult{JSONRPC: jsonRPCVersion, Error: &rpcError{Code: codeParseError, Message: "parse error"}})
		return
	}
	if req.JSONRPC != jsonRPCVersion || req.Method == "" {
		c.JSON(http.StatusOK, rpcResult{JSONRPC: jsonRPCVersion, ID: req.ID, Error: &rpcError{Code: codeInvalidRequest, Message: "invalid request"}})
		return
	}
	if len(req.ID) == 0 {
		c.Status(http.StatusAccepted)
		return
	}
	result, rpcErr := s.dispatch(c, req)
	if rpcErr != nil {
		c.JSON(http.StatusOK, rpcResult{JSONRPC: jsonRPCVersion, ID: req.ID, Error: rpcErr})
		return
	}
	c.JSON(http.StatusOK, rpcResult{JSONRPC: jsonRPCVersion, ID: req.ID, Result: result})
}

func (s *Server) dispatch(c *gin.Context, req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return gin.H{
			"protocolVersion": mcpProtocol,
			"capabilities":    gin.H{"tools": gin.H{"listChanged": false}},
			"serverInfo":      gin.H{"name": "configcenter", "version": "1.0.0"},
		}, nil
	case "ping":
		return gin.H{}, nil
	case "tools/list":
		return gin.H{"tools": s.mcpTools(c)}, nil
	case "tools/call":
		var call struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &call); err != nil {
			return nil, &rpcError{Code: codeInvalidParams, Message: "invalid params"}
		}
		return s.callTool(c, call.Name, call.Arguments)
	case "resources/list", "prompts/list":
		return gin.H{"resources": []any{}, "prompts": []any{}}, nil
	default:
		return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found"}
	}
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func schema(props map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

func (s *Server) mcpTools(c *gin.Context) []mcpTool {
	availableTools := []mcpTool{
		{
			Name:        "list_environments",
			Description: "List the environments of this project.",
			InputSchema: schema(map[string]any{}),
		},
		{
			Name:        "list_config",
			Description: "List configuration items of an environment, optionally filtered by key prefix.",
			InputSchema: schema(map[string]any{
				"environment": map[string]any{"type": "string", "description": "Environment slug."},
				"prefix":      map[string]any{"type": "string", "description": "Only return keys starting with this prefix."},
			}, "environment"),
		},
		{
			Name:        "get_config",
			Description: "Read one configuration item.",
			InputSchema: schema(map[string]any{
				"environment": map[string]any{"type": "string"},
				"key":         map[string]any{"type": "string"},
			}, "environment", "key"),
		},
		{
			Name:        "list_proposals",
			Description: "List pending change proposals awaiting human review.",
			InputSchema: schema(map[string]any{}),
		},
	}
	availableTools = append(availableTools,
		mcpTool{
			Name:        "set_config",
			Description: "Create or update a configuration item. Requires config:write and applies immediately.",
			InputSchema: schema(map[string]any{
				"environment": map[string]any{"type": "string"},
				"key":         map[string]any{"type": "string"},
				"value":       map[string]any{"type": "string"},
				"description": map[string]any{"type": "string"},
			}, "environment", "key", "value"),
		},
		mcpTool{
			Name:        "delete_config",
			Description: "Delete a configuration item immediately.",
			InputSchema: schema(map[string]any{
				"environment": map[string]any{"type": "string"},
				"key":         map[string]any{"type": "string"},
			}, "environment", "key"),
		},
		mcpTool{
			Name:        "propose_config",
			Description: "Propose a create, update or delete that a human must review in the web UI.",
			InputSchema: schema(map[string]any{
				"environment": map[string]any{"type": "string"},
				"key":         map[string]any{"type": "string"},
				"action":      map[string]any{"type": "string", "enum": []string{store.ActionSet, store.ActionDelete}},
				"value":       map[string]any{"type": "string"},
				"reason":      map[string]any{"type": "string"},
			}, "environment", "key", "action"),
		},
	)
	id, _ := auth.FromContext(c.Request.Context())
	tools := make([]mcpTool, 0, len(availableTools))
	for _, tool := range availableTools {
		if mcpAllowed(id, tool.Name) {
			tools = append(tools, tool)
		}
	}
	return tools
}

type toolArgs struct {
	Environment string `json:"environment"`
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Prefix      string `json:"prefix"`
	Action      string `json:"action"`
	Reason      string `json:"reason"`
}

func textResult(v any) (any, *rpcError) {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, &rpcError{Code: codeInternal, Message: "encode result"}
	}
	return gin.H{"content": []gin.H{{"type": "text", "text": string(body)}}, "isError": false}, nil
}

func toolError(msg string) (any, *rpcError) {
	return gin.H{"content": []gin.H{{"type": "text", "text": msg}}, "isError": true}, nil
}

func (s *Server) callTool(c *gin.Context, name string, raw map[string]any) (any, *rpcError) {
	var args toolArgs
	if raw != nil {
		encoded, err := json.Marshal(raw)
		if err != nil {
			return nil, &rpcError{Code: codeInvalidParams, Message: "invalid arguments"}
		}
		if err := json.Unmarshal(encoded, &args); err != nil {
			return nil, &rpcError{Code: codeInvalidParams, Message: "invalid arguments"}
		}
	}
	id, _ := auth.FromContext(c.Request.Context())
	if id.Project == nil {
		return nil, &rpcError{Code: codeInvalidRequest, Message: "api key required"}
	}
	if permission := mcpPermission(name); permission != "" && !mcpAllowed(id, name) {
		return toolError("insufficient api key scope")
	}
	ctx := c.Request.Context()

	switch name {
	case "list_environments":
		envs, err := s.store.ListEnvironments(ctx, id.Project.ID)
		if err != nil {
			return toolError(err.Error())
		}
		return textResult(gin.H{"project": id.Project.Slug, "environments": envView(envs)})
	case "list_config", "get_config", "set_config", "delete_config", "propose_config":
		env, err := s.store.EnvironmentBySlug(ctx, id.Project.ID, args.Environment)
		if err != nil {
			return toolError(err.Error())
		}
		return s.configTool(c, id, env, name, args)
	case "list_proposals":
		pending, err := s.store.ListProposals(ctx, id.Project.ID, store.StatusPending)
		if err != nil {
			return toolError(err.Error())
		}
		return textResult(gin.H{"proposals": proposalView(pending)})
	default:
		return nil, &rpcError{Code: codeMethodNotFound, Message: "unknown tool"}
	}
}

func (s *Server) configTool(c *gin.Context, id auth.Identity, env store.Environment, name string, args toolArgs) (any, *rpcError) {
	ctx := c.Request.Context()
	projectID := id.Project.ID
	switch name {
	case "set_config":
		if err := slug.ValidateKey(args.Key); err != nil {
			return toolError(err.Error())
		}
		item, err := s.store.PutItem(ctx, store.ConfigItem{
			ProjectID: projectID, EnvironmentID: env.ID, Key: args.Key, Value: args.Value,
			Description: args.Description, UpdatedBy: id.APIKey.CreatedBy,
		})
		if err != nil {
			return toolError(err.Error())
		}
		return textResult(toItemJSON(item))
	case "propose_config":
		if err := validateAction(args.Action); err != nil {
			return toolError("action must be set or delete")
		}
		proposal := store.MCPProposal{
			ProjectID: projectID, EnvironmentID: env.ID, Key: args.Key, Action: args.Action,
			Reason: args.Reason, Status: store.StatusPending, ProposedBy: id.APIKey.CreatedBy,
			APIKeyID: id.APIKey.ID, CreatedAt: time.Now().UTC(),
		}
		if args.Action == store.ActionSet {
			value := args.Value
			proposal.Value = &value
		}
		created, err := s.store.CreateProposal(ctx, proposal)
		if err != nil {
			return toolError(err.Error())
		}
		return textResult(gin.H{"id": created.ID, "key": created.Key, "action": created.Action, "status": created.Status})
	case "get_config":
		item, err := s.store.GetItem(ctx, projectID, env.ID, args.Key)
		if errors.Is(err, store.ErrNotFound) {
			return toolError("key not found")
		}
		if err != nil {
			return toolError(err.Error())
		}
		return textResult(toItemJSON(item))
	case "delete_config":
		if err := slug.ValidateKey(args.Key); err != nil {
			return toolError(err.Error())
		}
		if err := s.store.DeleteItem(ctx, projectID, env.ID, args.Key); err != nil {
			return toolError(err.Error())
		}
		return textResult(gin.H{"deleted": args.Key})
	default:
		items, err := s.store.ListItems(ctx, projectID, env.ID, args.Prefix)
		if err != nil {
			return toolError(err.Error())
		}
		return textResult(gin.H{"environment": env.Slug, "items": itemView(items)})
	}
}

func validateAction(action string) error {
	switch action {
	case store.ActionSet, store.ActionDelete:
		return nil
	default:
		return fmt.Errorf("unknown action %q", action)
	}
}

func envView(envs []store.Environment) []envJSON {
	out := make([]envJSON, 0, len(envs))
	for _, e := range envs {
		out = append(out, toEnvJSON(e))
	}
	return out
}

func itemView(items []store.ConfigItem) []itemJSON {
	out := make([]itemJSON, 0, len(items))
	for _, i := range items {
		out = append(out, toItemJSON(i))
	}
	return out
}

func proposalView(ps []store.MCPProposal) []gin.H {
	out := make([]gin.H, 0, len(ps))
	for _, p := range ps {
		out = append(out, gin.H{"id": p.ID, "environment_id": p.EnvironmentID, "key": p.Key, "action": p.Action, "value": p.Value, "reason": p.Reason, "status": p.Status})
	}
	return out
}

func mcpPermission(name string) store.Permission {
	switch name {
	case "list_environments":
		return store.EnvironmentsRead
	case "list_config", "get_config":
		return store.ConfigRead
	case "list_proposals":
		return store.ProposalsRead
	case "set_config":
		return store.ConfigWrite
	case "delete_config":
		return store.ConfigDelete
	case "propose_config":
		return store.ProposalsWrite
	default:
		return ""
	}
}

func mcpAllowed(id auth.Identity, name string) bool {
	permission := mcpPermission(name)
	if id.Project == nil || permission == "" {
		return false
	}
	if id.APIKey == nil && !strings.HasSuffix(string(permission), ":read") {
		return false
	}
	return id.Allows(id.Project.ID, permission)
}
