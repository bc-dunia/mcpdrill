package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/analysis"
	"github.com/bc-dunia/mcpdrill/internal/mockserver"
)

// =============================================================================
// JSON-RPC Helpers for Tool Tests
// =============================================================================

type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type toolsListResult struct {
	Tools []struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"inputSchema"`
	} `json:"tools"`
}

type toolsCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

func sendMCPRequest(t *testing.T, mcpURL string, method string, params interface{}) *jsonRPCResponse {
	t.Helper()
	return sendMCPRequestWithTimeout(t, mcpURL, method, params, 30*time.Second)
}

func sendMCPRequestWithTimeout(t *testing.T, mcpURL string, method string, params interface{}, timeout time.Duration) *jsonRPCResponse {
	t.Helper()

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, mcpURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v, body: %s", err, string(respBody))
	}

	return &rpcResp
}

func sendMCPRequestNoFail(mcpURL string, method string, params interface{}, timeout time.Duration) (*jsonRPCResponse, error) {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal error: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, mcpURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request error: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response error: %w", err)
	}

	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	return &rpcResp, nil
}

// =============================================================================
// Test: Tools List Operation
// =============================================================================

func TestToolsListOperation(t *testing.T) {
	server, cleanup := mockserver.StartTestServer()
	defer cleanup()

	mcpURL := server.MCPURL()
	t.Logf("Mock server started at %s", mcpURL)

	// Initialize session first
	initResp := sendMCPRequest(t, mcpURL, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "test-client",
			"version": "1.0.0",
		},
	})
	if initResp.Error != nil {
		t.Fatalf("Initialize failed: %v", initResp.Error.Message)
	}

	// Test tools/list
	resp := sendMCPRequest(t, mcpURL, "tools/list", nil)
	if resp.Error != nil {
		t.Fatalf("tools/list failed: %v", resp.Error.Message)
	}

	var result toolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("Failed to unmarshal tools list: %v", err)
	}

	// Verify we have all 27 tools (5 original + 15 new + 7 advanced testing)
	expectedToolCount := 27
	if len(result.Tools) != expectedToolCount {
		t.Errorf("Expected %d tools, got %d", expectedToolCount, len(result.Tools))
	}

	// Verify all expected tools are present
	expectedTools := []string{
		// Original 5
		"fast_echo", "slow_echo", "error_tool", "timeout_tool", "streaming_tool",
		// 15 new tools
		"json_transform", "text_processor", "list_operations",
		"validate_email", "calculate", "hash_generator",
		"weather_api", "geocode", "currency_convert",
		"read_file", "write_file", "list_directory",
		"large_payload", "random_latency", "conditional_error",
		// 7 advanced testing tools
		"degrading_performance", "flaky_connection", "rate_limited",
		"circuit_breaker", "backpressure", "stateful_counter", "realistic_latency",
	}

	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}

	for _, expected := range expectedTools {
		if !toolNames[expected] {
			t.Errorf("Expected tool %q not found in tools list", expected)
		}
	}

	t.Logf("tools/list returned %d tools successfully", len(result.Tools))
}

// =============================================================================
// Test: Simple Arguments (primitives)
// =============================================================================

func TestToolsCallSimpleArguments(t *testing.T) {
	server, cleanup := mockserver.StartTestServer()
	defer cleanup()

	mcpURL := server.MCPURL()

	tests := []struct {
		name     string
		tool     string
		args     map[string]interface{}
		validate func(t *testing.T, result *toolsCallResult)
	}{
		{
			name: "fast_echo with string",
			tool: "fast_echo",
			args: map[string]interface{}{"message": "hello world"},
			validate: func(t *testing.T, result *toolsCallResult) {
				if result.IsError {
					t.Error("Expected success, got error")
				}
				if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "hello world") {
					t.Error("Expected response to contain message")
				}
			},
		},
		{
			name: "text_processor uppercase",
			tool: "text_processor",
			args: map[string]interface{}{"text": "hello", "operation": "uppercase"},
			validate: func(t *testing.T, result *toolsCallResult) {
				if result.IsError {
					t.Error("Expected success, got error")
				}
				if len(result.Content) == 0 || result.Content[0].Text != "HELLO" {
					t.Errorf("Expected 'HELLO', got '%s'", result.Content[0].Text)
				}
			},
		},
		{
			name: "calculate simple expression",
			tool: "calculate",
			args: map[string]interface{}{"expression": "2 + 3 * 4"},
			validate: func(t *testing.T, result *toolsCallResult) {
				if result.IsError {
					t.Error("Expected success, got error")
				}
				if len(result.Content) == 0 || result.Content[0].Text != "14" {
					t.Errorf("Expected '14', got '%s'", result.Content[0].Text)
				}
			},
		},
		{
			name: "validate_email valid",
			tool: "validate_email",
			args: map[string]interface{}{"email": "test@example.com"},
			validate: func(t *testing.T, result *toolsCallResult) {
				if result.IsError {
					t.Error("Expected success, got error")
				}
				if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, `"valid":true`) {
					t.Error("Expected email to be valid")
				}
			},
		},
		{
			name: "hash_generator sha256",
			tool: "hash_generator",
			args: map[string]interface{}{"data": "test", "algorithm": "sha256"},
			validate: func(t *testing.T, result *toolsCallResult) {
				if result.IsError {
					t.Error("Expected success, got error")
				}
				// SHA256 of "test" is known
				expected := "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
				if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, expected) {
					t.Errorf("Expected hash %s in result", expected)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := sendMCPRequest(t, mcpURL, "tools/call", map[string]interface{}{
				"name":      tc.tool,
				"arguments": tc.args,
			})

			if resp.Error != nil {
				t.Fatalf("tools/call failed: %v", resp.Error.Message)
			}

			var result toolsCallResult
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				t.Fatalf("Failed to unmarshal result: %v", err)
			}

			tc.validate(t, &result)
		})
	}
}

// =============================================================================
// Test: Complex Arguments (nested objects)
// =============================================================================

func TestToolsCallComplexArguments(t *testing.T) {
	server, cleanup := mockserver.StartTestServer()
	defer cleanup()

	mcpURL := server.MCPURL()

	tests := []struct {
		name     string
		tool     string
		args     map[string]interface{}
		validate func(t *testing.T, result *toolsCallResult)
	}{
		{
			name: "json_transform with nested object",
			tool: "json_transform",
			args: map[string]interface{}{
				"operation": "uppercase_keys",
				"data": map[string]interface{}{
					"firstName": "John",
					"lastName":  "Doe",
					"address": map[string]interface{}{
						"city":  "NYC",
						"state": "NY",
					},
				},
			},
			validate: func(t *testing.T, result *toolsCallResult) {
				if result.IsError {
					t.Error("Expected success, got error")
				}
				if len(result.Content) == 0 {
					t.Error("Expected content in result")
					return
				}
				// Verify uppercase keys
				if !strings.Contains(result.Content[0].Text, "FIRSTNAME") {
					t.Error("Expected uppercase key FIRSTNAME")
				}
			},
		},
		{
			name: "json_transform filter operation",
			tool: "json_transform",
			args: map[string]interface{}{
				"operation":  "filter",
				"filter_key": "name",
				"data": map[string]interface{}{
					"name":  "test",
					"value": 123,
					"extra": "ignored",
				},
			},
			validate: func(t *testing.T, result *toolsCallResult) {
				if result.IsError {
					t.Error("Expected success, got error")
				}
				if len(result.Content) == 0 {
					t.Error("Expected content in result")
					return
				}
				// Should only contain filtered key
				text := result.Content[0].Text
				if !strings.Contains(text, "name") {
					t.Error("Expected filtered result to contain 'name'")
				}
				if strings.Contains(text, "extra") {
					t.Error("Expected filtered result to not contain 'extra'")
				}
			},
		},
		{
			name: "weather_api with options",
			tool: "weather_api",
			args: map[string]interface{}{
				"city":  "London",
				"units": "celsius",
			},
			validate: func(t *testing.T, result *toolsCallResult) {
				if result.IsError {
					t.Error("Expected success, got error")
				}
				if len(result.Content) == 0 {
					t.Error("Expected content in result")
					return
				}
				text := result.Content[0].Text
				if !strings.Contains(text, "London") {
					t.Error("Expected result to contain city name")
				}
				if !strings.Contains(text, "celsius") {
					t.Error("Expected result to contain units")
				}
			},
		},
		{
			name: "currency_convert",
			tool: "currency_convert",
			args: map[string]interface{}{
				"amount": 100.0,
				"from":   "USD",
				"to":     "EUR",
			},
			validate: func(t *testing.T, result *toolsCallResult) {
				if result.IsError {
					t.Error("Expected success, got error")
				}
				if len(result.Content) == 0 {
					t.Error("Expected content in result")
					return
				}
				text := result.Content[0].Text
				if !strings.Contains(text, "converted") {
					t.Error("Expected result to contain 'converted' field")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := sendMCPRequest(t, mcpURL, "tools/call", map[string]interface{}{
				"name":      tc.tool,
				"arguments": tc.args,
			})

			if resp.Error != nil {
				t.Fatalf("tools/call failed: %v", resp.Error.Message)
			}

			var result toolsCallResult
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				t.Fatalf("Failed to unmarshal result: %v", err)
			}

			tc.validate(t, &result)
		})
	}
}

// =============================================================================
// Test: Array Arguments
// =============================================================================

func TestToolsCallArrayArguments(t *testing.T) {
	server, cleanup := mockserver.StartTestServer()
	defer cleanup()

	mcpURL := server.MCPURL()

	tests := []struct {
		name     string
		tool     string
		args     map[string]interface{}
		validate func(t *testing.T, result *toolsCallResult)
	}{
		{
			name: "list_operations sum",
			tool: "list_operations",
			args: map[string]interface{}{
				"list":      []float64{1, 2, 3, 4, 5},
				"operation": "sum",
			},
			validate: func(t *testing.T, result *toolsCallResult) {
				if result.IsError {
					t.Error("Expected success, got error")
				}
				if len(result.Content) == 0 || result.Content[0].Text != "15" {
					t.Errorf("Expected '15', got '%s'", result.Content[0].Text)
				}
			},
		},
		{
			name: "list_operations avg",
			tool: "list_operations",
			args: map[string]interface{}{
				"list":      []float64{10, 20, 30},
				"operation": "avg",
			},
			validate: func(t *testing.T, result *toolsCallResult) {
				if result.IsError {
					t.Error("Expected success, got error")
				}
				if len(result.Content) == 0 || result.Content[0].Text != "20" {
					t.Errorf("Expected '20', got '%s'", result.Content[0].Text)
				}
			},
		},
		{
			name: "list_operations sort",
			tool: "list_operations",
			args: map[string]interface{}{
				"list":      []float64{5, 2, 8, 1, 9},
				"operation": "sort",
			},
			validate: func(t *testing.T, result *toolsCallResult) {
				if result.IsError {
					t.Error("Expected success, got error")
				}
				if len(result.Content) == 0 {
					t.Error("Expected content in result")
					return
				}
				expected := "[1,2,5,8,9]"
				if result.Content[0].Text != expected {
					t.Errorf("Expected '%s', got '%s'", expected, result.Content[0].Text)
				}
			},
		},
		{
			name: "list_operations filter",
			tool: "list_operations",
			args: map[string]interface{}{
				"list":         []float64{1, 5, 10, 15, 20},
				"operation":    "filter",
				"filter_value": 10.0,
			},
			validate: func(t *testing.T, result *toolsCallResult) {
				if result.IsError {
					t.Error("Expected success, got error")
				}
				if len(result.Content) == 0 {
					t.Error("Expected content in result")
					return
				}
				// Filter keeps values > 10
				expected := "[15,20]"
				if result.Content[0].Text != expected {
					t.Errorf("Expected '%s', got '%s'", expected, result.Content[0].Text)
				}
			},
		},
		{
			name: "list_operations max",
			tool: "list_operations",
			args: map[string]interface{}{
				"list":      []float64{3, 7, 2, 9, 4},
				"operation": "max",
			},
			validate: func(t *testing.T, result *toolsCallResult) {
				if result.IsError {
					t.Error("Expected success, got error")
				}
				if len(result.Content) == 0 || result.Content[0].Text != "9" {
					t.Errorf("Expected '9', got '%s'", result.Content[0].Text)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := sendMCPRequest(t, mcpURL, "tools/call", map[string]interface{}{
				"name":      tc.tool,
				"arguments": tc.args,
			})

			if resp.Error != nil {
				t.Fatalf("tools/call failed: %v", resp.Error.Message)
			}

			var result toolsCallResult
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				t.Fatalf("Failed to unmarshal result: %v", err)
			}

			tc.validate(t, &result)
		})
	}
}

// =============================================================================
// Test: Nested Objects (deep structures)
// =============================================================================

func TestToolsCallNestedObjects(t *testing.T) {
	server, cleanup := mockserver.StartTestServer()
	defer cleanup()

	mcpURL := server.MCPURL()

	// Test with deeply nested JSON (3-4 levels)
	t.Run("deeply nested json_transform", func(t *testing.T) {
		args := map[string]interface{}{
			"operation": "lowercase_values",
			"data": map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": map[string]interface{}{
							"value": "DEEPLY_NESTED_VALUE",
						},
						"anotherValue": "MID_LEVEL",
					},
				},
				"topLevel": "TOP_VALUE",
			},
		}

		resp := sendMCPRequest(t, mcpURL, "tools/call", map[string]interface{}{
			"name":      "json_transform",
			"arguments": args,
		})

		if resp.Error != nil {
			t.Fatalf("tools/call failed: %v", resp.Error.Message)
		}

		var result toolsCallResult
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			t.Fatalf("Failed to unmarshal result: %v", err)
		}

		if result.IsError {
			t.Error("Expected success, got error")
		}

		// Verify depth calculation
		depth := analysis.CalculateArgumentDepth(args)
		if depth < 4 {
			t.Errorf("Expected depth >= 4, got %d", depth)
		}
		t.Logf("Nested object depth: %d", depth)
	})

	// Test write_file with nested content
	t.Run("write_file with content", func(t *testing.T) {
		args := map[string]interface{}{
			"path":    "/test/nested/file.json",
			"content": `{"nested": {"data": {"value": 123}}}`,
		}

		resp := sendMCPRequest(t, mcpURL, "tools/call", map[string]interface{}{
			"name":      "write_file",
			"arguments": args,
		})

		if resp.Error != nil {
			t.Fatalf("tools/call failed: %v", resp.Error.Message)
		}

		var result toolsCallResult
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			t.Fatalf("Failed to unmarshal result: %v", err)
		}

		if result.IsError {
			t.Error("Expected success, got error")
		}
		if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "success") {
			t.Error("Expected success message in result")
		}
	})
}

// =============================================================================
// Test: Invalid Arguments
// =============================================================================

func TestToolsCallInvalidArguments(t *testing.T) {
	server, cleanup := mockserver.StartTestServer()
	defer cleanup()

	mcpURL := server.MCPURL()

	tests := []struct {
		name string
		tool string
		args map[string]interface{}
	}{
		{
			name: "json_transform missing data",
			tool: "json_transform",
			args: map[string]interface{}{
				"operation": "uppercase_keys",
				// missing "data" field
			},
		},
		{
			name: "json_transform invalid data type",
			tool: "json_transform",
			args: map[string]interface{}{
				"operation": "uppercase_keys",
				"data":      "not an object", // should be object
			},
		},
		{
			name: "list_operations empty list",
			tool: "list_operations",
			args: map[string]interface{}{
				"list":      []float64{},
				"operation": "sum",
			},
		},
		{
			name: "calculate invalid expression",
			tool: "calculate",
			args: map[string]interface{}{
				"expression": "invalid!!expression",
			},
		},
		{
			name: "hash_generator unknown algorithm",
			tool: "hash_generator",
			args: map[string]interface{}{
				"data":      "test",
				"algorithm": "unknown_algo",
			},
		},
		{
			name: "currency_convert unsupported currency",
			tool: "currency_convert",
			args: map[string]interface{}{
				"amount": 100.0,
				"from":   "XYZ",
				"to":     "ABC",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := sendMCPRequest(t, mcpURL, "tools/call", map[string]interface{}{
				"name":      tc.tool,
				"arguments": tc.args,
			})

			// Either JSON-RPC error or tool result with isError=true
			if resp.Error == nil {
				var result toolsCallResult
				if err := json.Unmarshal(resp.Result, &result); err != nil {
					t.Fatalf("Failed to unmarshal result: %v", err)
				}
				if !result.IsError {
					t.Errorf("Expected error for invalid arguments, but got success")
				}
			}
			// If resp.Error != nil, that's also acceptable
		})
	}
}

// =============================================================================
// Test: Unknown Tool
// =============================================================================

func TestToolsCallUnknownTool(t *testing.T) {
	server, cleanup := mockserver.StartTestServer()
	defer cleanup()

	mcpURL := server.MCPURL()

	resp := sendMCPRequest(t, mcpURL, "tools/call", map[string]interface{}{
		"name":      "nonexistent_tool_xyz",
		"arguments": map[string]interface{}{},
	})

	// Should return error (either JSON-RPC error or tool error)
	if resp.Error == nil {
		var result toolsCallResult
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			t.Fatalf("Failed to unmarshal result: %v", err)
		}
		if !result.IsError {
			t.Errorf("Expected error for unknown tool, but got success")
		}
		if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "unknown tool") {
			t.Errorf("Expected 'unknown tool' error message")
		}
	}
	t.Logf("Unknown tool correctly rejected")
}

// =============================================================================
// Test: Timeout Handling
// =============================================================================

func TestToolsCallTimeout(t *testing.T) {
	server, cleanup := mockserver.StartTestServer()
	defer cleanup()

	mcpURL := server.MCPURL()

	// Test with random_latency tool configured for long delay
	t.Run("timeout with random_latency", func(t *testing.T) {
		// Use a very short client timeout to trigger timeout
		_, err := sendMCPRequestNoFail(mcpURL, "tools/call", map[string]interface{}{
			"name": "random_latency",
			"arguments": map[string]interface{}{
				"min_ms": 5000.0, // 5 seconds
				"max_ms": 6000.0, // 6 seconds
			},
		}, 100*time.Millisecond) // 100ms timeout

		if err == nil {
			t.Error("Expected timeout error")
		} else if !strings.Contains(err.Error(), "context deadline") && !strings.Contains(err.Error(), "timeout") {
			t.Logf("Got expected error type: %v", err)
		}
	})

	// Test with timeout_tool (blocks forever)
	t.Run("timeout_tool", func(t *testing.T) {
		_, err := sendMCPRequestNoFail(mcpURL, "tools/call", map[string]interface{}{
			"name":      "timeout_tool",
			"arguments": map[string]interface{}{},
		}, 200*time.Millisecond)

		if err == nil {
			t.Error("Expected timeout error")
		}
	})
}

// =============================================================================
// Test: Load Testing (1000+ concurrent calls)
// =============================================================================

func TestToolsCallUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	server, cleanup := mockserver.StartTestServer()
	defer cleanup()

	mcpURL := server.MCPURL()

	const (
		numConcurrent = 100
		numRequests   = 1000
	)

	// Track results
	var successCount atomic.Int64
	var errorCount atomic.Int64
	var latencies sync.Map // map[int]int64 for latency tracking

	// Tools to test in rotation
	tools := []struct {
		name string
		args map[string]interface{}
	}{
		{"fast_echo", map[string]interface{}{"message": "load test"}},
		{"text_processor", map[string]interface{}{"text": "test", "operation": "uppercase"}},
		{"calculate", map[string]interface{}{"expression": "1+1"}},
		{"validate_email", map[string]interface{}{"email": "test@test.com"}},
		{"hash_generator", map[string]interface{}{"data": "test", "algorithm": "md5"}},
		{"list_operations", map[string]interface{}{"list": []float64{1, 2, 3}, "operation": "sum"}},
	}

	// Semaphore for concurrency control
	sem := make(chan struct{}, numConcurrent)
	var wg sync.WaitGroup

	startTime := time.Now()

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		sem <- struct{}{} // Acquire

		go func(reqNum int) {
			defer wg.Done()
			defer func() { <-sem }() // Release

			tool := tools[reqNum%len(tools)]
			reqStart := time.Now()

			resp, err := sendMCPRequestNoFail(mcpURL, "tools/call", map[string]interface{}{
				"name":      tool.name,
				"arguments": tool.args,
			}, 10*time.Second)

			latency := time.Since(reqStart).Milliseconds()
			latencies.Store(reqNum, latency)

			if err != nil {
				errorCount.Add(1)
				return
			}

			if resp.Error != nil {
				errorCount.Add(1)
				return
			}

			var result toolsCallResult
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				errorCount.Add(1)
				return
			}

			if result.IsError {
				errorCount.Add(1)
				return
			}

			successCount.Add(1)
		}(i)
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	// Calculate statistics
	var allLatencies []int
	latencies.Range(func(key, value interface{}) bool {
		allLatencies = append(allLatencies, int(value.(int64)))
		return true
	})

	sort.Ints(allLatencies)
	var totalLatency int64
	for _, l := range allLatencies {
		totalLatency += int64(l)
	}

	avgLatency := float64(totalLatency) / float64(len(allLatencies))
	p50 := allLatencies[len(allLatencies)*50/100]
	p95 := allLatencies[len(allLatencies)*95/100]
	p99 := allLatencies[len(allLatencies)*99/100]

	success := successCount.Load()
	errors := errorCount.Load()
	rps := float64(numRequests) / totalDuration.Seconds()

	t.Logf("Load test results:")
	t.Logf("  Total requests: %d", numRequests)
	t.Logf("  Concurrent: %d", numConcurrent)
	t.Logf("  Success: %d (%.2f%%)", success, float64(success)/float64(numRequests)*100)
	t.Logf("  Errors: %d", errors)
	t.Logf("  Duration: %v", totalDuration)
	t.Logf("  RPS: %.2f", rps)
	t.Logf("  Latency avg: %.2fms, p50: %dms, p95: %dms, p99: %dms", avgLatency, p50, p95, p99)

	// Assertions
	if float64(errors)/float64(numRequests) > 0.01 { // Max 1% error rate
		t.Errorf("Error rate too high: %.2f%%", float64(errors)/float64(numRequests)*100)
	}
}

// =============================================================================
// Test: Mixed Operations
// =============================================================================

func TestMixedOperationsWithToolCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping mixed operations test in short mode")
	}

	server, cleanup := mockserver.StartTestServer()
	defer cleanup()

	mcpURL := server.MCPURL()

	const numIterations = 100

	var wg sync.WaitGroup
	var successCount atomic.Int64
	var errorCount atomic.Int64

	for i := 0; i < numIterations; i++ {
		wg.Add(1)
		go func(iteration int) {
			defer wg.Done()

			// Mix of operations
			operations := []func() error{
				// tools/list
				func() error {
					resp, err := sendMCPRequestNoFail(mcpURL, "tools/list", nil, 5*time.Second)
					if err != nil || resp.Error != nil {
						return fmt.Errorf("tools/list failed")
					}
					return nil
				},
				// ping
				func() error {
					resp, err := sendMCPRequestNoFail(mcpURL, "ping", nil, 5*time.Second)
					if err != nil || resp.Error != nil {
						return fmt.Errorf("ping failed")
					}
					return nil
				},
				// tools/call fast_echo
				func() error {
					resp, err := sendMCPRequestNoFail(mcpURL, "tools/call", map[string]interface{}{
						"name":      "fast_echo",
						"arguments": map[string]interface{}{"message": "mixed test"},
					}, 5*time.Second)
					if err != nil || resp.Error != nil {
						return fmt.Errorf("tools/call failed")
					}
					return nil
				},
				// tools/call calculate
				func() error {
					resp, err := sendMCPRequestNoFail(mcpURL, "tools/call", map[string]interface{}{
						"name":      "calculate",
						"arguments": map[string]interface{}{"expression": "10 * 5"},
					}, 5*time.Second)
					if err != nil || resp.Error != nil {
						return fmt.Errorf("tools/call calculate failed")
					}
					return nil
				},
			}

			// Execute all operations for this iteration
			for _, op := range operations {
				if err := op(); err != nil {
					errorCount.Add(1)
				} else {
					successCount.Add(1)
				}
			}
		}(i)
	}

	wg.Wait()

	totalOps := numIterations * 4 // 4 operations per iteration
	success := successCount.Load()
	errors := errorCount.Load()

	t.Logf("Mixed operations results:")
	t.Logf("  Total operations: %d", totalOps)
	t.Logf("  Success: %d (%.2f%%)", success, float64(success)/float64(totalOps)*100)
	t.Logf("  Errors: %d", errors)

	if float64(errors)/float64(totalOps) > 0.01 {
		t.Errorf("Error rate too high: %.2f%%", float64(errors)/float64(totalOps)*100)
	}
}

// =============================================================================
// Test: Tool Metrics Aggregation
// =============================================================================

func TestToolMetricsAggregation(t *testing.T) {
	// Create mock operation logs for multiple tools
	logs := []analysis.OperationLog{
		// fast_echo calls
		{Operation: "tools/call", ToolName: "fast_echo", LatencyMs: 50, OK: true, ArgumentSize: 20, ResultSize: 30},
		{Operation: "tools/call", ToolName: "fast_echo", LatencyMs: 55, OK: true, ArgumentSize: 22, ResultSize: 32},
		{Operation: "tools/call", ToolName: "fast_echo", LatencyMs: 60, OK: true, ArgumentSize: 18, ResultSize: 28},
		{Operation: "tools/call", ToolName: "fast_echo", LatencyMs: 45, OK: false, ArgumentSize: 20, ResultSize: 10},
		// calculate calls
		{Operation: "tools/call", ToolName: "calculate", LatencyMs: 80, OK: true, ArgumentSize: 15, ResultSize: 5},
		{Operation: "tools/call", ToolName: "calculate", LatencyMs: 85, OK: true, ArgumentSize: 20, ResultSize: 8},
		{Operation: "tools/call", ToolName: "calculate", LatencyMs: 90, OK: true, ArgumentSize: 12, ResultSize: 4},
		// json_transform calls
		{Operation: "tools/call", ToolName: "json_transform", LatencyMs: 75, OK: true, ArgumentSize: 100, ResultSize: 80},
		{Operation: "tools/call", ToolName: "json_transform", LatencyMs: 80, OK: true, ArgumentSize: 150, ResultSize: 120},
		{Operation: "tools/call", ToolName: "json_transform", LatencyMs: 70, OK: false, ArgumentSize: 200, ResultSize: 50},
		// validate_email calls
		{Operation: "tools/call", ToolName: "validate_email", LatencyMs: 100, OK: true, ArgumentSize: 30, ResultSize: 40},
		{Operation: "tools/call", ToolName: "validate_email", LatencyMs: 105, OK: true, ArgumentSize: 35, ResultSize: 45},
	}

	// Aggregate metrics
	metrics := analysis.AggregateToolMetrics(logs)

	// Verify fast_echo metrics
	if fe, ok := metrics["fast_echo"]; ok {
		if fe.TotalCalls != 4 {
			t.Errorf("fast_echo: expected 4 total calls, got %d", fe.TotalCalls)
		}
		if fe.SuccessCount != 3 {
			t.Errorf("fast_echo: expected 3 success, got %d", fe.SuccessCount)
		}
		if fe.ErrorCount != 1 {
			t.Errorf("fast_echo: expected 1 error, got %d", fe.ErrorCount)
		}
		t.Logf("fast_echo: total=%d, success=%d, errors=%d, avgLatency=%.2f",
			fe.TotalCalls, fe.SuccessCount, fe.ErrorCount, fe.AvgLatencyMs)
	} else {
		t.Error("fast_echo metrics not found")
	}

	// Verify calculate metrics
	if calc, ok := metrics["calculate"]; ok {
		if calc.TotalCalls != 3 {
			t.Errorf("calculate: expected 3 total calls, got %d", calc.TotalCalls)
		}
		if calc.SuccessCount != 3 {
			t.Errorf("calculate: expected 3 success, got %d", calc.SuccessCount)
		}
		t.Logf("calculate: total=%d, success=%d, avgLatency=%.2f",
			calc.TotalCalls, calc.SuccessCount, calc.AvgLatencyMs)
	} else {
		t.Error("calculate metrics not found")
	}

	// Verify json_transform metrics
	if jt, ok := metrics["json_transform"]; ok {
		if jt.TotalCalls != 3 {
			t.Errorf("json_transform: expected 3 total calls, got %d", jt.TotalCalls)
		}
		if jt.ErrorCount != 1 {
			t.Errorf("json_transform: expected 1 error, got %d", jt.ErrorCount)
		}
		// Verify payload tracking
		if jt.AvgPayloadSize <= 0 {
			t.Error("json_transform: expected positive avg payload size")
		}
		t.Logf("json_transform: total=%d, avgPayloadSize=%d",
			jt.TotalCalls, jt.AvgPayloadSize)
	} else {
		t.Error("json_transform metrics not found")
	}

	// Verify total tool count
	if len(metrics) != 4 {
		t.Errorf("Expected 4 tools in metrics, got %d", len(metrics))
	}

	t.Logf("Aggregated metrics for %d tools successfully", len(metrics))
}

// =============================================================================
// Test: Per-Tool Latency Tracking
// =============================================================================

func TestPerToolLatencyTracking(t *testing.T) {
	// Create logs with varying latencies to test percentile calculation
	logs := []analysis.OperationLog{}

	// Add 100 fast_echo calls with latencies from 10-110ms
	for i := 0; i < 100; i++ {
		logs = append(logs, analysis.OperationLog{
			Operation:    "tools/call",
			ToolName:     "fast_echo",
			LatencyMs:    int64(10 + i), // 10, 11, 12, ..., 109
			OK:           true,
			ArgumentSize: 20,
			ResultSize:   30,
		})
	}

	// Add 50 slow_echo calls with latencies from 200-700ms
	for i := 0; i < 50; i++ {
		logs = append(logs, analysis.OperationLog{
			Operation:    "tools/call",
			ToolName:     "slow_echo",
			LatencyMs:    int64(200 + i*10), // 200, 210, 220, ..., 690
			OK:           true,
			ArgumentSize: 25,
			ResultSize:   35,
		})
	}

	metrics := analysis.AggregateToolMetrics(logs)

	// Verify fast_echo latency distribution
	fe := metrics["fast_echo"]
	if fe == nil {
		t.Fatal("fast_echo metrics not found")
	}

	t.Logf("fast_echo latency: avg=%.2fms, p95=%.2fms, p99=%.2fms",
		fe.AvgLatencyMs, fe.P95LatencyMs, fe.P99LatencyMs)

	// Avg should be around 59.5 ((10+109)/2)
	if fe.AvgLatencyMs < 55 || fe.AvgLatencyMs > 65 {
		t.Errorf("fast_echo avg latency %.2f not in expected range [55-65]", fe.AvgLatencyMs)
	}

	// P95 should be around 104 (95th percentile of 10-109)
	if fe.P95LatencyMs < 100 || fe.P95LatencyMs > 110 {
		t.Errorf("fast_echo p95 latency %.2f not in expected range [100-110]", fe.P95LatencyMs)
	}

	// Verify slow_echo latency distribution
	se := metrics["slow_echo"]
	if se == nil {
		t.Fatal("slow_echo metrics not found")
	}

	t.Logf("slow_echo latency: avg=%.2fms, p95=%.2fms, p99=%.2fms",
		se.AvgLatencyMs, se.P95LatencyMs, se.P99LatencyMs)

	// Avg should be around 445 ((200+690)/2)
	if se.AvgLatencyMs < 400 || se.AvgLatencyMs > 500 {
		t.Errorf("slow_echo avg latency %.2f not in expected range [400-500]", se.AvgLatencyMs)
	}

	// Verify latency ordering: avg < p95 <= p99 for each tool
	for name, m := range metrics {
		if m.P95LatencyMs > m.P99LatencyMs {
			t.Errorf("%s: p95 (%.2f) should be <= p99 (%.2f)", name, m.P95LatencyMs, m.P99LatencyMs)
		}
	}

	t.Logf("Per-tool latency tracking verified for %d tools", len(metrics))
}

// =============================================================================
// Test: Large Payload Tool
// =============================================================================

func TestToolsCallLargePayload(t *testing.T) {
	server, cleanup := mockserver.StartTestServer()
	defer cleanup()

	mcpURL := server.MCPURL()

	tests := []struct {
		name   string
		sizeKB float64
	}{
		{"1KB payload", 1},
		{"10KB payload", 10},
		{"50KB payload", 50},
		{"100KB payload", 100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := sendMCPRequest(t, mcpURL, "tools/call", map[string]interface{}{
				"name": "large_payload",
				"arguments": map[string]interface{}{
					"size_kb": tc.sizeKB,
				},
			})

			if resp.Error != nil {
				t.Fatalf("tools/call failed: %v", resp.Error.Message)
			}

			var result toolsCallResult
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				t.Fatalf("Failed to unmarshal result: %v", err)
			}

			if result.IsError {
				t.Error("Expected success, got error")
				return
			}

			if len(result.Content) == 0 {
				t.Error("Expected content in result")
				return
			}

			// Verify approximate size
			resultSize := len(result.Content[0].Text)
			expectedMinSize := int(tc.sizeKB * 1024 * 0.8) // Allow 20% variance
			if resultSize < expectedMinSize {
				t.Errorf("Expected result size >= %d bytes, got %d", expectedMinSize, resultSize)
			}

			t.Logf("Large payload %v: result size = %d bytes", tc.name, resultSize)
		})
	}
}

// =============================================================================
// Test: Conditional Error Tool
// =============================================================================

func TestToolsCallConditionalError(t *testing.T) {
	server, cleanup := mockserver.StartTestServer()
	defer cleanup()

	mcpURL := server.MCPURL()

	// Test with 0% error probability (should always succeed)
	t.Run("0% error probability", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			resp := sendMCPRequest(t, mcpURL, "tools/call", map[string]interface{}{
				"name": "conditional_error",
				"arguments": map[string]interface{}{
					"error_probability": 0.0,
				},
			})

			if resp.Error != nil {
				t.Fatalf("tools/call failed: %v", resp.Error.Message)
			}

			var result toolsCallResult
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				t.Fatalf("Failed to unmarshal result: %v", err)
			}

			if result.IsError {
				t.Error("Expected success with 0% error probability")
			}
		}
	})

	// Test with 100% error probability (should always fail)
	t.Run("100% error probability", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			resp := sendMCPRequest(t, mcpURL, "tools/call", map[string]interface{}{
				"name": "conditional_error",
				"arguments": map[string]interface{}{
					"error_probability": 1.0,
				},
			})

			if resp.Error != nil {
				continue // JSON-RPC error is acceptable
			}

			var result toolsCallResult
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				t.Fatalf("Failed to unmarshal result: %v", err)
			}

			if !result.IsError {
				t.Error("Expected error with 100% error probability")
			}
		}
	})

	// Test with 50% error probability (should have mix)
	t.Run("50% error probability", func(t *testing.T) {
		var successCount, errorCount int
		iterations := 100

		for i := 0; i < iterations; i++ {
			resp := sendMCPRequest(t, mcpURL, "tools/call", map[string]interface{}{
				"name": "conditional_error",
				"arguments": map[string]interface{}{
					"error_probability": 0.5,
				},
			})

			if resp.Error != nil {
				errorCount++
				continue
			}

			var result toolsCallResult
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				errorCount++
				continue
			}

			if result.IsError {
				errorCount++
			} else {
				successCount++
			}
		}

		// With 50% probability, expect roughly even split (allow 20% variance)
		errorRate := float64(errorCount) / float64(iterations)
		if errorRate < 0.3 || errorRate > 0.7 {
			t.Errorf("Expected error rate around 50%%, got %.2f%%", errorRate*100)
		}

		t.Logf("50%% probability: success=%d, errors=%d (%.2f%% error rate)",
			successCount, errorCount, errorRate*100)
	})
}

// =============================================================================
// Test: All 15 New Tools
// =============================================================================

func TestAllNewTools(t *testing.T) {
	server, cleanup := mockserver.StartTestServer()
	defer cleanup()

	mcpURL := server.MCPURL()

	// Test each of the 15 new tools
	toolTests := []struct {
		name string
		args map[string]interface{}
	}{
		{"json_transform", map[string]interface{}{"operation": "uppercase_keys", "data": map[string]interface{}{"key": "value"}}},
		{"text_processor", map[string]interface{}{"text": "hello", "operation": "uppercase"}},
		{"list_operations", map[string]interface{}{"list": []float64{1, 2, 3}, "operation": "sum"}},
		{"validate_email", map[string]interface{}{"email": "test@example.com"}},
		{"calculate", map[string]interface{}{"expression": "2+2"}},
		{"hash_generator", map[string]interface{}{"data": "test", "algorithm": "sha256"}},
		{"weather_api", map[string]interface{}{"city": "London"}},
		{"geocode", map[string]interface{}{"address": "123 Main St"}},
		{"currency_convert", map[string]interface{}{"amount": 100.0, "from": "USD", "to": "EUR"}},
		{"read_file", map[string]interface{}{"path": "/etc/config.json"}},
		{"write_file", map[string]interface{}{"path": "/tmp/test.txt", "content": "hello"}},
		{"list_directory", map[string]interface{}{"path": "/tmp"}},
		{"large_payload", map[string]interface{}{"size_kb": 1.0}},
		{"random_latency", map[string]interface{}{"min_ms": 10.0, "max_ms": 50.0}},
		{"conditional_error", map[string]interface{}{"error_probability": 0.0}},
	}

	for _, tc := range toolTests {
		t.Run(tc.name, func(t *testing.T) {
			resp := sendMCPRequest(t, mcpURL, "tools/call", map[string]interface{}{
				"name":      tc.name,
				"arguments": tc.args,
			})

			if resp.Error != nil {
				t.Fatalf("tools/call failed for %s: %v", tc.name, resp.Error.Message)
			}

			var result toolsCallResult
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				t.Fatalf("Failed to unmarshal result for %s: %v", tc.name, err)
			}

			if result.IsError {
				t.Errorf("Tool %s returned error: %v", tc.name, result.Content)
			}

			if len(result.Content) == 0 {
				t.Errorf("Tool %s returned empty content", tc.name)
			}

			t.Logf("Tool %s executed successfully", tc.name)
		})
	}
}
