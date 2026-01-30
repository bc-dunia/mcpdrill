package api

import (
	"context"
	"encoding/json"
	"fmt"
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
		TargetURL string `json:"target_url"`
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
		AllowPrivateNetworks: []string{"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "::1/128"},
		Timeouts: transport.TimeoutConfig{
			ConnectTimeout: 10 * time.Second,
			RequestTimeout: 30 * time.Second,
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
	defer conn.Close()

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
			ErrorMessage: fmt.Sprintf("Tools list returned error: %s", outcome.Error),
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
			ConnectTimeout: 10 * time.Second,
			RequestTimeout: 30 * time.Second,
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
	defer conn.Close()

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
			Error:          fmt.Sprintf("Server returned error: %s", outcome.Error),
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
