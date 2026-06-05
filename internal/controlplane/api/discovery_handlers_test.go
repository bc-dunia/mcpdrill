package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bc-dunia/mcpdrill/internal/auth"
	"github.com/bc-dunia/mcpdrill/internal/mcp"
	"github.com/bc-dunia/mcpdrill/internal/transport"
)

func TestHandleDiscoverToolsUsesModernDiscovery(t *testing.T) {
	var methods []string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req transport.JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode target request: %v", err)
		}
		methods = append(methods, req.Method)

		switch req.Method {
		case string(transport.OpServerDiscover):
			if got := r.Header.Get(transport.HeaderMCPProtocolVersion); got != mcp.ModernProtocolVersion {
				t.Fatalf("expected modern protocol header, got %q", got)
			}
			writeTargetResult(t, w, req.ID, map[string]interface{}{
				"resultType":        "complete",
				"supportedVersions": []string{mcp.ModernProtocolVersion},
				"capabilities":      map[string]interface{}{},
				"serverInfo":        map[string]interface{}{"name": "modern-only", "version": "1.0"},
			})
		case string(transport.OpToolsList):
			if got := r.Header.Get(transport.HeaderMCPMethod); got != string(transport.OpToolsList) {
				t.Fatalf("expected method header tools/list, got %q", got)
			}
			writeTargetResult(t, w, req.ID, transport.ToolsListResult{Tools: []transport.Tool{{Name: "modern_tool"}}})
		case string(transport.OpInitialize), string(transport.OpInitialized):
			t.Fatalf("modern-only discovery path should not call legacy method %s", req.Method)
		default:
			t.Fatalf("unexpected target method %s", req.Method)
		}
	}))
	defer target.Close()

	server := &Server{
		authConfig:       &auth.Config{Mode: auth.AuthModeNone, InsecureMode: true},
		allowPrivateNets: true,
	}
	body, _ := json.Marshal(map[string]interface{}{"target_url": target.URL})
	req := httptest.NewRequest(http.MethodPost, "/discover-tools", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	server.handleDiscoverTools(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Tools []transport.Tool `json:"tools"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Tools) != 1 || response.Tools[0].Name != "modern_tool" {
		t.Fatalf("unexpected tools response: %+v", response.Tools)
	}
	want := []string{string(transport.OpServerDiscover), string(transport.OpToolsList)}
	if len(methods) != len(want) {
		t.Fatalf("expected methods %v, got %v", want, methods)
	}
	for i := range want {
		if methods[i] != want[i] {
			t.Fatalf("expected methods %v, got %v", want, methods)
		}
	}
}

func TestHandleDiscoverToolsExplicitModernDoesNotFallbackToLegacy(t *testing.T) {
	var methods []string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req transport.JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode target request: %v", err)
		}
		methods = append(methods, req.Method)

		switch req.Method {
		case string(transport.OpServerDiscover):
			w.Header().Set(transport.HeaderContentType, transport.ContentTypeJSON)
			if err := json.NewEncoder(w).Encode(transport.JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &transport.JSONRPCError{Code: -32601, Message: "method not found"},
			}); err != nil {
				t.Fatalf("encode target response: %v", err)
			}
		case string(transport.OpInitialize), string(transport.OpInitialized):
			t.Fatalf("explicit modern discovery must not call legacy method %s", req.Method)
		default:
			t.Fatalf("unexpected target method %s", req.Method)
		}
	}))
	defer target.Close()

	server := &Server{
		authConfig:       &auth.Config{Mode: auth.AuthModeNone, InsecureMode: true},
		allowPrivateNets: true,
	}
	body, _ := json.Marshal(map[string]interface{}{
		"target_url":              target.URL,
		"protocol_version":        mcp.ModernProtocolVersion,
		"protocol_version_policy": "strict",
	})
	req := httptest.NewRequest(http.MethodPost, "/discover-tools", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	server.handleDiscoverTools(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(methods) != 1 || methods[0] != string(transport.OpServerDiscover) {
		t.Fatalf("expected only server/discover, got %v", methods)
	}
}

func TestHandleDiscoverToolsModernPolicyNoneSkipsAdvertisedVersionCheck(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req transport.JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode target request: %v", err)
		}
		switch req.Method {
		case string(transport.OpServerDiscover):
			writeTargetResult(t, w, req.ID, map[string]interface{}{
				"resultType":        "complete",
				"supportedVersions": []string{mcp.ModernProtocolVersion},
				"capabilities":      map[string]interface{}{},
				"serverInfo":        map[string]interface{}{"name": "modern", "version": "1.0"},
			})
		case string(transport.OpToolsList):
			writeTargetResult(t, w, req.ID, transport.ToolsListResult{Tools: []transport.Tool{{Name: "modern_tool"}}})
		default:
			t.Fatalf("unexpected target method %s", req.Method)
		}
	}))
	defer target.Close()

	server := &Server{
		authConfig:       &auth.Config{Mode: auth.AuthModeNone, InsecureMode: true},
		allowPrivateNets: true,
	}
	body, _ := json.Marshal(map[string]interface{}{
		"target_url":              target.URL,
		"protocol_version":        "2026-08-01",
		"protocol_version_policy": "none",
	})
	req := httptest.NewRequest(http.MethodPost, "/discover-tools", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	server.handleDiscoverTools(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func writeTargetResult(t *testing.T, w http.ResponseWriter, id interface{}, result interface{}) {
	t.Helper()
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal target result: %v", err)
	}
	w.Header().Set(transport.HeaderContentType, transport.ContentTypeJSON)
	if err := json.NewEncoder(w).Encode(transport.JSONRPCResponse{JSONRPC: "2.0", ID: id, Result: payload}); err != nil {
		t.Fatalf("encode target response: %v", err)
	}
}
