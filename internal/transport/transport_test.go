package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
		expectedCode ErrorCode
	}{
		{
			name:         "context cancelled",
			err:          context.Canceled,
			expectedType: ErrorTypeCancelled,
			expectedCode: CodeCancelled,
		},
		{
			name:         "context deadline exceeded",
			err:          context.DeadlineExceeded,
			expectedType: ErrorTypeTimeout,
			expectedCode: CodeRequestTimeout,
		},
		{
			name: "DNS lookup failed",
			err: &net.DNSError{
				Err:  "no such host",
				Name: "example.com",
			},
			expectedType: ErrorTypeDNS,
			expectedCode: CodeDNSLookupFailed,
		},
		{
			name: "DNS timeout",
			err: &net.DNSError{
				Err:       "timeout",
				Name:      "example.com",
				IsTimeout: true,
			},
			expectedType: ErrorTypeDNS,
			expectedCode: CodeDNSTimeout,
		},
		{
			name:         "nil error",
			err:          nil,
			expectedType: "",
			expectedCode: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapError(tt.err)
			if tt.err == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			if result.Type != tt.expectedType {
				t.Errorf("expected type %s, got %s", tt.expectedType, result.Type)
			}
			if result.Code != tt.expectedCode {
				t.Errorf("expected code %s, got %s", tt.expectedCode, result.Code)
			}
		})
	}
}

func TestMapHTTPStatus(t *testing.T) {
	tests := []struct {
		status       int
		expectedType ErrorType
		expectedCode ErrorCode
		expectNil    bool
	}{
		{200, "", "", true},
		{201, "", "", true},
		{204, "", "", true},
		{400, ErrorTypeHTTP, CodeHTTPBadRequest, false},
		{401, ErrorTypeHTTP, CodeHTTPUnauthorized, false},
		{403, ErrorTypeHTTP, CodeHTTPForbidden, false},
		{404, ErrorTypeHTTP, CodeHTTPNotFound, false},
		{429, ErrorTypeRateLimited, CodeHTTPRateLimited, false},
		{500, ErrorTypeHTTP, CodeHTTPServerError, false},
		{502, ErrorTypeHTTP, CodeHTTPServerError, false},
		{503, ErrorTypeHTTP, CodeHTTPServerError, false},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			result := MapHTTPStatus(tt.status)
			if tt.expectNil {
				if result != nil {
					t.Errorf("expected nil for status %d, got %v", tt.status, result)
				}
				return
			}
			if result == nil {
				t.Fatalf("expected error for status %d, got nil", tt.status)
			}
			if result.Type != tt.expectedType {
				t.Errorf("expected type %s, got %s", tt.expectedType, result.Type)
			}
			if result.Code != tt.expectedCode {
				t.Errorf("expected code %s, got %s", tt.expectedCode, result.Code)
			}
		})
	}
}

func TestMapJSONRPCError(t *testing.T) {
	tests := []struct {
		code         int
		message      string
		expectedCode ErrorCode
	}{
		{-32700, "Parse error", CodeJSONRPCParseError},
		{-32600, "Invalid Request", CodeJSONRPCInvalidRequest},
		{-32601, "Method not found", CodeJSONRPCMethodNotFound},
		{-32602, "Invalid params", CodeJSONRPCInvalidParams},
		{-32603, "Internal error", CodeJSONRPCInternalError},
		{-32000, "Custom error", "JSONRPC_-32000"},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			result := MapJSONRPCError(tt.code, tt.message, nil)
			if result.Type != ErrorTypeJSONRPC {
				t.Errorf("expected type %s, got %s", ErrorTypeJSONRPC, result.Type)
			}
			if result.Code != tt.expectedCode {
				t.Errorf("expected code %s, got %s", tt.expectedCode, result.Code)
			}
			if result.Message != tt.message {
				t.Errorf("expected message %s, got %s", tt.message, result.Message)
			}
		})
	}
}

func TestSSEDecoder(t *testing.T) {
	t.Run("basic event parsing", func(t *testing.T) {
		data := "data: hello world\n\n"
		decoder := NewSSEDecoder(io.NopCloser(strings.NewReader(data)), 5*time.Second)
		defer decoder.Close()

		event, err := decoder.ReadEvent()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if event.Data != "hello world" {
			t.Errorf("expected data 'hello world', got '%s'", event.Data)
		}
	})

	t.Run("event with id", func(t *testing.T) {
		data := "id: evt_00000001\ndata: test\n\n"
		decoder := NewSSEDecoder(io.NopCloser(strings.NewReader(data)), 5*time.Second)
		defer decoder.Close()

		event, err := decoder.ReadEvent()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if event.ID != "evt_00000001" {
			t.Errorf("expected id 'evt_00000001', got '%s'", event.ID)
		}
		if decoder.LastEventID() != "evt_00000001" {
			t.Errorf("expected last event id 'evt_00000001', got '%s'", decoder.LastEventID())
		}
	})

	t.Run("event with event type", func(t *testing.T) {
		data := "event: message\ndata: test\n\n"
		decoder := NewSSEDecoder(io.NopCloser(strings.NewReader(data)), 5*time.Second)
		defer decoder.Close()

		event, err := decoder.ReadEvent()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if event.Event != "message" {
			t.Errorf("expected event 'message', got '%s'", event.Event)
		}
	})

	t.Run("multiline data", func(t *testing.T) {
		data := "data: line1\ndata: line2\ndata: line3\n\n"
		decoder := NewSSEDecoder(io.NopCloser(strings.NewReader(data)), 5*time.Second)
		defer decoder.Close()

		event, err := decoder.ReadEvent()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "line1\nline2\nline3"
		if event.Data != expected {
			t.Errorf("expected data '%s', got '%s'", expected, event.Data)
		}
	})

	t.Run("comment lines ignored", func(t *testing.T) {
		data := ": this is a comment\ndata: actual data\n\n"
		decoder := NewSSEDecoder(io.NopCloser(strings.NewReader(data)), 5*time.Second)
		defer decoder.Close()

		event, err := decoder.ReadEvent()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if event.Data != "actual data" {
			t.Errorf("expected data 'actual data', got '%s'", event.Data)
		}
	})

	t.Run("retry field", func(t *testing.T) {
		data := "retry: 5000\ndata: test\n\n"
		decoder := NewSSEDecoder(io.NopCloser(strings.NewReader(data)), 5*time.Second)
		defer decoder.Close()

		event, err := decoder.ReadEvent()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if event.Retry != 5000 {
			t.Errorf("expected retry 5000, got %d", event.Retry)
		}
	})

	t.Run("multiple events", func(t *testing.T) {
		data := "data: event1\n\ndata: event2\n\n"
		decoder := NewSSEDecoder(io.NopCloser(strings.NewReader(data)), 5*time.Second)
		defer decoder.Close()

		event1, err := decoder.ReadEvent()
		if err != nil {
			t.Fatalf("unexpected error reading event1: %v", err)
		}
		if event1.Data != "event1" {
			t.Errorf("expected data 'event1', got '%s'", event1.Data)
		}

		event2, err := decoder.ReadEvent()
		if err != nil {
			t.Fatalf("unexpected error reading event2: %v", err)
		}
		if event2.Data != "event2" {
			t.Errorf("expected data 'event2', got '%s'", event2.Data)
		}
	})

	t.Run("EOF handling", func(t *testing.T) {
		data := "data: final\n\n"
		decoder := NewSSEDecoder(io.NopCloser(strings.NewReader(data)), 5*time.Second)
		defer decoder.Close()

		_, err := decoder.ReadEvent()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = decoder.ReadEvent()
		if err != io.EOF {
			t.Errorf("expected EOF, got %v", err)
		}
	})

	t.Run("EOF handling without stall timeout", func(t *testing.T) {
		data := "data: final\n\n"
		decoder := NewSSEDecoder(io.NopCloser(strings.NewReader(data)), 0)
		defer decoder.Close()

		_, err := decoder.ReadEvent()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = decoder.ReadEvent()
		if err != io.EOF {
			t.Errorf("expected EOF, got %v", err)
		}
	})

	t.Run("EOF handling with unterminated final event", func(t *testing.T) {
		data := "data: final"
		decoder := NewSSEDecoder(io.NopCloser(strings.NewReader(data)), 5*time.Second)
		defer decoder.Close()

		event, err := decoder.ReadEvent()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if event.Data != "final" {
			t.Fatalf("expected data 'final', got %q", event.Data)
		}

		_, err = decoder.ReadEvent()
		if err != io.EOF {
			t.Errorf("expected EOF, got %v", err)
		}
	})

	t.Run("id with null byte ignored", func(t *testing.T) {
		data := "id: bad\x00id\ndata: test\n\n"
		decoder := NewSSEDecoder(io.NopCloser(strings.NewReader(data)), 5*time.Second)
		defer decoder.Close()

		event, err := decoder.ReadEvent()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if event.ID != "" {
			t.Errorf("expected empty id (null byte should be rejected), got '%s'", event.ID)
		}
	})
}

func TestParseSSEFromBytes(t *testing.T) {
	data := []byte("data: event1\n\ndata: event2\n\n")
	events, err := ParseSSEFromBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Data != "event1" {
		t.Errorf("expected first event data 'event1', got '%s'", events[0].Data)
	}
	if events[1].Data != "event2" {
		t.Errorf("expected second event data 'event2', got '%s'", events[1].Data)
	}
}

func TestMCPOperations(t *testing.T) {
	t.Run("NewInitializeRequest", func(t *testing.T) {
		req := NewInitializeRequest("init_001", nil)
		if req.JSONRPC != "2.0" {
			t.Errorf("expected jsonrpc '2.0', got '%s'", req.JSONRPC)
		}
		if req.ID != "init_001" {
			t.Errorf("expected id 'init_001', got '%v'", req.ID)
		}
		if req.Method != string(OpInitialize) {
			t.Errorf("expected method '%s', got '%s'", OpInitialize, req.Method)
		}
		params, ok := req.Params.(InitializeParams)
		if !ok {
			t.Fatalf("expected InitializeParams, got %T", req.Params)
		}
		if params.ProtocolVersion != MCPProtocolVersion {
			t.Errorf("expected protocol version '%s', got '%s'", MCPProtocolVersion, params.ProtocolVersion)
		}
	})

	t.Run("NewInitializeRequest_CustomParams", func(t *testing.T) {
		customParams := &InitializeParams{
			ProtocolVersion: "2025-03-26",
			Capabilities:    map[string]interface{}{"test": true},
			ClientInfo: ClientInfo{
				Name:    "test-client",
				Version: "2.0.0",
			},
		}
		req := NewInitializeRequest("init_002", customParams)
		params, ok := req.Params.(InitializeParams)
		if !ok {
			t.Fatalf("expected InitializeParams, got %T", req.Params)
		}
		if params.ProtocolVersion != "2025-03-26" {
			t.Errorf("expected protocol version '2025-03-26', got '%s'", params.ProtocolVersion)
		}
		if params.ClientInfo.Name != "test-client" {
			t.Errorf("expected client name 'test-client', got '%s'", params.ClientInfo.Name)
		}
	})

	t.Run("NewInitializedNotification", func(t *testing.T) {
		req := NewInitializedNotification()
		if req.ID != nil {
			t.Errorf("notification should have nil id, got %v", req.ID)
		}
		if req.Method != string(OpInitialized) {
			t.Errorf("expected method '%s', got '%s'", OpInitialized, req.Method)
		}
	})

	t.Run("NewToolsListRequest", func(t *testing.T) {
		req := NewToolsListRequest("tl_001", nil)
		if req.ID != "tl_001" {
			t.Errorf("expected id 'tl_001', got '%v'", req.ID)
		}
		if req.Method != string(OpToolsList) {
			t.Errorf("expected method '%s', got '%s'", OpToolsList, req.Method)
		}
	})

	t.Run("NewToolsListRequest with cursor", func(t *testing.T) {
		cursor := "next_page"
		req := NewToolsListRequest("tl_002", &cursor)
		params, ok := req.Params.(map[string]interface{})
		if !ok {
			t.Fatalf("expected map params, got %T", req.Params)
		}
		if params["cursor"] != cursor {
			t.Errorf("expected cursor '%s', got '%v'", cursor, params["cursor"])
		}
	})

	t.Run("NewToolsCallRequest", func(t *testing.T) {
		args := map[string]interface{}{"message": "hello"}
		req := NewToolsCallRequest("tc_001", "echo", args)
		if req.ID != "tc_001" {
			t.Errorf("expected id 'tc_001', got '%v'", req.ID)
		}
		if req.Method != string(OpToolsCall) {
			t.Errorf("expected method '%s', got '%s'", OpToolsCall, req.Method)
		}
		params, ok := req.Params.(ToolsCallParams)
		if !ok {
			t.Fatalf("expected ToolsCallParams, got %T", req.Params)
		}
		if params.Name != "echo" {
			t.Errorf("expected tool name 'echo', got '%s'", params.Name)
		}
	})

	t.Run("NewPingRequest", func(t *testing.T) {
		req := NewPingRequest("ping_001")
		if req.ID != "ping_001" {
			t.Errorf("expected id 'ping_001', got '%v'", req.ID)
		}
		if req.Method != string(OpPing) {
			t.Errorf("expected method '%s', got '%s'", OpPing, req.Method)
		}
	})
}

func TestValidateJSONRPCResponse(t *testing.T) {
	t.Run("valid response", func(t *testing.T) {
		resp := &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      "req_001",
			Result:  json.RawMessage(`{}`),
		}
		err := ValidateJSONRPCResponse(resp, "req_001")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("invalid jsonrpc version", func(t *testing.T) {
		resp := &JSONRPCResponse{
			JSONRPC: "1.0",
			ID:      "req_001",
			Result:  json.RawMessage(`{}`),
		}
		err := ValidateJSONRPCResponse(resp, "req_001")
		if err == nil {
			t.Error("expected error for invalid jsonrpc version")
		}
		if err.Code != CodeInvalidJSONRPC {
			t.Errorf("expected code %s, got %s", CodeInvalidJSONRPC, err.Code)
		}
	})

	t.Run("missing id", func(t *testing.T) {
		resp := &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      nil,
			Result:  json.RawMessage(`{}`),
		}
		err := ValidateJSONRPCResponse(resp, "req_001")
		if err == nil {
			t.Error("expected error for missing id")
		}
		if err.Code != CodeMissingID {
			t.Errorf("expected code %s, got %s", CodeMissingID, err.Code)
		}
	})

	t.Run("id mismatch", func(t *testing.T) {
		resp := &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      "req_002",
			Result:  json.RawMessage(`{}`),
		}
		err := ValidateJSONRPCResponse(resp, "req_001")
		if err == nil {
			t.Error("expected error for id mismatch")
		}
		if err.Code != CodeIDMismatch {
			t.Errorf("expected code %s, got %s", CodeIDMismatch, err.Code)
		}
	})
}

func TestParseResults(t *testing.T) {
	t.Run("ParseInitializeResult", func(t *testing.T) {
		data, _ := os.ReadFile("../../testdata/transport/initialize_response.json")
		var resp JSONRPCResponse
		json.Unmarshal(data, &resp)

		result, err := ParseInitializeResult(resp.Result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ProtocolVersion != "2025-11-25" {
			t.Errorf("expected protocol version '2025-11-25', got '%s'", result.ProtocolVersion)
		}
		if result.ServerInfo.Name != "test-server" {
			t.Errorf("expected server name 'test-server', got '%s'", result.ServerInfo.Name)
		}
	})

	t.Run("ParseToolsListResult", func(t *testing.T) {
		data, _ := os.ReadFile("../../testdata/transport/tools_list_response.json")
		var resp JSONRPCResponse
		json.Unmarshal(data, &resp)

		result, err := ParseToolsListResult(resp.Result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Tools) != 2 {
			t.Errorf("expected 2 tools, got %d", len(result.Tools))
		}
		if result.Tools[0].Name != "echo" {
			t.Errorf("expected first tool name 'echo', got '%s'", result.Tools[0].Name)
		}
	})

	t.Run("ParseToolsCallResult", func(t *testing.T) {
		data, _ := os.ReadFile("../../testdata/transport/tools_call_response.json")
		var resp JSONRPCResponse
		json.Unmarshal(data, &resp)

		result, err := ParseToolsCallResult(resp.Result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Content) != 1 {
			t.Errorf("expected 1 content item, got %d", len(result.Content))
		}
		if result.Content[0].Text != "hello world" {
			t.Errorf("expected text 'hello world', got '%s'", result.Content[0].Text)
		}
		if result.IsError {
			t.Error("expected IsError to be false")
		}
	})

	t.Run("ParseToolsCallResult with error", func(t *testing.T) {
		data, _ := os.ReadFile("../../testdata/transport/tools_call_error_response.json")
		var resp JSONRPCResponse
		json.Unmarshal(data, &resp)

		result, err := ParseToolsCallResult(resp.Result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Error("expected IsError to be true")
		}
	})
}

func TestCheckToolError(t *testing.T) {
	t.Run("no error", func(t *testing.T) {
		result := &ToolsCallResult{
			Content: []ToolContent{{Type: "text", Text: "success"}},
			IsError: false,
		}
		err := CheckToolError(result, "echo")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("tool error", func(t *testing.T) {
		result := &ToolsCallResult{
			Content: []ToolContent{{Type: "text", Text: "failed"}},
			IsError: true,
		}
		err := CheckToolError(result, "echo")
		if err == nil {
			t.Error("expected error")
		}
		if err.Type != ErrorTypeTool {
			t.Errorf("expected type %s, got %s", ErrorTypeTool, err.Type)
		}
	})
}

func TestStreamableHTTPAdapter(t *testing.T) {
	adapter := NewStreamableHTTPAdapter()

	t.Run("ID", func(t *testing.T) {
		if adapter.ID() != TransportIDStreamableHTTP {
			t.Errorf("expected id '%s', got '%s'", TransportIDStreamableHTTP, adapter.ID())
		}
	})
}

func TestStreamableHTTPConnection(t *testing.T) {
	t.Run("Initialize success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if r.Header.Get(HeaderContentType) != ContentTypeJSON {
				t.Errorf("expected content-type %s", ContentTypeJSON)
			}
			if r.Header.Get(HeaderAccept) != AcceptBoth {
				t.Errorf("expected accept %s", AcceptBoth)
			}

			var req JSONRPCRequest
			json.NewDecoder(r.Body).Decode(&req)
			if req.Method != string(OpInitialize) {
				t.Errorf("expected method %s, got %s", OpInitialize, req.Method)
			}

			w.Header().Set(HeaderMCPSessionID, "ses_test123")
			w.Header().Set(HeaderContentType, ContentTypeJSON)
			json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: json.RawMessage(`{
					"protocolVersion": "2025-11-25",
					"capabilities": {},
					"serverInfo": {"name": "test", "version": "1.0"}
				}`),
			})
		}))
		defer server.Close()

		adapter := NewStreamableHTTPAdapter()
		config := &TransportConfig{
			AllowPrivateNetworks: []string{"127.0.0.0/8"},
			Endpoint:             server.URL,
			Timeouts:             DefaultTimeoutConfig(),
		}

		conn, err := adapter.Connect(context.Background(), config)
		if err != nil {
			t.Fatalf("connect failed: %v", err)
		}
		defer conn.Close()

		outcome, err := conn.Initialize(context.Background(), nil)
		if err != nil {
			t.Fatalf("initialize failed: %v", err)
		}
		if !outcome.OK {
			t.Errorf("expected OK, got error: %v", outcome.Error)
		}
		if outcome.SessionID != "ses_test123" {
			t.Errorf("expected session id 'ses_test123', got '%s'", outcome.SessionID)
		}
		if conn.SessionID() != "ses_test123" {
			t.Errorf("expected connection session id 'ses_test123', got '%s'", conn.SessionID())
		}
	})

	t.Run("SendInitialized success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req JSONRPCRequest
			json.NewDecoder(r.Body).Decode(&req)
			if req.Method != string(OpInitialized) {
				t.Errorf("expected method %s, got %s", OpInitialized, req.Method)
			}
			if req.ID != nil {
				t.Errorf("notification should have nil id, got %v", req.ID)
			}
			w.WriteHeader(http.StatusAccepted)
		}))
		defer server.Close()

		adapter := NewStreamableHTTPAdapter()
		config := &TransportConfig{
			AllowPrivateNetworks: []string{"127.0.0.0/8"},
			Endpoint:             server.URL,
			Timeouts:             DefaultTimeoutConfig(),
		}

		conn, _ := adapter.Connect(context.Background(), config)
		defer conn.Close()

		outcome, err := conn.SendInitialized(context.Background())
		if err != nil {
			t.Fatalf("send initialized failed: %v", err)
		}
		if !outcome.OK {
			t.Errorf("expected OK, got error: %v", outcome.Error)
		}
	})

	t.Run("ToolsList success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req JSONRPCRequest
			json.NewDecoder(r.Body).Decode(&req)
			if req.Method != string(OpToolsList) {
				t.Errorf("expected method %s, got %s", OpToolsList, req.Method)
			}

			w.Header().Set(HeaderContentType, ContentTypeJSON)
			json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`{"tools": [{"name": "echo"}]}`),
			})
		}))
		defer server.Close()

		adapter := NewStreamableHTTPAdapter()
		config := &TransportConfig{
			AllowPrivateNetworks: []string{"127.0.0.0/8"},
			Endpoint:             server.URL,
			Timeouts:             DefaultTimeoutConfig(),
		}

		conn, _ := adapter.Connect(context.Background(), config)
		defer conn.Close()

		outcome, err := conn.ToolsList(context.Background(), nil)
		if err != nil {
			t.Fatalf("tools list failed: %v", err)
		}
		if !outcome.OK {
			t.Errorf("expected OK, got error: %v", outcome.Error)
		}
	})

	t.Run("ToolsCall JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req JSONRPCRequest
			json.NewDecoder(r.Body).Decode(&req)
			if req.Method != string(OpToolsCall) {
				t.Errorf("expected method %s, got %s", OpToolsCall, req.Method)
			}

			w.Header().Set(HeaderContentType, ContentTypeJSON)
			json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`{"content": [{"type": "text", "text": "hello"}]}`),
			})
		}))
		defer server.Close()

		adapter := NewStreamableHTTPAdapter()
		config := &TransportConfig{
			AllowPrivateNetworks: []string{"127.0.0.0/8"},
			Endpoint:             server.URL,
			Timeouts:             DefaultTimeoutConfig(),
		}

		conn, _ := adapter.Connect(context.Background(), config)
		defer conn.Close()

		outcome, err := conn.ToolsCall(context.Background(), &ToolsCallParams{
			Name:      "echo",
			Arguments: map[string]interface{}{"message": "hello"},
		})
		if err != nil {
			t.Fatalf("tools call failed: %v", err)
		}
		if !outcome.OK {
			t.Errorf("expected OK, got error: %v", outcome.Error)
		}
		if outcome.ToolName != "echo" {
			t.Errorf("expected tool name 'echo', got '%s'", outcome.ToolName)
		}
	})

	t.Run("ToolsCall SSE response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req JSONRPCRequest
			json.NewDecoder(r.Body).Decode(&req)

			w.Header().Set(HeaderContentType, ContentTypeSSE)
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("expected flusher")
			}

			w.Write([]byte(`data: {"jsonrpc":"2.0","method":"notifications/progress","params":{"progress":50}}` + "\n\n"))
			flusher.Flush()

			resp := JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`{"content": [{"type": "text", "text": "streamed"}]}`),
			}
			respBytes, _ := json.Marshal(resp)
			w.Write([]byte("data: " + string(respBytes) + "\n\n"))
			flusher.Flush()
		}))
		defer server.Close()

		adapter := NewStreamableHTTPAdapter()
		config := &TransportConfig{
			AllowPrivateNetworks: []string{"127.0.0.0/8"},
			Endpoint:             server.URL,
			Timeouts:             DefaultTimeoutConfig(),
		}

		conn, _ := adapter.Connect(context.Background(), config)
		defer conn.Close()

		outcome, err := conn.ToolsCall(context.Background(), &ToolsCallParams{
			Name: "stream_tool",
		})
		if err != nil {
			t.Fatalf("tools call failed: %v", err)
		}
		if !outcome.OK {
			t.Errorf("expected OK, got error: %v", outcome.Error)
		}
		if outcome.Stream == nil {
			t.Error("expected stream signals")
		} else {
			if !outcome.Stream.IsStreaming {
				t.Error("expected IsStreaming to be true")
			}
			if outcome.Stream.EventsCount < 2 {
				t.Errorf("expected at least 2 events, got %d", outcome.Stream.EventsCount)
			}
		}
	})

	t.Run("Ping success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req JSONRPCRequest
			json.NewDecoder(r.Body).Decode(&req)
			if req.Method != string(OpPing) {
				t.Errorf("expected method %s, got %s", OpPing, req.Method)
			}

			w.Header().Set(HeaderContentType, ContentTypeJSON)
			json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`{}`),
			})
		}))
		defer server.Close()

		adapter := NewStreamableHTTPAdapter()
		config := &TransportConfig{
			AllowPrivateNetworks: []string{"127.0.0.0/8"},
			Endpoint:             server.URL,
			Timeouts:             DefaultTimeoutConfig(),
		}

		conn, _ := adapter.Connect(context.Background(), config)
		defer conn.Close()

		outcome, err := conn.Ping(context.Background())
		if err != nil {
			t.Fatalf("ping failed: %v", err)
		}
		if !outcome.OK {
			t.Errorf("expected OK, got error: %v", outcome.Error)
		}
	})

	t.Run("HTTP error handling", func(t *testing.T) {
		tests := []struct {
			status       int
			expectedType ErrorType
		}{
			{400, ErrorTypeHTTP},
			{401, ErrorTypeHTTP},
			{403, ErrorTypeHTTP},
			{404, ErrorTypeHTTP},
			{429, ErrorTypeRateLimited},
			{500, ErrorTypeHTTP},
			{503, ErrorTypeHTTP},
		}

		for _, tt := range tests {
			t.Run(http.StatusText(tt.status), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tt.status)
				}))
				defer server.Close()

				adapter := NewStreamableHTTPAdapter()
				config := &TransportConfig{
					AllowPrivateNetworks: []string{"127.0.0.0/8"},
					Endpoint:             server.URL,
					Timeouts:             DefaultTimeoutConfig(),
				}

				conn, _ := adapter.Connect(context.Background(), config)
				defer conn.Close()

				outcome, _ := conn.Ping(context.Background())
				if outcome.OK {
					t.Error("expected error")
				}
				if outcome.Error.Type != tt.expectedType {
					t.Errorf("expected type %s, got %s", tt.expectedType, outcome.Error.Type)
				}
			})
		}
	})

	t.Run("JSON-RPC error handling", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req JSONRPCRequest
			json.NewDecoder(r.Body).Decode(&req)

			w.Header().Set(HeaderContentType, ContentTypeJSON)
			json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &JSONRPCError{
					Code:    -32601,
					Message: "Method not found",
				},
			})
		}))
		defer server.Close()

		adapter := NewStreamableHTTPAdapter()
		config := &TransportConfig{
			AllowPrivateNetworks: []string{"127.0.0.0/8"},
			Endpoint:             server.URL,
			Timeouts:             DefaultTimeoutConfig(),
		}

		conn, _ := adapter.Connect(context.Background(), config)
		defer conn.Close()

		outcome, _ := conn.Ping(context.Background())
		if outcome.OK {
			t.Error("expected error")
		}
		if outcome.Error.Type != ErrorTypeJSONRPC {
			t.Errorf("expected type %s, got %s", ErrorTypeJSONRPC, outcome.Error.Type)
		}
		if outcome.JSONRPCErrorCode == nil || *outcome.JSONRPCErrorCode != -32601 {
			t.Errorf("expected jsonrpc error code -32601")
		}
	})

	t.Run("Tool error handling", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req JSONRPCRequest
			json.NewDecoder(r.Body).Decode(&req)

			w.Header().Set(HeaderContentType, ContentTypeJSON)
			json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`{"content": [{"type": "text", "text": "error"}], "isError": true}`),
			})
		}))
		defer server.Close()

		adapter := NewStreamableHTTPAdapter()
		config := &TransportConfig{
			AllowPrivateNetworks: []string{"127.0.0.0/8"},
			Endpoint:             server.URL,
			Timeouts:             DefaultTimeoutConfig(),
		}

		conn, _ := adapter.Connect(context.Background(), config)
		defer conn.Close()

		outcome, _ := conn.ToolsCall(context.Background(), &ToolsCallParams{Name: "failing_tool"})
		if outcome.OK {
			t.Error("expected error")
		}
		if outcome.Error.Type != ErrorTypeTool {
			t.Errorf("expected type %s, got %s", ErrorTypeTool, outcome.Error.Type)
		}
	})

	t.Run("Session ID propagation", func(t *testing.T) {
		var receivedSessionID string
		callCount := 0

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			receivedSessionID = r.Header.Get(HeaderMCPSessionID)

			var req JSONRPCRequest
			json.NewDecoder(r.Body).Decode(&req)

			if callCount == 1 {
				w.Header().Set(HeaderMCPSessionID, "ses_abc123")
			}

			w.Header().Set(HeaderContentType, ContentTypeJSON)
			json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`{}`),
			})
		}))
		defer server.Close()

		adapter := NewStreamableHTTPAdapter()
		config := &TransportConfig{
			AllowPrivateNetworks: []string{"127.0.0.0/8"},
			Endpoint:             server.URL,
			Timeouts:             DefaultTimeoutConfig(),
		}

		conn, _ := adapter.Connect(context.Background(), config)
		defer conn.Close()

		conn.Initialize(context.Background(), nil)

		if conn.SessionID() != "ses_abc123" {
			t.Errorf("expected session id 'ses_abc123', got '%s'", conn.SessionID())
		}

		conn.Ping(context.Background())

		if receivedSessionID != "ses_abc123" {
			t.Errorf("expected session id header 'ses_abc123', got '%s'", receivedSessionID)
		}
	})

	t.Run("Custom headers", func(t *testing.T) {
		var receivedAuth string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAuth = r.Header.Get(HeaderAuthorization)

			var req JSONRPCRequest
			json.NewDecoder(r.Body).Decode(&req)

			w.Header().Set(HeaderContentType, ContentTypeJSON)
			json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`{}`),
			})
		}))
		defer server.Close()

		adapter := NewStreamableHTTPAdapter()
		config := &TransportConfig{
			AllowPrivateNetworks: []string{"127.0.0.0/8"},
			Endpoint:             server.URL,
			Timeouts:             DefaultTimeoutConfig(),
			Headers: map[string]string{
				HeaderAuthorization: "Bearer test_token",
			},
		}

		conn, _ := adapter.Connect(context.Background(), config)
		defer conn.Close()

		conn.Ping(context.Background())

		if receivedAuth != "Bearer test_token" {
			t.Errorf("expected auth header 'Bearer test_token', got '%s'", receivedAuth)
		}
	})

	t.Run("Request timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		adapter := NewStreamableHTTPAdapter()
		config := &TransportConfig{
			AllowPrivateNetworks: []string{"127.0.0.0/8"},
			Endpoint:             server.URL,
			Timeouts: TimeoutConfig{
				ConnectTimeout:     5 * time.Second,
				RequestTimeout:     50 * time.Millisecond,
				StreamStallTimeout: 5 * time.Second,
			},
		}

		conn, _ := adapter.Connect(context.Background(), config)
		defer conn.Close()

		outcome, _ := conn.Ping(context.Background())
		if outcome.OK {
			t.Error("expected timeout error")
		}
		if outcome.Error.Type != ErrorTypeTimeout && outcome.Error.Type != ErrorTypeCancelled {
			t.Errorf("expected timeout or cancelled error, got %s", outcome.Error.Type)
		}
	})

	t.Run("Context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(5 * time.Second)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		adapter := NewStreamableHTTPAdapter()
		config := &TransportConfig{
			AllowPrivateNetworks: []string{"127.0.0.0/8"},
			Endpoint:             server.URL,
			Timeouts:             DefaultTimeoutConfig(),
		}

		conn, _ := adapter.Connect(context.Background(), config)
		defer conn.Close()

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		outcome, _ := conn.Ping(ctx)
		if outcome.OK {
			t.Error("expected cancellation error")
		}
	})

	t.Run("Connection close", func(t *testing.T) {
		adapter := NewStreamableHTTPAdapter()
		config := &TransportConfig{
			AllowPrivateNetworks: []string{"127.0.0.0/8"},
			Endpoint:             "http://localhost:9999",
			Timeouts:             DefaultTimeoutConfig(),
		}

		conn, _ := adapter.Connect(context.Background(), config)

		err := conn.Close()
		if err != nil {
			t.Errorf("unexpected error on close: %v", err)
		}

		err = conn.Close()
		if err != nil {
			t.Errorf("double close should not error: %v", err)
		}
	})
}

func TestConcurrentOperations(t *testing.T) {
	var requestCount int32
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()

		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		time.Sleep(10 * time.Millisecond)

		w.Header().Set(HeaderContentType, ContentTypeJSON)
		json.NewEncoder(w).Encode(JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{}`),
		})
	}))
	defer server.Close()

	adapter := NewStreamableHTTPAdapter()
	config := &TransportConfig{
		AllowPrivateNetworks: []string{"127.0.0.0/8"},
		Endpoint:             server.URL,
		Timeouts:             DefaultTimeoutConfig(),
	}

	conn, _ := adapter.Connect(context.Background(), config)
	defer conn.Close()

	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outcome, _ := conn.Ping(context.Background())
			if !outcome.OK {
				t.Errorf("ping failed: %v", outcome.Error)
			}
		}()
	}

	wg.Wait()

	mu.Lock()
	if int(requestCount) != numGoroutines {
		t.Errorf("expected %d requests, got %d", numGoroutines, requestCount)
	}
	mu.Unlock()
}

func TestSSEStreamStall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set(HeaderContentType, ContentTypeSSE)
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}

		w.Write([]byte(`data: {"jsonrpc":"2.0","method":"notifications/progress","params":{"progress":50}}` + "\n\n"))
		flusher.Flush()

		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	adapter := NewStreamableHTTPAdapter()
	config := &TransportConfig{
		AllowPrivateNetworks: []string{"127.0.0.0/8"},
		Endpoint:             server.URL,
		Timeouts: TimeoutConfig{
			ConnectTimeout:     5 * time.Second,
			RequestTimeout:     5 * time.Second,
			StreamStallTimeout: 50 * time.Millisecond,
		},
	}

	conn, _ := adapter.Connect(context.Background(), config)
	defer conn.Close()

	outcome, _ := conn.ToolsCall(context.Background(), &ToolsCallParams{Name: "stalling_tool"})
	if outcome.OK {
		t.Error("expected stall error")
	}
	if outcome.Stream == nil {
		t.Fatal("expected stream signals")
	}
	if !outcome.Stream.Stalled {
		t.Error("expected Stalled to be true")
	}
}

func TestMapSyscallErrors(t *testing.T) {
	tests := []struct {
		errno        syscall.Errno
		expectedType ErrorType
		expectedCode ErrorCode
	}{
		{syscall.ECONNREFUSED, ErrorTypeConnect, CodeConnectionRefused},
		{syscall.ECONNRESET, ErrorTypeConnect, CodeConnectionReset},
		{syscall.ENETUNREACH, ErrorTypeConnect, CodeNetworkUnreachable},
		{syscall.ETIMEDOUT, ErrorTypeTimeout, CodeConnectTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.errno.Error(), func(t *testing.T) {
			opErr := &net.OpError{
				Op:   "dial",
				Net:  "tcp",
				Addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080},
				Err:  tt.errno,
			}
			result := MapError(opErr)
			if result.Type != tt.expectedType {
				t.Errorf("expected type %s, got %s", tt.expectedType, result.Type)
			}
			if result.Code != tt.expectedCode {
				t.Errorf("expected code %s, got %s", tt.expectedCode, result.Code)
			}
		})
	}
}

func TestIsNotification(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected bool
	}{
		{
			name:     "notification",
			data:     `{"jsonrpc":"2.0","method":"notifications/progress","params":{}}`,
			expected: true,
		},
		{
			name:     "request with id",
			data:     `{"jsonrpc":"2.0","id":"1","method":"ping","params":{}}`,
			expected: false,
		},
		{
			name:     "response",
			data:     `{"jsonrpc":"2.0","id":"1","result":{}}`,
			expected: false,
		},
		{
			name:     "invalid json",
			data:     `not json`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNotification([]byte(tt.data))
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestBufferedSSEReader(t *testing.T) {
	events := []*SSEEvent{
		{Data: "event1"},
		{Data: "event2"},
		{ID: "id3", Data: "event3"},
	}

	reader := NewBufferedSSEReader(events)

	for i, expected := range events {
		event, err := reader.ReadEvent()
		if err != nil {
			t.Fatalf("unexpected error at event %d: %v", i, err)
		}
		if event.Data != expected.Data {
			t.Errorf("event %d: expected data '%s', got '%s'", i, expected.Data, event.Data)
		}
	}

	_, err := reader.ReadEvent()
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}

	if err := reader.Close(); err != nil {
		t.Errorf("unexpected error on close: %v", err)
	}
}

func TestOperationOutcomeFields(t *testing.T) {
	outcome := &OperationOutcome{
		Operation: OpToolsCall,
		ToolName:  "echo",
		JSONRPCID: "req_001",
		Transport: TransportIDStreamableHTTP,
		OK:        true,
	}

	if outcome.Operation != OpToolsCall {
		t.Errorf("expected operation %s, got %s", OpToolsCall, outcome.Operation)
	}
	if outcome.ToolName != "echo" {
		t.Errorf("expected tool name 'echo', got '%s'", outcome.ToolName)
	}
	if outcome.Transport != TransportIDStreamableHTTP {
		t.Errorf("expected transport %s, got %s", TransportIDStreamableHTTP, outcome.Transport)
	}
}

func TestOperationErrorInterface(t *testing.T) {
	opErr := &OperationError{
		Type:    ErrorTypeTimeout,
		Code:    CodeRequestTimeout,
		Message: "request timed out",
	}

	var err error = opErr
	if err.Error() != "request timed out" {
		t.Errorf("expected error message 'request timed out', got '%s'", err.Error())
	}

	var opErr2 *OperationError
	if !errors.As(err, &opErr2) {
		t.Error("expected errors.As to work with OperationError")
	}
}

func TestDefaultTimeoutConfig(t *testing.T) {
	config := DefaultTimeoutConfig()

	if config.ConnectTimeout != 5*time.Second {
		t.Errorf("expected connect timeout 5s, got %v", config.ConnectTimeout)
	}
	if config.RequestTimeout != 30*time.Second {
		t.Errorf("expected request timeout 30s, got %v", config.RequestTimeout)
	}
	if config.StreamStallTimeout != 15*time.Second {
		t.Errorf("expected stream stall timeout 15s, got %v", config.StreamStallTimeout)
	}
}

func TestNewRequestContext(t *testing.T) {
	ctx := context.Background()
	reqCtx := NewRequestContext(ctx, "req_001", "ses_001")

	if reqCtx.Ctx != ctx {
		t.Error("context mismatch")
	}
	if reqCtx.RequestID != "req_001" {
		t.Errorf("expected request id 'req_001', got '%s'", reqCtx.RequestID)
	}
	if reqCtx.SessionID != "ses_001" {
		t.Errorf("expected session id 'ses_001', got '%s'", reqCtx.SessionID)
	}
}

func TestSSEDecoderClose(t *testing.T) {
	data := "data: test\n\n"
	decoder := NewSSEDecoder(io.NopCloser(strings.NewReader(data)), 5*time.Second)

	if err := decoder.Close(); err != nil {
		t.Errorf("unexpected error on close: %v", err)
	}

	_, err := decoder.ReadEvent()
	if err != ErrStreamClosed {
		t.Errorf("expected ErrStreamClosed, got %v", err)
	}

	if err := decoder.Close(); err != nil {
		t.Errorf("double close should not error: %v", err)
	}
}

func TestSSEResponseHandlerWithNotifications(t *testing.T) {
	sseData := `data: {"jsonrpc":"2.0","method":"notifications/progress","params":{"progressToken":"tc_001","progress":25,"total":100}}

data: {"jsonrpc":"2.0","method":"notifications/progress","params":{"progressToken":"tc_001","progress":75,"total":100}}

data: {"jsonrpc":"2.0","id":"tc_001","result":{"content":[{"type":"text","text":"done"}]}}

`
	handler := NewSSEResponseHandler(5 * time.Second)
	body := io.NopCloser(bytes.NewReader([]byte(sseData)))

	resp, signals, err := handler.HandleSSEStream(context.Background(), body, "tc_001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp == nil {
		t.Fatal("expected response")
	}

	if signals.EventsCount != 3 {
		t.Errorf("expected 3 events, got %d", signals.EventsCount)
	}

	if !signals.EndedNormally {
		t.Error("expected stream to end normally")
	}
}

func TestMapMCPError(t *testing.T) {
	err := MapMCPError("TOOL_NOT_FOUND", "Tool 'unknown' not found")

	if err.Type != ErrorTypeMCP {
		t.Errorf("expected type %s, got %s", ErrorTypeMCP, err.Type)
	}
	if err.Code != CodeMCPError {
		t.Errorf("expected code %s, got %s", CodeMCPError, err.Code)
	}
	if err.Details["mcp_error_code"] != "TOOL_NOT_FOUND" {
		t.Errorf("expected mcp_error_code 'TOOL_NOT_FOUND', got '%v'", err.Details["mcp_error_code"])
	}
}

func TestMapToolError(t *testing.T) {
	content := []ToolContent{
		{Type: "text", Text: "Error: something went wrong"},
		{Type: "text", Text: "Details: more info"},
	}

	err := MapToolError("failing_tool", content)

	if err.Type != ErrorTypeTool {
		t.Errorf("expected type %s, got %s", ErrorTypeTool, err.Type)
	}
	if err.Details["tool_name"] != "failing_tool" {
		t.Errorf("expected tool_name 'failing_tool', got '%v'", err.Details["tool_name"])
	}

	messages, ok := err.Details["content"].([]string)
	if !ok {
		t.Fatal("expected content to be []string")
	}
	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}
}

func TestNewStreamStallError(t *testing.T) {
	err := NewStreamStallError(15000)

	if err.Type != ErrorTypeStreamStall {
		t.Errorf("expected type %s, got %s", ErrorTypeStreamStall, err.Type)
	}
	if err.Code != CodeStreamStallTimeout {
		t.Errorf("expected code %s, got %s", CodeStreamStallTimeout, err.Code)
	}
	if err.Details["stall_duration_ms"] != 15000 {
		t.Errorf("expected stall_duration_ms 15000, got %v", err.Details["stall_duration_ms"])
	}
}
