package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/auth"
	"github.com/bc-dunia/mcpdrill/internal/mcp"
	"github.com/bc-dunia/mcpdrill/internal/transport"
)

var privateNetworkCIDRs = []string{"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "::1/128"}

type discoveryRequest struct {
	TargetURL             string            `json:"target_url"`
	Headers               map[string]string `json:"headers,omitempty"`
	ProtocolVersion       string            `json:"protocol_version,omitempty"`
	ProtocolVersionPolicy string            `json:"protocol_version_policy,omitempty"`
}

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

	if !s.requireAdminDiscoveryAccess(w, r) {
		return
	}

	var req discoveryRequest
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
		AllowPrivateNetworks: s.discoveryPrivateNetworks(),
		Timeouts: transport.TimeoutConfig{
			ConnectTimeout:     10 * time.Second,
			RequestTimeout:     30 * time.Second,
			StreamStallTimeout: 15 * time.Second,
		},
	}

	adapter := transport.NewStreamableHTTPAdapter()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	conn, err := connectDiscoverySession(ctx, adapter, config, req.ProtocolVersion, req.ProtocolVersionPolicy)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, &ErrorResponse{
			ErrorType:    "connection_error",
			ErrorCode:    "TARGET_UNREACHABLE",
			ErrorMessage: fmt.Sprintf("Failed to connect to target MCP server: %v", err),
			Details:      map[string]interface{}{"target_url": req.TargetURL},
		})
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("failed to close discovery connection: %v", err)
		}
	}()

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

	if !s.requireAdminDiscoveryAccess(w, r) {
		return
	}

	var req discoveryRequest
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
		AllowPrivateNetworks: s.discoveryPrivateNetworks(),
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

	conn, err := connectDiscoverySession(ctx, adapter, config, req.ProtocolVersion, req.ProtocolVersionPolicy)
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

	if !s.requireAdminDiscoveryAccess(w, r) {
		return
	}

	var req struct {
		TargetURL             string                 `json:"target_url"`
		ToolName              string                 `json:"tool_name"`
		Arguments             map[string]interface{} `json:"arguments,omitempty"`
		Headers               map[string]string      `json:"headers,omitempty"`
		ProtocolVersion       string                 `json:"protocol_version,omitempty"`
		ProtocolVersionPolicy string                 `json:"protocol_version_policy,omitempty"`
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
		AllowPrivateNetworks: s.discoveryPrivateNetworks(),
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

	conn, err := connectDiscoverySession(ctx, adapter, config, req.ProtocolVersion, req.ProtocolVersionPolicy)
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

type discoveryProtocolConfigurer interface {
	ConfigureProtocol(version string)
}

type discoveryModernProtocolConfigurer interface {
	ConfigureModernProtocol(version string)
}

type discoveryLegacyProtocolConfigurer interface {
	ConfigureLegacyProtocol(version string)
}

type discoveryModernDiscoverer interface {
	Discover(ctx context.Context) (*transport.OperationOutcome, error)
}

func connectDiscoverySession(ctx context.Context, adapter transport.Adapter, config *transport.TransportConfig, version string, policy string) (transport.Connection, error) {
	if version == "" {
		version = mcp.ProtocolVersionAuto
	}
	if policy == "" {
		policy = string(mcp.VersionPolicySupported)
	}
	versionPolicy := mcp.ParseVersionPolicy(policy)

	switch mcp.EraForVersion(version) {
	case mcp.ProtocolEraModern:
		return connectModernDiscoverySession(ctx, adapter, config, version, versionPolicy)
	case mcp.ProtocolEraLegacy:
		return connectLegacyDiscoverySession(ctx, adapter, config, version, versionPolicy)
	}

	modernConn, err := adapter.Connect(ctx, config)
	if err == nil {
		if err := prepareModernDiscoverySession(ctx, modernConn, mcp.ModernProtocolVersion, versionPolicy); err == nil {
			return modernConn, nil
		}
		if closeErr := modernConn.Close(); closeErr != nil {
			log.Printf("failed to close failed modern discovery connection: %v", closeErr)
		}
	}

	return connectLegacyDiscoverySession(ctx, adapter, config, mcp.DefaultProtocolVersion, versionPolicy)
}

func connectModernDiscoverySession(ctx context.Context, adapter transport.Adapter, config *transport.TransportConfig, version string, policy mcp.VersionPolicy) (transport.Connection, error) {
	conn, err := adapter.Connect(ctx, config)
	if err != nil {
		return nil, err
	}
	if err := prepareModernDiscoverySession(ctx, conn, version, policy); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("failed to close failed modern discovery connection: %v", closeErr)
		}
		return nil, err
	}
	return conn, nil
}

func connectLegacyDiscoverySession(ctx context.Context, adapter transport.Adapter, config *transport.TransportConfig, version string, policy mcp.VersionPolicy) (transport.Connection, error) {
	legacyConn, err := adapter.Connect(ctx, config)
	if err != nil {
		return nil, err
	}
	if err := prepareLegacyDiscoverySession(ctx, legacyConn, version, policy); err != nil {
		if closeErr := legacyConn.Close(); closeErr != nil {
			log.Printf("failed to close failed legacy discovery connection: %v", closeErr)
		}
		return nil, err
	}
	return legacyConn, nil
}

func prepareModernDiscoverySession(ctx context.Context, conn transport.Connection, version string, policy mcp.VersionPolicy) error {
	if version == "" || version == mcp.ProtocolVersionAuto {
		version = mcp.ModernProtocolVersion
	}
	if c, ok := conn.(discoveryModernProtocolConfigurer); ok {
		c.ConfigureModernProtocol(version)
	} else if c, ok := conn.(discoveryProtocolConfigurer); ok {
		c.ConfigureProtocol(version)
	}

	discoverer, ok := conn.(discoveryModernDiscoverer)
	if !ok {
		return fmt.Errorf("transport does not support server/discover")
	}
	outcome, err := discoverer.Discover(ctx)
	if err != nil {
		return err
	}
	if !outcome.OK {
		if outcome.Error != nil {
			return fmt.Errorf("server/discover failed: %s", outcome.Error.Message)
		}
		return fmt.Errorf("server/discover failed")
	}
	discoverResult, err := transport.ParseDiscoverResult(outcome.Result)
	if err != nil {
		return err
	}
	selectedVersion, err := selectModernDiscoveryVersion(version, discoverResult.SupportedVersions, policy)
	if err != nil {
		return err
	}
	if selectedVersion != version {
		if c, ok := conn.(discoveryModernProtocolConfigurer); ok {
			c.ConfigureModernProtocol(selectedVersion)
		} else if c, ok := conn.(discoveryProtocolConfigurer); ok {
			c.ConfigureProtocol(selectedVersion)
		}
	}
	return nil
}

func selectModernDiscoveryVersion(requested string, supported []string, policy mcp.VersionPolicy) (string, error) {
	if requested == "" || requested == mcp.ProtocolVersionAuto {
		requested = mcp.ModernProtocolVersion
	}
	if policy == "" {
		policy = mcp.VersionPolicyStrict
	}
	if policy == mcp.VersionPolicyNone {
		return requested, nil
	}
	for _, supportedVersion := range supported {
		if supportedVersion == requested {
			return requested, nil
		}
	}
	if policy == mcp.VersionPolicySupported {
		for _, supportedVersion := range supported {
			if mcp.IsSupported(supportedVersion) && mcp.EraForVersion(supportedVersion) == mcp.ProtocolEraModern {
				return supportedVersion, nil
			}
		}
	}
	return "", fmt.Errorf("server/discover did not advertise an acceptable modern protocol version for %s", requested)
}

func prepareLegacyDiscoverySession(ctx context.Context, conn transport.Connection, version string, policy mcp.VersionPolicy) error {
	if version == "" || version == mcp.ProtocolVersionAuto {
		version = mcp.DefaultProtocolVersion
	}
	if c, ok := conn.(discoveryLegacyProtocolConfigurer); ok {
		c.ConfigureLegacyProtocol(version)
	} else if c, ok := conn.(discoveryProtocolConfigurer); ok {
		c.ConfigureProtocol(version)
	}

	initParams := &transport.InitializeParams{
		ProtocolVersion: version,
		Capabilities:    make(map[string]interface{}),
		ClientInfo: transport.ClientInfo{
			Name:    mcp.ClientName,
			Version: mcp.ClientVersion,
		},
	}

	initOutcome, err := conn.Initialize(ctx, initParams)
	if err != nil {
		return fmt.Errorf("MCP initialize failed: %w", err)
	}
	if !initOutcome.OK {
		if initOutcome.Error != nil {
			return fmt.Errorf("MCP initialize failed: %s", initOutcome.Error.Message)
		}
		return fmt.Errorf("MCP initialize failed")
	}
	initResult, err := transport.ParseInitializeResult(initOutcome.Result)
	if err != nil {
		return err
	}
	if err := mcp.ValidateNegotiation(version, initResult.ProtocolVersion, policy); err != nil {
		return err
	}
	if _, err = conn.SendInitialized(ctx); err != nil {
		return fmt.Errorf("MCP initialized notification failed: %w", err)
	}
	return nil
}

func (s *Server) discoveryPrivateNetworks() []string {
	if !s.allowPrivateDiscoveryNetworks() {
		return nil
	}
	return append([]string(nil), privateNetworkCIDRs...)
}

func (s *Server) requireAdminDiscoveryAccess(w http.ResponseWriter, r *http.Request) bool {
	if s.authConfig == nil || s.authConfig.Mode == auth.AuthModeNone {
		// Security: discovery endpoints perform outbound requests, so require auth unless
		// explicitly running insecure for local testing.
		//
		// Note: when running inside Docker, requests from the host often appear as a private
		// gateway IP (not 127.0.0.1) from the container's perspective.
		if s.authConfig != nil && s.authConfig.InsecureMode && isLocalOrPrivateRequest(r) {
			return true
		}
		s.writeError(w, http.StatusForbidden, &ErrorResponse{
			ErrorType:    ErrorTypeForbidden,
			ErrorCode:    "INSUFFICIENT_PERMISSIONS",
			ErrorMessage: "Admin role required for discovery endpoints",
			Retryable:    false,
		})
		return false
	}

	if !auth.HasRole(r.Context(), auth.RoleAdmin) {
		s.writeError(w, http.StatusForbidden, &ErrorResponse{
			ErrorType:    ErrorTypeForbidden,
			ErrorCode:    "INSUFFICIENT_PERMISSIONS",
			ErrorMessage: "Admin role required for discovery endpoints",
			Retryable:    false,
		})
		return false
	}

	return true
}

func isLocalOrPrivateRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}
