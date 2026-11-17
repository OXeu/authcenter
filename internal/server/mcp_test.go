package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestMCPMethodsAndPermissions(t *testing.T) {
	f := newFlows(t)
	status, _ := f.do(http.MethodPost, "/api/v1/projects", map[string]any{"slug": "mcp-test", "name": "MCP"})
	if status != http.StatusCreated {
		t.Fatal("create project failed")
	}
	token, _ := issueKey(t, f, "/api/v1/projects/mcp-test/keys", map[string]any{"name": "reader", "role": "viewer"})
	for _, method := range []string{"initialize", "ping", "tools/list", "resources/list", "prompts/list"} {
		rec := keyRequest(t, f.router, token, http.MethodPost, "/mcp/mcp-test", map[string]any{"jsonrpc": "2.0", "id": 7, "method": method}, http.StatusOK)
		var reply struct {
			ID     int             `json:"id"`
			Error  *rpcError       `json:"error"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
			t.Fatal(err)
		}
		if reply.ID != 7 || reply.Error != nil {
			t.Fatalf("%s did not return a successful matching response", method)
		}
		if method == "initialize" {
			var result struct {
				Protocol string `json:"protocolVersion"`
			}
			if err := json.Unmarshal(reply.Result, &result); err != nil || result.Protocol != mcpProtocol {
				t.Fatal("unexpected protocol version")
			}
		}
		if method == "tools/list" {
			var result struct {
				Tools []mcpTool `json:"tools"`
			}
			if err := json.Unmarshal(reply.Result, &result); err != nil || len(result.Tools) != 4 {
				t.Fatal("reader should only discover four read tools")
			}
		}
	}
	for _, tool := range []string{"set_config", "delete_config", "propose_config"} {
		body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{
			"name": tool, "arguments": map[string]any{"environment": "production", "key": "db.pool", "value": "20", "action": "set"},
		}}
		rec := keyRequest(t, f.router, token, http.MethodPost, "/mcp/mcp-test", body, http.StatusOK)
		var reply struct {
			Result struct {
				IsError bool `json:"isError"`
			} `json:"result"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil || !reply.Result.IsError {
			t.Fatalf("viewer could invoke %s", tool)
		}
		status, sessionReply := f.do(http.MethodPost, "/mcp/mcp-test", body)
		result, ok := sessionReply["result"].(map[string]any)
		if status != http.StatusOK || !ok || result["isError"] != true {
			t.Fatalf("browser write %s must fail without panicking", tool)
		}
	}
	notification := keyRequest(t, f.router, token, http.MethodPost, "/mcp/mcp-test", map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}, http.StatusAccepted)
	if notification.Body.Len() != 0 {
		t.Fatal("notifications should have no response body")
	}
}
