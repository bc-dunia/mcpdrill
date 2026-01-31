package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/transport"
)

func validateTargetURL(urlStr string) (string, error) {
	if urlStr == "" {
		return "", fmt.Errorf("target_url is required")
	}

	parsed, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %v", err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("only http and https schemes are allowed")
	}

	if parsed.User != nil {
		return "", fmt.Errorf("URLs with embedded credentials are not allowed")
	}

	if parsed.Hostname() == "" {
		return "", fmt.Errorf("URL must have a valid host")
	}

	return parsed.String(), nil
}

func (s *Server) handleDiscoverTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w, r.Method, "POST")
		return
	}

	var req struct {
		TargetURL string            `json:"target_url"`
		Headers   map[string]string `json:"headers,omitempty"`
	}
	if err := json.NewDecoder(limitedBody(w, r)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"Invalid JSON request body",
			map[string]interface{}{"parse_error": err.Error()},
		))
		return
	}

	validatedURL, err := validateTargetURL(req.TargetURL)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			err.Error(),
			map[string]interface{}{"field": "target_url"},
		))
		return
	}

	config := &transport.TransportConfig{
		Endpoint:             validatedURL,
		Headers:              req.Headers,
		AllowPrivateNetworks: []string{"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "::1/128"},
		Timeouts: transport.TimeoutConfig{
			ConnectTimeout:     10 * time.Second,
			RequestTimeout:     30 * time.Second,
			StreamStallTimeout: 15 * time.Second,
		},
	}

	adapter := transport.NewStreamableHTTPAdapter()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	conn, err := adapter.Connect(ctx, config)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, &ErrorResponse{
			ErrorType:    "connection_error",
			ErrorCode:    "TARGET_UNREACHABLE",
			ErrorMessage: fmt.Sprintf("Failed to connect to target: %v", err),
			Details:      map[string]interface{}{"target_url": req.TargetURL},
		})
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("failed to close discovery connection: %v", err)
		}
	}()

	initParams := &transport.InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities:    make(map[string]interface{}),
		ClientInfo: transport.ClientInfo{
			Name:    "mcpdrill",
			Version: "1.0.0",
		},
	}

	initOutcome, err := conn.Initialize(ctx, initParams)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, &ErrorResponse{
			ErrorType:    "mcp_error",
			ErrorCode:    "INIT_FAILED",
			ErrorMessage: fmt.Sprintf("MCP initialize failed: %v", err),
			Details:      map[string]interface{}{"target_url": req.TargetURL},
		})
		return
	}

	if !initOutcome.OK {
		errMsg := "MCP initialize failed"
		if initOutcome.Error != nil {
			errMsg = fmt.Sprintf("MCP initialize failed: %v", initOutcome.Error.Message)
		}
		s.writeError(w, http.StatusBadGateway, &ErrorResponse{
			ErrorType:    "mcp_error",
			ErrorCode:    "INIT_FAILED",
			ErrorMessage: errMsg,
			Details:      map[string]interface{}{"target_url": req.TargetURL},
		})
		return
	}

	if _, err = conn.SendInitialized(ctx); err != nil {
		s.writeError(w, http.StatusBadGateway, &ErrorResponse{
			ErrorType:    "mcp_error",
			ErrorCode:    "INIT_FAILED",
			ErrorMessage: fmt.Sprintf("MCP initialized notification failed: %v", err),
			Details:      map[string]interface{}{"target_url": req.TargetURL},
		})
		return
	}

	outcome, err := conn.ToolsList(ctx, nil)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, &ErrorResponse{
			ErrorType:    "mcp_error",
			ErrorCode:    "TOOLS_LIST_FAILED",
			ErrorMessage: fmt.Sprintf("Failed to list tools: %v", err),
			Details:      map[string]interface{}{"target_url": req.TargetURL},
		})
		return
	}

	if !outcome.OK {
		s.writeError(w, http.StatusBadGateway, &ErrorResponse{
			ErrorType:    "mcp_error",
			ErrorCode:    "TOOLS_LIST_ERROR",
			ErrorMessage: fmt.Sprintf("Tools list returned error: %v", outcome.Error),
			Details:      map[string]interface{}{"target_url": req.TargetURL},
		})
		return
	}

	var result transport.ToolsListResult
	if err := json.Unmarshal(outcome.Result, &result); err != nil {
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(
			fmt.Sprintf("Failed to parse tools list result: %v", err),
		))
		return
	}

	s.writeJSON(w, http.StatusOK, &struct {
		Tools []transport.Tool `json:"tools"`
	}{
		Tools: result.Tools,
	})
}

func (s *Server) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w, r.Method, "POST")
		return
	}

	var req struct {
		TargetURL string            `json:"target_url"`
		Headers   map[string]string `json:"headers,omitempty"`
	}
	if err := json.NewDecoder(limitedBody(w, r)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"Invalid JSON request body",
			map[string]interface{}{"parse_error": err.Error()},
		))
		return
	}

	validatedURL, err := validateTargetURL(req.TargetURL)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			err.Error(),
			map[string]interface{}{"field": "target_url"},
		))
		return
	}

	config := &transport.TransportConfig{
		Endpoint:             validatedURL,
		Headers:              req.Headers,
		AllowPrivateNetworks: []string{"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "::1/128"},
		Timeouts: transport.TimeoutConfig{
			ConnectTimeout:     10 * time.Second,
			RequestTimeout:     30 * time.Second,
			StreamStallTimeout: 15 * time.Second,
		},
	}

	adapter := transport.NewStreamableHTTPAdapter()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	startTime := time.Now()

	conn, err := adapter.Connect(ctx, config)
	connectLatency := time.Since(startTime)
	if err != nil {
		s.writeJSON(w, http.StatusOK, &struct {
			Success        bool   `json:"success"`
			Error          string `json:"error"`
			ErrorCode      string `json:"error_code"`
			ConnectLatency int64  `json:"connect_latency_ms"`
		}{
			Success:        false,
			Error:          fmt.Sprintf("Failed to connect: %v", err),
			ErrorCode:      "CONNECTION_FAILED",
			ConnectLatency: connectLatency.Milliseconds(),
		})
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("failed to close discovery connection: %v", err)
		}
	}()

	// MCP protocol requires initialize handshake before any other operations
	initParams := &transport.InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities:    make(map[string]interface{}),
		ClientInfo: transport.ClientInfo{
			Name:    "mcpdrill",
			Version: "1.0.0",
		},
	}

	initOutcome, err := conn.Initialize(ctx, initParams)
	if err != nil {
		s.writeJSON(w, http.StatusOK, &struct {
			Success        bool   `json:"success"`
			Error          string `json:"error"`
			ErrorCode      string `json:"error_code"`
			ConnectLatency int64  `json:"connect_latency_ms"`
			TotalLatency   int64  `json:"total_latency_ms"`
		}{
			Success:        false,
			Error:          fmt.Sprintf("MCP initialize failed: %v", err),
			ErrorCode:      "INIT_FAILED",
			ConnectLatency: connectLatency.Milliseconds(),
			TotalLatency:   time.Since(startTime).Milliseconds(),
		})
		return
	}

	if !initOutcome.OK {
		errMsg := "MCP initialize failed"
		if initOutcome.Error != nil {
			errMsg = fmt.Sprintf("MCP initialize failed: %v", initOutcome.Error.Message)
		}
		s.writeJSON(w, http.StatusOK, &struct {
			Success        bool   `json:"success"`
			Error          string `json:"error"`
			ErrorCode      string `json:"error_code"`
			ConnectLatency int64  `json:"connect_latency_ms"`
			TotalLatency   int64  `json:"total_latency_ms"`
		}{
			Success:        false,
			Error:          errMsg,
			ErrorCode:      "INIT_FAILED",
			ConnectLatency: connectLatency.Milliseconds(),
			TotalLatency:   time.Since(startTime).Milliseconds(),
		})
		return
	}

	// Send initialized notification to complete handshake
	_, err = conn.SendInitialized(ctx)
	if err != nil {
		s.writeJSON(w, http.StatusOK, &struct {
			Success        bool   `json:"success"`
			Error          string `json:"error"`
			ErrorCode      string `json:"error_code"`
			ConnectLatency int64  `json:"connect_latency_ms"`
			TotalLatency   int64  `json:"total_latency_ms"`
		}{
			Success:        false,
			Error:          fmt.Sprintf("MCP initialized notification failed: %v", err),
			ErrorCode:      "INIT_FAILED",
			ConnectLatency: connectLatency.Milliseconds(),
			TotalLatency:   time.Since(startTime).Milliseconds(),
		})
		return
	}

	toolsStartTime := time.Now()
	outcome, err := conn.ToolsList(ctx, nil)
	toolsLatency := time.Since(toolsStartTime)
	totalLatency := time.Since(startTime)

	if err != nil {
		s.writeJSON(w, http.StatusOK, &struct {
			Success        bool   `json:"success"`
			Error          string `json:"error"`
			ErrorCode      string `json:"error_code"`
			ConnectLatency int64  `json:"connect_latency_ms"`
			TotalLatency   int64  `json:"total_latency_ms"`
		}{
			Success:        false,
			Error:          fmt.Sprintf("MCP request failed: %v", err),
			ErrorCode:      "MCP_ERROR",
			ConnectLatency: connectLatency.Milliseconds(),
			TotalLatency:   totalLatency.Milliseconds(),
		})
		return
	}

	if !outcome.OK {
		s.writeJSON(w, http.StatusOK, &struct {
			Success        bool   `json:"success"`
			Error          string `json:"error"`
			ErrorCode      string `json:"error_code"`
			ConnectLatency int64  `json:"connect_latency_ms"`
			ToolsLatency   int64  `json:"tools_latency_ms"`
			TotalLatency   int64  `json:"total_latency_ms"`
		}{
			Success:        false,
			Error:          fmt.Sprintf("Server returned error: %v", outcome.Error),
			ErrorCode:      "SERVER_ERROR",
			ConnectLatency: connectLatency.Milliseconds(),
			ToolsLatency:   toolsLatency.Milliseconds(),
			TotalLatency:   totalLatency.Milliseconds(),
		})
		return
	}

	var result transport.ToolsListResult
	if err := json.Unmarshal(outcome.Result, &result); err != nil {
		result.Tools = []transport.Tool{}
	}

	s.writeJSON(w, http.StatusOK, &struct {
		Success        bool             `json:"success"`
		Message        string           `json:"message"`
		ToolCount      int              `json:"tool_count"`
		Tools          []transport.Tool `json:"tools"`
		ConnectLatency int64            `json:"connect_latency_ms"`
		ToolsLatency   int64            `json:"tools_latency_ms"`
		TotalLatency   int64            `json:"total_latency_ms"`
	}{
		Success:        true,
		Message:        fmt.Sprintf("Connected successfully. Found %d tools.", len(result.Tools)),
		ToolCount:      len(result.Tools),
		Tools:          result.Tools,
		ConnectLatency: connectLatency.Milliseconds(),
		ToolsLatency:   toolsLatency.Milliseconds(),
		TotalLatency:   totalLatency.Milliseconds(),
	})
}

func (s *Server) handleTestTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w, r.Method, "POST")
		return
	}

	var req struct {
		TargetURL string                 `json:"target_url"`
		ToolName  string                 `json:"tool_name"`
		Arguments map[string]interface{} `json:"arguments,omitempty"`
		Headers   map[string]string      `json:"headers,omitempty"`
	}
	if err := json.NewDecoder(limitedBody(w, r)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"Invalid JSON request body",
			map[string]interface{}{"parse_error": err.Error()},
		))
		return
	}

	if req.ToolName == "" {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"tool_name is required",
			map[string]interface{}{"field": "tool_name"},
		))
		return
	}

	validatedURL, err := validateTargetURL(req.TargetURL)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			err.Error(),
			map[string]interface{}{"field": "target_url"},
		))
		return
	}

	config := &transport.TransportConfig{
		Endpoint:             validatedURL,
		Headers:              req.Headers,
		AllowPrivateNetworks: []string{"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "::1/128"},
		Timeouts: transport.TimeoutConfig{
			ConnectTimeout:     10 * time.Second,
			RequestTimeout:     60 * time.Second,
			StreamStallTimeout: 15 * time.Second,
		},
	}

	adapter := transport.NewStreamableHTTPAdapter()
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	startTime := time.Now()

	conn, err := adapter.Connect(ctx, config)
	if err != nil {
		s.writeJSON(w, http.StatusOK, &struct {
			Success   bool   `json:"success"`
			Error     string `json:"error"`
			LatencyMs int64  `json:"latency_ms"`
		}{
			Success:   false,
			Error:     fmt.Sprintf("Failed to connect: %v", err),
			LatencyMs: time.Since(startTime).Milliseconds(),
		})
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("failed to close test-tool connection: %v", err)
		}
	}()

	outcome, err := conn.ToolsCall(ctx, &transport.ToolsCallParams{
		Name:      req.ToolName,
		Arguments: req.Arguments,
	})
	latencyMs := time.Since(startTime).Milliseconds()

	if err != nil {
		s.writeJSON(w, http.StatusOK, &struct {
			Success   bool   `json:"success"`
			Error     string `json:"error"`
			LatencyMs int64  `json:"latency_ms"`
		}{
			Success:   false,
			Error:     fmt.Sprintf("Tool call failed: %v", err),
			LatencyMs: latencyMs,
		})
		return
	}

	if !outcome.OK {
		s.writeJSON(w, http.StatusOK, &struct {
			Success   bool   `json:"success"`
			Error     string `json:"error"`
			LatencyMs int64  `json:"latency_ms"`
		}{
			Success:   false,
			Error:     fmt.Sprintf("Tool returned error: %v", outcome.Error),
			LatencyMs: latencyMs,
		})
		return
	}

	var toolResult transport.ToolsCallResult
	if err := json.Unmarshal(outcome.Result, &toolResult); err != nil {
		s.writeJSON(w, http.StatusOK, &struct {
			Success   bool            `json:"success"`
			Result    json.RawMessage `json:"result"`
			LatencyMs int64           `json:"latency_ms"`
		}{
			Success:   true,
			Result:    outcome.Result,
			LatencyMs: latencyMs,
		})
		return
	}

	if toolResult.IsError {
		s.writeJSON(w, http.StatusOK, &struct {
			Success   bool        `json:"success"`
			Error     string      `json:"error"`
			Result    interface{} `json:"result,omitempty"`
			LatencyMs int64       `json:"latency_ms"`
		}{
			Success:   false,
			Error:     "Tool execution returned an error",
			Result:    toolResult.Content,
			LatencyMs: latencyMs,
		})
		return
	}

	s.writeJSON(w, http.StatusOK, &struct {
		Success   bool        `json:"success"`
		Result    interface{} `json:"result"`
		LatencyMs int64       `json:"latency_ms"`
	}{
		Success:   true,
		Result:    toolResult.Content,
		LatencyMs: latencyMs,
	})
}
