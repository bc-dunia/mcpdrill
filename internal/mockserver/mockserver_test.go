package mockserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

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

	resp := postJSONRPC(t, srv.MCPURL(), map[string]interface{}{
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
			resp := postJSONRPC(t, srv.MCPURL(), map[string]interface{}{
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

	resp := postJSONRPC(t, srv.MCPURL(), map[string]interface{}{
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

	resp := postJSONRPC(t, srv.MCPURL(), map[string]interface{}{
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
		},
	})
	req, err := http.NewRequest(http.MethodPost, srv.MCPURL(), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post subscription SSE: %v", err)
	}
	defer httpResp.Body.Close()
	sseBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		t.Fatalf("read SSE body: %v", err)
	}
	if bytes.Contains(sseBody, []byte(`"id":"sub-sse"`)) || bytes.Contains(sseBody, []byte(`"result"`)) {
		t.Fatalf("expected raw notification SSE, got %s", string(sseBody))
	}
	if !bytes.Contains(sseBody, []byte(`"jsonrpc":"2.0"`)) {
		t.Fatalf("expected JSON-RPC notification version, got %s", string(sseBody))
	}
	if !bytes.Contains(sseBody, []byte("notifications/subscriptions/acknowledged")) {
		t.Fatalf("expected subscription acknowledgement SSE, got %s", string(sseBody))
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
