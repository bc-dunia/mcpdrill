package mockserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/mcp"
	"github.com/bc-dunia/mcpdrill/internal/types"
)

func TestEvalExpression_DivisionByZeroReturnsError(t *testing.T) {
	_, err := evalExpression("1/0")
	if err == nil {
		t.Fatal("expected division by zero error")
	}
}

func TestMockServerModernDiscover(t *testing.T) {
	srv, cleanup := StartTestServer()
	defer cleanup()

	resp := postModernJSONRPC(t, srv.MCPURL(), map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "discover-1",
		"method":  "server/discover",
	})

	if resp.Error != nil {
		t.Fatalf("expected discover success, got error %+v", resp.Error)
	}
	var result struct {
		SupportedVersions []string               `json:"supportedVersions"`
		Capabilities      map[string]interface{} `json:"capabilities"`
		ServerInfo        types.ServerInfo       `json:"serverInfo"`
		TTLMs             int64                  `json:"ttlMs"`
		CacheScope        string                 `json:"cacheScope"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal discover result: %v", err)
	}
	if len(result.SupportedVersions) == 0 || result.SupportedVersions[0] != mcp.ModernProtocolVersion {
		t.Fatalf("expected modern version first, got %v", result.SupportedVersions)
	}
	if result.ServerInfo.Name != "mockserver" {
		t.Fatalf("expected mockserver info, got %+v", result.ServerInfo)
	}
	if result.TTLMs == 0 || result.CacheScope != "public" {
		t.Fatalf("expected cache metadata, got ttl=%d scope=%q", result.TTLMs, result.CacheScope)
	}
	extensions, ok := result.Capabilities["extensions"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected extension capabilities, got %+v", result.Capabilities)
	}
	if _, ok := extensions["io.modelcontextprotocol/tasks"]; !ok {
		t.Fatalf("expected task extension capability, got %+v", extensions)
	}
}

func TestMockServerAcceptsInitializedNotification(t *testing.T) {
	srv, cleanup := StartTestServer()
	defer cleanup()

	body := []byte(`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`)
	resp, err := http.Post(srv.MCPURL(), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post initialized notification: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}
	data, _ := io.ReadAll(resp.Body)
	if len(bytes.TrimSpace(data)) != 0 {
		t.Fatalf("expected empty notification response body, got %s", string(data))
	}
}

func TestMockServerModernTaskOperations(t *testing.T) {
	srv, cleanup := StartTestServer()
	defer cleanup()

	for _, tc := range []struct {
		method       string
		params       map[string]interface{}
		expectTask   bool
		expectStatus types.TaskStatus
	}{
		{method: "tasks/get", params: map[string]interface{}{"taskId": "task-123"}, expectTask: true, expectStatus: types.TaskStatusCompleted},
		{method: "tasks/update", params: map[string]interface{}{"taskId": "task-123", "inputResponses": map[string]interface{}{"request-1": map[string]interface{}{"type": "text", "text": "ok"}}}},
		{method: "tasks/cancel", params: map[string]interface{}{"taskId": "task-123"}},
	} {
		t.Run(tc.method, func(t *testing.T) {
			resp := postModernJSONRPC(t, srv.MCPURL(), map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      tc.method,
				"method":  tc.method,
				"params":  tc.params,
			})
			if resp.Error != nil {
				t.Fatalf("expected task success, got error %+v", resp.Error)
			}
			if tc.expectTask {
				var task types.Task
				if err := json.Unmarshal(resp.Result, &task); err != nil {
					t.Fatalf("unmarshal task: %v", err)
				}
				if task.ResultType != "complete" || task.TaskID != "task-123" || task.Status != tc.expectStatus {
					t.Fatalf("unexpected task result: %+v", task)
				}
				return
			}
			var ack struct {
				ResultType string `json:"resultType"`
			}
			if err := json.Unmarshal(resp.Result, &ack); err != nil {
				t.Fatalf("unmarshal ack: %v", err)
			}
			if ack.ResultType != "complete" {
				t.Fatalf("unexpected ack result: %+v", ack)
			}
		})
	}
}

func TestMockServerTasksUpdateRequiresInputResponses(t *testing.T) {
	srv, cleanup := StartTestServer()
	defer cleanup()

	resp := postModernJSONRPC(t, srv.MCPURL(), map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "tasks/update",
		"method":  "tasks/update",
		"params":  map[string]interface{}{"taskId": "task-123"},
	})
	if resp.Error == nil {
		t.Fatal("expected missing inputResponses error")
	}
	if resp.Error.Code != -32602 {
		t.Fatalf("error code = %d, want -32602", resp.Error.Code)
	}
}

func TestMockServerSubscriptionsListen(t *testing.T) {
	srv, cleanup := StartTestServer()
	defer cleanup()

	resp := postModernJSONRPC(t, srv.MCPURL(), map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "sub-1",
		"method":  "subscriptions/listen",
		"params": map[string]interface{}{
			"notifications": map[string]interface{}{"toolsListChanged": true},
		},
	})
	if resp.Error != nil {
		t.Fatalf("expected subscriptions/listen success, got error %+v", resp.Error)
	}
	if !bytes.Contains(resp.Result, []byte("notifications/subscriptions/acknowledged")) {
		t.Fatalf("expected subscription acknowledgement, got %s", string(resp.Result))
	}
	if !bytes.Contains(resp.Result, []byte("io.modelcontextprotocol/subscriptionId")) {
		t.Fatalf("expected subscription id metadata, got %s", string(resp.Result))
	}

	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "sub-sse",
		"method":  "subscriptions/listen",
		"params": map[string]interface{}{
			"notifications": map[string]interface{}{"toolsListChanged": true},
			"_meta": map[string]interface{}{
				"io.modelcontextprotocol/protocolVersion": mcp.ModernProtocolVersion,
				"io.modelcontextprotocol/clientInfo":      map[string]interface{}{"name": "mockserver-test", "version": "1.0"},
				"io.modelcontextprotocol/clientCapabilities": map[string]interface{}{
					"extensions": map[string]interface{}{"io.modelcontextprotocol/tasks": map[string]interface{}{}},
				},
			},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.MCPURL(), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("MCP-Protocol-Version", mcp.ModernProtocolVersion)
	req.Header.Set("Mcp-Method", "subscriptions/listen")
	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post subscription SSE: %v", err)
	}
	defer httpResp.Body.Close()
	reader := bufio.NewReader(httpResp.Body)
	sseLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read SSE event line: %v", err)
	}
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("read SSE event terminator: %v", err)
	}
	if strings.Contains(sseLine, `"id":"sub-sse"`) || strings.Contains(sseLine, `"result"`) {
		t.Fatalf("expected raw notification SSE, got %s", sseLine)
	}
	if !strings.Contains(sseLine, `"jsonrpc":"2.0"`) {
		t.Fatalf("expected JSON-RPC notification version, got %s", sseLine)
	}
	if !strings.Contains(sseLine, "notifications/subscriptions/acknowledged") {
		t.Fatalf("expected subscription acknowledgement SSE, got %s", sseLine)
	}
}

func TestMockServerModernMetadataValidation(t *testing.T) {
	srv, cleanup := StartTestServer()
	defer cleanup()

	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "discover-missing-metadata",
		"method":  "server/discover",
	})
	resp, err := http.Post(srv.MCPURL(), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post server/discover: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	var jsonResp types.JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&jsonResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if jsonResp.Error == nil || jsonResp.Error.Code != -32001 {
		t.Fatalf("expected header mismatch JSON-RPC error, got %+v", jsonResp.Error)
	}
}

func TestMockServerModernUnsupportedVersionIncludesNegotiationData(t *testing.T) {
	srv, cleanup := StartTestServer()
	defer cleanup()

	body := modernPayload(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "unsupported-version",
		"method":  "server/discover",
	})
	data, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, srv.MCPURL(), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("MCP-Protocol-Version", "2026-08-01")
	req.Header.Set("Mcp-Method", "server/discover")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post server/discover: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	var jsonResp types.JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&jsonResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if jsonResp.Error == nil || jsonResp.Error.Code != -32004 {
		t.Fatalf("expected unsupported protocol JSON-RPC error, got %+v", jsonResp.Error)
	}
	var details struct {
		Supported []string `json:"supported"`
		Requested string   `json:"requested"`
	}
	if err := json.Unmarshal(jsonResp.Error.Data, &details); err != nil {
		t.Fatalf("decode error data: %v", err)
	}
	if details.Requested != "2026-08-01" || len(details.Supported) == 0 {
		t.Fatalf("unexpected error data: %+v", details)
	}
}

func TestMockServerModernUnknownMethodReturnsHTTP404(t *testing.T) {
	srv, cleanup := StartTestServer()
	defer cleanup()

	body := modernPayload(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "unknown-modern",
		"method":  "unknown/method",
	})
	data, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, srv.MCPURL(), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("MCP-Protocol-Version", mcp.ModernProtocolVersion)
	req.Header.Set("Mcp-Method", "unknown/method")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post unknown method: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	var jsonResp types.JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&jsonResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if jsonResp.Error == nil || jsonResp.Error.Code != -32601 {
		t.Fatalf("expected method-not-found JSON-RPC error, got %+v", jsonResp.Error)
	}
}

func TestMockServerModernRequiresMCPNameForRoutedMethods(t *testing.T) {
	srv, cleanup := StartTestServer()
	defer cleanup()

	body := modernPayload(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "call-missing-name-header",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "fast_echo",
			"arguments": map[string]interface{}{"message": "hello"},
		},
	})
	data, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, srv.MCPURL(), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("MCP-Protocol-Version", mcp.ModernProtocolVersion)
	req.Header.Set("Mcp-Method", "tools/call")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post tools/call: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	var jsonResp types.JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&jsonResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if jsonResp.Error == nil || jsonResp.Error.Code != -32001 {
		t.Fatalf("expected header mismatch JSON-RPC error, got %+v", jsonResp.Error)
	}
}

func postJSONRPC(t *testing.T, url string, payload map[string]interface{}) types.JSONRPCResponse {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post json-rpc: %v", err)
	}
	defer resp.Body.Close()
	var rpcResp types.JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		t.Fatalf("decode json-rpc: %v", err)
	}
	return rpcResp
}

func postModernJSONRPC(t *testing.T, url string, payload map[string]interface{}) types.JSONRPCResponse {
	t.Helper()
	body, err := json.Marshal(modernPayload(payload))
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	method, _ := payload["method"].(string)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("MCP-Protocol-Version", mcp.ModernProtocolVersion)
	req.Header.Set("Mcp-Method", method)
	if name := modernRouteName(payload); name != "" {
		req.Header.Set("Mcp-Name", encodeMCPTestHeaderValue(name))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post json-rpc: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d: %s", resp.StatusCode, string(data))
	}
	var jsonResp types.JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&jsonResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return jsonResp
}

func modernRouteName(payload map[string]interface{}) string {
	method, _ := payload["method"].(string)
	params, _ := payload["params"].(map[string]interface{})
	switch method {
	case "tools/call", "prompts/get":
		name, _ := params["name"].(string)
		return name
	case "resources/read":
		uri, _ := params["uri"].(string)
		return uri
	case "tasks/get", "tasks/update", "tasks/cancel":
		taskID, _ := params["taskId"].(string)
		return taskID
	default:
		return ""
	}
}

func encodeMCPTestHeaderValue(value string) string {
	if strings.TrimSpace(value) != value {
		return "=?base64?" + base64.StdEncoding.EncodeToString([]byte(value)) + "?="
	}
	for _, r := range value {
		if r < 0x20 || r > 0x7e {
			return "=?base64?" + base64.StdEncoding.EncodeToString([]byte(value)) + "?="
		}
	}
	return value
}

func modernPayload(payload map[string]interface{}) map[string]interface{} {
	copyPayload := make(map[string]interface{}, len(payload)+1)
	for key, value := range payload {
		copyPayload[key] = value
	}
	params, _ := copyPayload["params"].(map[string]interface{})
	if params == nil {
		params = map[string]interface{}{}
	}
	params["_meta"] = map[string]interface{}{
		"io.modelcontextprotocol/protocolVersion": mcp.ModernProtocolVersion,
		"io.modelcontextprotocol/clientInfo": map[string]interface{}{
			"name":    "mockserver-test",
			"version": "1.0",
		},
		"io.modelcontextprotocol/clientCapabilities": map[string]interface{}{
			"extensions": map[string]interface{}{
				"io.modelcontextprotocol/tasks": map[string]interface{}{},
			},
		},
	}
	copyPayload["params"] = params
	return copyPayload
}
