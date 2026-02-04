package transport

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/mockserver"
)

func TestStreamableHTTPAdapter_WithMockServer(t *testing.T) {
	server, cleanup := mockserver.StartTestServer()
	defer cleanup()

	adapter := NewStreamableHTTPAdapter()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := adapter.Connect(ctx, &TransportConfig{
		Endpoint:             server.MCPURL(),
		AllowPrivateNetworks: []string{"127.0.0.0/8"},
		Timeouts: TimeoutConfig{
			ConnectTimeout:     2 * time.Second,
			RequestTimeout:     5 * time.Second,
			StreamStallTimeout: 5 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer conn.Close()

	initOutcome, err := conn.Initialize(ctx, nil)
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
	}
	if !initOutcome.OK {
		t.Fatalf("initialize outcome not OK: %v", initOutcome.Error)
	}

	initializedOutcome, err := conn.SendInitialized(ctx)
	if err != nil {
		t.Fatalf("send initialized failed: %v", err)
	}
	if !initializedOutcome.OK {
		t.Fatalf("send initialized outcome not OK: %v", initializedOutcome.Error)
	}

	listOutcome, err := conn.ToolsList(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list failed: %v", err)
	}
	if !listOutcome.OK {
		t.Fatalf("tools/list outcome not OK: %v", listOutcome.Error)
	}

	var list ToolsListResult
	if err := json.Unmarshal(listOutcome.Result, &list); err != nil {
		t.Fatalf("unmarshal tools/list result failed: %v", err)
	}
	if len(list.Tools) == 0 {
		t.Fatalf("expected at least one tool, got 0")
	}

	hasFastEcho := false
	for _, tool := range list.Tools {
		if tool.Name == "fast_echo" {
			hasFastEcho = true
			break
		}
	}
	if !hasFastEcho {
		t.Fatalf("expected tools/list to include fast_echo")
	}

	callOutcome, err := conn.ToolsCall(ctx, &ToolsCallParams{
		Name:      "fast_echo",
		Arguments: map[string]interface{}{"message": "hello"},
	})
	if err != nil {
		t.Fatalf("tools/call failed: %v", err)
	}
	if !callOutcome.OK {
		t.Fatalf("tools/call outcome not OK: %v", callOutcome.Error)
	}

	var call ToolsCallResult
	if err := json.Unmarshal(callOutcome.Result, &call); err != nil {
		t.Fatalf("unmarshal tools/call result failed: %v", err)
	}
	if call.IsError {
		t.Fatalf("expected tool call to succeed, got isError=true")
	}
	if len(call.Content) == 0 {
		t.Fatalf("expected tool call to return content")
	}
	if call.Content[0].Text != "echo: hello" {
		t.Fatalf("unexpected tool content: %q", call.Content[0].Text)
	}
}
