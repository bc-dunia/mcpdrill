package transport

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/mcp"
)

const (
	TransportIDStreamableHTTP = "streamable_http"

	HeaderContentType        = "Content-Type"
	HeaderAccept             = "Accept"
	HeaderMCPProtocolVersion = "MCP-Protocol-Version"
	HeaderMCPMethod          = "Mcp-Method"
	HeaderMCPName            = "Mcp-Name"
	HeaderMCPSessionID       = "Mcp-Session-Id"
	HeaderLastEventID        = "Last-Event-ID"
	HeaderAuthorization      = "Authorization"

	ContentTypeJSON = "application/json"
	ContentTypeSSE  = "text/event-stream"
	AcceptBoth      = "application/json, text/event-stream"
)

type StreamableHTTPAdapter struct{}

func NewStreamableHTTPAdapter() *StreamableHTTPAdapter {
	return &StreamableHTTPAdapter{}
}

func (a *StreamableHTTPAdapter) ID() string {
	return TransportIDStreamableHTTP
}

func (a *StreamableHTTPAdapter) Connect(ctx context.Context, config *TransportConfig) (Connection, error) {
	safeDialer := newSafeDialer(config.Timeouts.ConnectTimeout, config.AllowPrivateNetworks)
	transport := &http.Transport{
		DialContext:           safeDialer.DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   config.Timeouts.ConnectTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}

	if config.TLSSkipVerify || len(config.CABundle) > 0 {
		if config.TLSSkipVerify {
			slog.Warn("tls_verification_disabled",
				"warning", "TLS certificate verification is DISABLED - connections are vulnerable to MITM attacks",
				"endpoint", config.Endpoint)
		}
		tlsConfig := &tls.Config{
			InsecureSkipVerify: config.TLSSkipVerify,
		}
		if len(config.CABundle) > 0 {
			certPool := x509.NewCertPool()
			if certPool.AppendCertsFromPEM(config.CABundle) {
				tlsConfig.RootCAs = certPool
			}
		}
		transport.TLSClientConfig = tlsConfig
	}
	// Build CheckRedirect function based on redirect policy
	checkRedirect := buildCheckRedirect(config)

	client := &http.Client{
		Transport:     transport,
		Timeout:       0,
		CheckRedirect: checkRedirect,
	}

	conn := &StreamableHTTPConnection{
		client:          client,
		transport:       transport,
		config:          config,
		sseHandler:      NewSSEResponseHandler(config.Timeouts.StreamStallTimeout),
		sessionID:       config.SessionID,
		lastEventID:     config.LastEventID,
		protocolEra:     mcp.ProtocolEraLegacy,
		protocolVersion: mcp.DefaultProtocolVersion,
		toolHeaders:     make(map[string][]toolHeaderBinding),
		requestCount:    0,
	}

	return conn, nil
}

// buildCheckRedirect creates a CheckRedirect function based on the redirect policy configuration.
func buildCheckRedirect(config *TransportConfig) func(req *http.Request, via []*http.Request) error {
	// Default to deny if no policy configured
	if config.RedirectPolicy == nil || config.RedirectPolicy.Mode == "" || config.RedirectPolicy.Mode == "deny" {
		return func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	maxRedirects := config.RedirectPolicy.MaxRedirects
	if maxRedirects <= 0 {
		maxRedirects = 0
	}
	if maxRedirects > 3 {
		maxRedirects = 3
	}

	// Parse original endpoint for same_origin comparison
	originalURL, _ := url.Parse(config.Endpoint)
	originalHostname := ""
	originalScheme := ""
	if originalURL != nil {
		originalHostname = strings.ToLower(originalURL.Hostname())
		originalScheme = strings.ToLower(originalURL.Scheme)
	}

	return func(req *http.Request, via []*http.Request) error {
		// Check max redirects - use > to allow exactly maxRedirects redirects
		if len(via) > maxRedirects {
			return http.ErrUseLastResponse
		}

		// Prevent HTTPS to HTTP downgrade
		if originalScheme == "https" && strings.ToLower(req.URL.Scheme) == "http" {
			return http.ErrUseLastResponse
		}

		redirectHostname := strings.ToLower(req.URL.Hostname())

		switch config.RedirectPolicy.Mode {
		case "same_origin":
			// Only allow redirects to the same host (without port)
			if redirectHostname != originalHostname {
				return http.ErrUseLastResponse
			}
			return nil

		case "allowlist_only":
			// Only allow redirects to hosts in the allowlist (without port)
			// Normalize allowlist entries: parse as URL and extract hostname, fallback to raw string
			for _, allowed := range config.RedirectPolicy.Allowlist {
				allowedHostname := strings.ToLower(allowed)
				// Try to parse as URL to extract hostname
				if parsedURL, err := url.Parse(allowed); err == nil && parsedURL.Host != "" {
					allowedHostname = strings.ToLower(parsedURL.Hostname())
				}
				if redirectHostname == allowedHostname || strings.HasSuffix(redirectHostname, "."+allowedHostname) {
					return nil
				}
			}
			return http.ErrUseLastResponse

		default:
			// Unknown mode, deny
			return http.ErrUseLastResponse
		}
	}
}

type StreamableHTTPConnection struct {
	client            *http.Client
	transport         *http.Transport
	config            *TransportConfig
	sseHandler        *SSEResponseHandler
	sessionID         string
	lastEventID       string
	protocolEra       mcp.ProtocolEra
	protocolVersion   string
	toolHeaders       map[string][]toolHeaderBinding
	toolHeadersReady  bool
	toolHeadersExpiry time.Time
	requestCount      int64
	mu                sync.RWMutex
	closed            int32
}

type toolHeaderBinding struct {
	Path       []string
	HeaderName string
	ValueType  string
}

func (c *StreamableHTTPConnection) SessionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionID
}

func (c *StreamableHTTPConnection) SetSessionID(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionID = sessionID
}

func (c *StreamableHTTPConnection) SetLastEventID(eventID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastEventID = eventID
}

func (c *StreamableHTTPConnection) ConfigureProtocol(version string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if version == "" || version == mcp.ProtocolVersionAuto {
		version = mcp.DefaultProtocolVersion
	}
	c.protocolVersion = version
	c.protocolEra = mcp.EraForVersion(version)
}

func (c *StreamableHTTPConnection) ConfigureModernProtocol(version string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if version == "" || version == mcp.ProtocolVersionAuto {
		version = mcp.ModernProtocolVersion
	}
	c.protocolVersion = version
	c.protocolEra = mcp.ProtocolEraModern
}

func (c *StreamableHTTPConnection) ConfigureLegacyProtocol(version string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if version == "" || version == mcp.ProtocolVersionAuto {
		version = mcp.DefaultProtocolVersion
	}
	c.protocolVersion = version
	c.protocolEra = mcp.ProtocolEraLegacy
}

func (c *StreamableHTTPConnection) ProtocolEra() mcp.ProtocolEra {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.protocolEra
}

func (c *StreamableHTTPConnection) ProtocolVersion() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.protocolVersion
}

func (c *StreamableHTTPConnection) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return nil
	}
	c.transport.CloseIdleConnections()
	return nil
}

func (c *StreamableHTTPConnection) Initialize(ctx context.Context, params *InitializeParams) (*OperationOutcome, error) {
	requestID := c.nextRequestID()
	req := NewInitializeRequest(requestID, params)

	outcome := c.doRequest(ctx, req, OpInitialize, requestID)
	return outcome, nil
}

func (c *StreamableHTTPConnection) SendInitialized(ctx context.Context) (*OperationOutcome, error) {
	req := NewInitializedNotification()

	outcome := c.doNotification(ctx, req, OpInitialized)
	return outcome, nil
}

func (c *StreamableHTTPConnection) Discover(ctx context.Context) (*OperationOutcome, error) {
	requestID := c.nextRequestID()
	req := NewServerDiscoverRequest(requestID)

	outcome := c.doRequest(ctx, req, OpServerDiscover, requestID)
	return outcome, nil
}

func (c *StreamableHTTPConnection) ToolsList(ctx context.Context, cursor *string) (*OperationOutcome, error) {
	requestID := c.nextRequestID()
	req := NewToolsListRequest(requestID, cursor)

	outcome := c.doRequest(ctx, req, OpToolsList, requestID)
	return outcome, nil
}

func (c *StreamableHTTPConnection) ToolsCall(ctx context.Context, params *ToolsCallParams) (*OperationOutcome, error) {
	requestID := c.nextRequestID()
	c.ensureToolHeaderBindings(ctx, params.Name)

	if c.config.ValidationConfig != nil && c.config.ValidationConfig.MaxArgumentSizeBytes > 0 {
		if err := ValidateArgumentSize(params.Arguments, c.config.ValidationConfig.MaxArgumentSizeBytes); err != nil {
			slog.Warn("argument validation failed",
				"request_id", requestID,
				"tool_name", params.Name,
				"error", err.Error())
		}
	}

	req := NewToolsCallRequestWithParams(requestID, *params)

	outcome := c.doRequest(ctx, req, OpToolsCall, requestID, params.Name)
	return outcome, nil
}

func (c *StreamableHTTPConnection) ensureToolHeaderBindings(ctx context.Context, toolName string) {
	if toolName == "" || c.ProtocolEra() != mcp.ProtocolEraModern {
		return
	}
	c.mu.RLock()
	_, hasBindings := c.toolHeaders[toolName]
	ready := c.toolHeadersReady
	expiry := c.toolHeadersExpiry
	c.mu.RUnlock()
	if !expiry.IsZero() && !time.Now().Before(expiry) {
		hasBindings = false
		ready = false
	}
	if hasBindings || ready {
		return
	}
	c.mu.Lock()
	if c.toolHeadersReady && (c.toolHeadersExpiry.IsZero() || time.Now().Before(c.toolHeadersExpiry)) {
		c.mu.Unlock()
		return
	}
	c.toolHeadersReady = false
	c.mu.Unlock()
	outcome, err := c.ToolsList(ctx, nil)
	if err != nil || outcome == nil || !outcome.OK {
		c.mu.Lock()
		c.toolHeadersReady = false
		c.toolHeadersExpiry = time.Time{}
		c.mu.Unlock()
	}
}

func (c *StreamableHTTPConnection) Ping(ctx context.Context) (*OperationOutcome, error) {
	requestID := c.nextRequestID()
	if c.ProtocolEra() == mcp.ProtocolEraModern {
		return c.unsupportedModernOutcome(OpPing, requestID), nil
	}
	req := NewPingRequest(requestID)

	outcome := c.doRequest(ctx, req, OpPing, requestID)
	return outcome, nil
}

func (c *StreamableHTTPConnection) ResourcesList(ctx context.Context, cursor *string) (*OperationOutcome, error) {
	requestID := c.nextRequestID()
	req := NewResourcesListRequest(requestID, cursor)

	outcome := c.doRequest(ctx, req, OpResourcesList, requestID)
	return outcome, nil
}

func (c *StreamableHTTPConnection) ResourcesRead(ctx context.Context, params *ResourcesReadParams) (*OperationOutcome, error) {
	requestID := c.nextRequestID()
	req := NewResourcesReadRequestWithParams(requestID, *params)

	outcome := c.doRequest(ctx, req, OpResourcesRead, requestID)
	return outcome, nil
}

func (c *StreamableHTTPConnection) PromptsList(ctx context.Context, cursor *string) (*OperationOutcome, error) {
	requestID := c.nextRequestID()
	req := NewPromptsListRequest(requestID, cursor)

	outcome := c.doRequest(ctx, req, OpPromptsList, requestID)
	return outcome, nil
}

func (c *StreamableHTTPConnection) PromptsGet(ctx context.Context, params *PromptsGetParams) (*OperationOutcome, error) {
	requestID := c.nextRequestID()
	req := NewPromptsGetRequestWithParams(requestID, *params)

	outcome := c.doRequest(ctx, req, OpPromptsGet, requestID)
	return outcome, nil
}

func (c *StreamableHTTPConnection) SubscriptionsListen(ctx context.Context, params *SubscriptionsListenParams) (*OperationOutcome, error) {
	requestID := c.nextRequestID()
	req := NewSubscriptionsListenRequest(requestID, params)

	outcome := c.doRequest(ctx, req, OpSubscriptionsListen, requestID)
	return outcome, nil
}

func (c *StreamableHTTPConnection) TasksGet(ctx context.Context, taskID string) (*OperationOutcome, error) {
	requestID := c.nextRequestID()
	req := NewTasksGetRequest(requestID, taskID)

	outcome := c.doRequest(ctx, req, OpTasksGet, requestID)
	return outcome, nil
}

func (c *StreamableHTTPConnection) TasksUpdate(ctx context.Context, params *TasksUpdateParams) (*OperationOutcome, error) {
	requestID := c.nextRequestID()
	req := NewTasksUpdateRequest(requestID, params)

	outcome := c.doRequest(ctx, req, OpTasksUpdate, requestID)
	return outcome, nil
}

func (c *StreamableHTTPConnection) TasksCancel(ctx context.Context, taskID string) (*OperationOutcome, error) {
	requestID := c.nextRequestID()
	req := NewTasksCancelRequest(requestID, taskID)

	outcome := c.doRequest(ctx, req, OpTasksCancel, requestID)
	return outcome, nil
}

func (c *StreamableHTTPConnection) unsupportedModernOutcome(op OperationType, requestID string) *OperationOutcome {
	return &OperationOutcome{
		Operation: op,
		JSONRPCID: requestID,
		Transport: TransportIDStreamableHTTP,
		StartTime: time.Now(),
		OK:        false,
		Error: &OperationError{
			Type:    ErrorTypeProtocol,
			Code:    CodeJSONRPCMethodNotFound,
			Message: fmt.Sprintf("%s is not supported by modern MCP protocol %s", op, c.ProtocolVersion()),
		},
	}
}

func (c *StreamableHTTPConnection) nextRequestID() string {
	count := atomic.AddInt64(&c.requestCount, 1)
	return fmt.Sprintf("req_%d", count)
}

func (c *StreamableHTTPConnection) doRequest(
	ctx context.Context,
	jsonrpcReq *JSONRPCRequest,
	opType OperationType,
	requestID string,
	toolName ...string,
) *OperationOutcome {
	outcome := &OperationOutcome{
		Operation: opType,
		JSONRPCID: requestID,
		Transport: TransportIDStreamableHTTP,
		StartTime: time.Now(),
	}
	if len(toolName) > 0 {
		outcome.ToolName = toolName[0]
	}

	ctx, cancel := context.WithTimeout(ctx, c.config.Timeouts.RequestTimeout)
	defer cancel()

	tracedCtx, phaseTracker := createTracedContext(ctx)
	jsonrpcReq = c.prepareRequest(jsonrpcReq)

	body, err := json.Marshal(jsonrpcReq)
	if err != nil {
		outcome.OK = false
		outcome.Error = MapProtocolError(fmt.Sprintf("failed to marshal request: %v", err))
		outcome.LatencyMs = time.Since(outcome.StartTime).Milliseconds()
		return outcome
	}
	outcome.BytesOut = int64(len(body))

	httpReq, err := http.NewRequestWithContext(tracedCtx, http.MethodPost, c.config.Endpoint, bytes.NewReader(body))
	if err != nil {
		outcome.OK = false
		outcome.Error = MapError(err)
		outcome.LatencyMs = time.Since(outcome.StartTime).Milliseconds()
		return outcome
	}

	c.mu.RLock()
	hasLastEventID := c.lastEventID != ""
	c.mu.RUnlock()
	c.setHeaders(httpReq, jsonrpcReq, hasLastEventID)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		outcome.OK = false
		outcome.Error = MapError(err)
		outcome.LatencyMs = time.Since(outcome.StartTime).Milliseconds()
		outcome.PhaseTiming = phaseTracker.computePhaseTiming(time.Now())
		return outcome
	}
	defer resp.Body.Close()

	outcome.HTTPStatus = &resp.StatusCode
	outcome.ContentType = resp.Header.Get(HeaderContentType)

	if sessionID := resp.Header.Get(HeaderMCPSessionID); sessionID != "" {
		c.SetSessionID(sessionID)
		outcome.SessionID = sessionID
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		outcome.OK = false
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		outcome.Error = MapHTTPStatusWithBody(resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
		outcome.LatencyMs = time.Since(outcome.StartTime).Milliseconds()
		outcome.PhaseTiming = phaseTracker.computePhaseTiming(time.Now())
		return outcome
	}

	c.handleResponse(ctx, resp, outcome, requestID)
	endTime := time.Now()
	outcome.LatencyMs = endTime.Sub(outcome.StartTime).Milliseconds()
	outcome.PhaseTiming = phaseTracker.computePhaseTiming(endTime)

	return outcome
}

func (c *StreamableHTTPConnection) doNotification(
	ctx context.Context,
	jsonrpcReq *JSONRPCRequest,
	opType OperationType,
) *OperationOutcome {
	outcome := &OperationOutcome{
		Operation: opType,
		Transport: TransportIDStreamableHTTP,
		StartTime: time.Now(),
	}

	ctx, cancel := context.WithTimeout(ctx, c.config.Timeouts.RequestTimeout)
	defer cancel()

	tracedCtx, phaseTracker := createTracedContext(ctx)
	jsonrpcReq = c.prepareRequest(jsonrpcReq)

	body, err := json.Marshal(jsonrpcReq)
	if err != nil {
		outcome.OK = false
		outcome.Error = MapProtocolError(fmt.Sprintf("failed to marshal notification: %v", err))
		outcome.LatencyMs = time.Since(outcome.StartTime).Milliseconds()
		return outcome
	}
	outcome.BytesOut = int64(len(body))

	httpReq, err := http.NewRequestWithContext(tracedCtx, http.MethodPost, c.config.Endpoint, bytes.NewReader(body))
	if err != nil {
		outcome.OK = false
		outcome.Error = MapError(err)
		outcome.LatencyMs = time.Since(outcome.StartTime).Milliseconds()
		return outcome
	}

	c.setHeaders(httpReq, jsonrpcReq, false)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		outcome.OK = false
		outcome.Error = MapError(err)
		outcome.LatencyMs = time.Since(outcome.StartTime).Milliseconds()
		outcome.PhaseTiming = phaseTracker.computePhaseTiming(time.Now())
		return outcome
	}
	defer resp.Body.Close()

	outcome.HTTPStatus = &resp.StatusCode
	outcome.ContentType = resp.Header.Get(HeaderContentType)

	switch resp.StatusCode {
	case http.StatusNoContent:
		outcome.OK = true
	case http.StatusOK, http.StatusAccepted:
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		outcome.BytesIn = int64(len(bodyBytes))
		if len(strings.TrimSpace(string(bodyBytes))) == 0 {
			outcome.OK = true
			break
		}
		var jsonrpcResp JSONRPCResponse
		if err := json.Unmarshal(bodyBytes, &jsonrpcResp); err == nil && jsonrpcResp.Error != nil {
			outcome.OK = false
			outcome.Error = ExtractJSONRPCError(&jsonrpcResp)
			outcome.JSONRPCErrorCode = &jsonrpcResp.Error.Code
		} else {
			outcome.OK = true
		}
	default:
		outcome.OK = false
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		outcome.Error = MapHTTPStatusWithBody(resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	endTime := time.Now()
	outcome.LatencyMs = endTime.Sub(outcome.StartTime).Milliseconds()
	outcome.PhaseTiming = phaseTracker.computePhaseTiming(endTime)
	return outcome
}

func (c *StreamableHTTPConnection) prepareRequest(req *JSONRPCRequest) *JSONRPCRequest {
	if req == nil {
		return nil
	}

	c.mu.RLock()
	era := c.protocolEra
	version := c.protocolVersion
	c.mu.RUnlock()

	if era != mcp.ProtocolEraModern {
		return req
	}
	if version == "" {
		version = mcp.ModernProtocolVersion
	}

	prepared := *req
	params := paramsToMap(prepared.Params)
	params["_meta"] = ModernRequestMeta{
		ProtocolVersion: version,
		ClientInfo: Implementation{
			Name:    mcp.ClientName,
			Version: mcp.ClientVersion,
		},
		ClientCapabilities: modernClientCapabilities(),
	}
	prepared.Params = params
	return &prepared
}

func modernClientCapabilities() map[string]interface{} {
	return map[string]interface{}{
		"extensions": map[string]interface{}{
			"io.modelcontextprotocol/tasks":         map[string]interface{}{},
			"io.modelcontextprotocol/inputRequests": map[string]interface{}{},
		},
	}
}

func paramsToMap(params interface{}) map[string]interface{} {
	if params == nil {
		return map[string]interface{}{}
	}
	if m, ok := params.(map[string]interface{}); ok {
		copyMap := make(map[string]interface{}, len(m)+1)
		for k, v := range m {
			copyMap[k] = v
		}
		return copyMap
	}

	data, err := json.Marshal(params)
	if err != nil {
		return map[string]interface{}{}
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]interface{}{}
	}
	return m
}

func (c *StreamableHTTPConnection) setHeaders(req *http.Request, jsonrpcReq *JSONRPCRequest, includeLastEventID bool) {
	req.Header.Set(HeaderContentType, ContentTypeJSON)
	req.Header.Set(HeaderAccept, AcceptBoth)

	c.mu.RLock()
	sessionID := c.sessionID
	lastEventID := c.lastEventID
	era := c.protocolEra
	version := c.protocolVersion
	c.mu.RUnlock()

	if era == mcp.ProtocolEraModern {
		if version == "" {
			version = mcp.ModernProtocolVersion
		}
		req.Header.Set(HeaderMCPProtocolVersion, version)
		if jsonrpcReq != nil {
			req.Header.Set(HeaderMCPMethod, jsonrpcReq.Method)
			if name := modernRouteName(jsonrpcReq); name != "" {
				encoded, ok := encodeMCPParamHeaderValue(name, "string")
				if ok {
					req.Header.Set(HeaderMCPName, encoded)
				}
			}
			c.setMCPParamHeaders(req, jsonrpcReq)
		}
		for key, value := range c.config.Headers {
			if isModernReservedHeader(key) {
				continue
			}
			req.Header.Set(key, value)
		}
		return
	}

	if sessionID != "" {
		req.Header.Set(HeaderMCPSessionID, sessionID)
	}

	if includeLastEventID && lastEventID != "" {
		req.Header.Set(HeaderLastEventID, lastEventID)
	}

	for key, value := range c.config.Headers {
		req.Header.Set(key, value)
	}
}

func isModernReservedHeader(key string) bool {
	if strings.HasPrefix(strings.ToLower(key), "mcp-param-") {
		return true
	}
	for _, reserved := range []string{HeaderMCPProtocolVersion, HeaderMCPMethod, HeaderMCPName, HeaderMCPSessionID, HeaderLastEventID} {
		if strings.EqualFold(key, reserved) {
			return true
		}
	}
	return false
}

func modernRouteName(req *JSONRPCRequest) string {
	if req == nil {
		return ""
	}
	params := paramsToMap(req.Params)
	switch OperationType(req.Method) {
	case OpToolsCall, OpPromptsGet:
		if name, ok := params["name"].(string); ok {
			return name
		}
	case OpResourcesRead:
		if uri, ok := params["uri"].(string); ok {
			return uri
		}
	case OpTasksGet, OpTasksUpdate, OpTasksCancel:
		if taskID, ok := params["taskId"].(string); ok {
			return taskID
		}
	}
	return ""
}

func (c *StreamableHTTPConnection) cacheToolHeaderBindings(data json.RawMessage) json.RawMessage {
	var result ToolsListResult
	if err := json.Unmarshal(data, &result); err != nil {
		return data
	}

	bindings := make(map[string][]toolHeaderBinding, len(result.Tools))
	validTools := make([]Tool, 0, len(result.Tools))
	for _, tool := range result.Tools {
		toolBindings, ok := extractToolHeaderBindings(tool.InputSchema)
		if !ok {
			continue
		}
		validTools = append(validTools, tool)
		if len(toolBindings) > 0 {
			bindings[tool.Name] = toolBindings
		}
	}
	if len(validTools) != len(result.Tools) {
		result.Tools = validTools
		if filtered, err := json.Marshal(result); err == nil {
			data = filtered
		}
	}

	c.mu.Lock()
	c.toolHeaders = bindings
	c.toolHeadersReady = true
	if result.TTLMs > 0 {
		c.toolHeadersExpiry = time.Now().Add(time.Duration(result.TTLMs) * time.Millisecond)
	} else {
		c.toolHeadersExpiry = time.Time{}
	}
	c.mu.Unlock()
	return data
}

func extractToolHeaderBindings(schema json.RawMessage) ([]toolHeaderBinding, bool) {
	if len(schema) == 0 {
		return nil, true
	}
	var root map[string]interface{}
	if err := json.Unmarshal(schema, &root); err != nil {
		return nil, false
	}

	seen := map[string]struct{}{}
	bindings := make([]toolHeaderBinding, 0)
	if !collectToolHeaderBindings(root, nil, seen, &bindings) {
		return nil, false
	}
	return bindings, true
}

func collectToolHeaderBindings(node map[string]interface{}, path []string, seen map[string]struct{}, bindings *[]toolHeaderBinding) bool {
	if rawHeader, hasHeader := node["x-mcp-header"]; hasHeader {
		header, ok := rawHeader.(string)
		if !ok || !isValidMCPHeaderName(header) {
			return false
		}
		lower := strings.ToLower(header)
		if _, exists := seen[lower]; exists {
			return false
		}
		valueType := primitiveSchemaType(node["type"])
		if valueType == "" {
			return false
		}
		seen[lower] = struct{}{}
		pathCopy := append([]string(nil), path...)
		*bindings = append(*bindings, toolHeaderBinding{Path: pathCopy, HeaderName: header, ValueType: valueType})
	}

	props, ok := node["properties"].(map[string]interface{})
	if !ok {
		return true
	}
	for name, child := range props {
		childMap, ok := child.(map[string]interface{})
		if !ok {
			continue
		}
		if !collectToolHeaderBindings(childMap, append(path, name), seen, bindings) {
			return false
		}
	}
	return true
}

func primitiveSchemaType(value interface{}) string {
	s, ok := value.(string)
	if !ok {
		return ""
	}
	switch s {
	case "string", "integer", "boolean":
		return s
	default:
		return ""
	}
}

func isValidMCPHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r > 127 || !(r == '!' || r == '#' || r == '$' || r == '%' || r == '&' || r == '\'' || r == '*' || r == '+' || r == '-' || r == '.' || r == '^' || r == '_' || r == '`' || r == '|' || r == '~' || (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
			return false
		}
	}
	return true
}

func (c *StreamableHTTPConnection) setMCPParamHeaders(req *http.Request, jsonrpcReq *JSONRPCRequest) {
	if OperationType(jsonrpcReq.Method) != OpToolsCall {
		return
	}
	params := paramsToMap(jsonrpcReq.Params)
	toolName, _ := params["name"].(string)
	arguments, _ := params["arguments"].(map[string]interface{})
	if toolName == "" || len(arguments) == 0 {
		return
	}

	c.mu.RLock()
	bindings := append([]toolHeaderBinding(nil), c.toolHeaders[toolName]...)
	c.mu.RUnlock()

	for _, binding := range bindings {
		value, ok := nestedArgument(arguments, binding.Path)
		if !ok || value == nil {
			continue
		}
		encoded, ok := encodeMCPParamHeaderValue(value, binding.ValueType)
		if !ok {
			continue
		}
		req.Header.Set("Mcp-Param-"+binding.HeaderName, encoded)
	}
}

func nestedArgument(arguments map[string]interface{}, path []string) (interface{}, bool) {
	if len(path) == 0 {
		return nil, false
	}
	var current interface{} = arguments
	for _, segment := range path {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		current, ok = m[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func encodeMCPParamHeaderValue(value interface{}, valueType string) (string, bool) {
	var s string
	switch valueType {
	case "string":
		v, ok := value.(string)
		if !ok {
			return "", false
		}
		s = v
	case "integer":
		i, ok := integerValue(value)
		if !ok {
			return "", false
		}
		s = fmt.Sprintf("%d", i)
	case "boolean":
		v, ok := value.(bool)
		if !ok {
			return "", false
		}
		if v {
			s = "true"
		} else {
			s = "false"
		}
	default:
		return "", false
	}

	if needsMCPHeaderBase64(s) {
		return "=?base64?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?=", true
	}
	return s, true
}

func integerValue(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		if v == float64(int64(v)) {
			return int64(v), true
		}
	}
	return 0, false
}

func needsMCPHeaderBase64(s string) bool {
	if strings.HasPrefix(s, "=?base64?") && strings.HasSuffix(s, "?=") {
		return true
	}
	if strings.TrimSpace(s) != s {
		return true
	}
	for _, r := range s {
		if r < 0x20 || r > 0x7e {
			return true
		}
	}
	return false
}

func (c *StreamableHTTPConnection) handleResponse(
	ctx context.Context,
	resp *http.Response,
	outcome *OperationOutcome,
	requestID string,
) {
	contentType := resp.Header.Get(HeaderContentType)

	if isSSEContentType(contentType) {
		c.handleSSEResponse(ctx, resp, outcome, requestID)
		return
	}

	c.handleJSONResponse(resp, outcome, requestID)
}

func (c *StreamableHTTPConnection) handleJSONResponse(
	resp *http.Response,
	outcome *OperationOutcome,
	requestID string,
) {
	const maxResponseSize = 100 * 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		outcome.OK = false
		outcome.Error = MapError(err)
		return
	}
	outcome.BytesIn = int64(len(body))

	if len(body) > maxResponseSize {
		outcome.OK = false
		outcome.Error = &OperationError{
			Type:    ErrorTypeProtocol,
			Code:    "RESPONSE_TOO_LARGE",
			Message: fmt.Sprintf("response exceeds maximum size of %d bytes", maxResponseSize),
		}
		return
	}

	var jsonrpcResp JSONRPCResponse
	if err := json.Unmarshal(body, &jsonrpcResp); err != nil {
		outcome.OK = false
		outcome.Error = MapProtocolError(fmt.Sprintf("failed to parse JSON-RPC response: %v", err))
		return
	}

	if validationErr := ValidateJSONRPCResponse(&jsonrpcResp, requestID); validationErr != nil {
		outcome.OK = false
		outcome.Error = validationErr
		return
	}

	if jsonrpcErr := ExtractJSONRPCError(&jsonrpcResp); jsonrpcErr != nil {
		outcome.OK = false
		outcome.Error = jsonrpcErr
		outcome.JSONRPCErrorCode = &jsonrpcResp.Error.Code
		return
	}

	outcome.OK = true
	outcome.Result = jsonrpcResp.Result
	if outcome.Operation == OpToolsList {
		outcome.Result = c.cacheToolHeaderBindings(jsonrpcResp.Result)
	}

	if outcome.Operation == OpToolsCall {
		var toolResult ToolsCallResult
		if err := json.Unmarshal(jsonrpcResp.Result, &toolResult); err != nil {
			outcome.OK = false
			outcome.Error = MapProtocolError(fmt.Sprintf("invalid tools/call result: %v", err))
			return
		}

		if c.config.ValidationConfig != nil && c.config.ValidationConfig.MaxResultSizeBytes > 0 {
			if valErr := ValidateResultWithMaxSize(&toolResult, c.config.ValidationConfig.MaxResultSizeBytes); valErr != nil && !valErr.Valid {
				slog.Warn("result validation warning",
					"request_id", requestID,
					"error", valErr.Error())
			}
		}

		if toolErr := CheckToolError(&toolResult, outcome.ToolName); toolErr != nil {
			outcome.OK = false
			outcome.Error = toolErr
		}
	}
}

func (c *StreamableHTTPConnection) handleSSEResponse(
	ctx context.Context,
	resp *http.Response,
	outcome *OperationOutcome,
	requestID string,
) {
	jsonrpcResp, signals, err := c.sseHandler.HandleSSEStream(ctx, resp.Body, requestID)

	outcome.Stream = signals

	if err != nil {
		outcome.OK = false
		if opErr, ok := err.(*OperationError); ok {
			outcome.Error = opErr
		} else {
			outcome.Error = MapError(err)
		}
		return
	}

	if jsonrpcResp == nil {
		outcome.OK = false
		outcome.Error = MapProtocolError("no response received from SSE stream")
		return
	}

	if validationErr := ValidateJSONRPCResponse(jsonrpcResp, requestID); validationErr != nil {
		outcome.OK = false
		outcome.Error = validationErr
		return
	}

	if jsonrpcErr := ExtractJSONRPCError(jsonrpcResp); jsonrpcErr != nil {
		outcome.OK = false
		outcome.Error = jsonrpcErr
		outcome.JSONRPCErrorCode = &jsonrpcResp.Error.Code
		return
	}

	outcome.OK = true
	outcome.Result = jsonrpcResp.Result
	if outcome.Operation == OpToolsList {
		outcome.Result = c.cacheToolHeaderBindings(jsonrpcResp.Result)
	}

	if outcome.Operation == OpToolsCall {
		var toolResult ToolsCallResult
		if err := json.Unmarshal(jsonrpcResp.Result, &toolResult); err != nil {
			outcome.OK = false
			outcome.Error = MapProtocolError(fmt.Sprintf("invalid tools/call result: %v", err))
			return
		}

		if c.config.ValidationConfig != nil && c.config.ValidationConfig.MaxResultSizeBytes > 0 {
			if valErr := ValidateResultWithMaxSize(&toolResult, c.config.ValidationConfig.MaxResultSizeBytes); valErr != nil && !valErr.Valid {
				slog.Warn("result validation warning",
					"request_id", requestID,
					"error", valErr.Error())
			}
		}

		if toolErr := CheckToolError(&toolResult, outcome.ToolName); toolErr != nil {
			outcome.OK = false
			outcome.Error = toolErr
		}
	}
}

func isSSEContentType(contentType string) bool {
	if contentType == ContentTypeSSE {
		return true
	}
	// Match "text/event-stream; charset=utf-8" etc., but not "text/event-stream-evil"
	if len(contentType) > len(ContentTypeSSE) {
		prefix := contentType[:len(ContentTypeSSE)]
		next := contentType[len(ContentTypeSSE)]
		return prefix == ContentTypeSSE && (next == ';' || next == ' ')
	}
	return false
}

type safeDialer struct {
	dialer               *net.Dialer
	allowedPrivateRanges []*net.IPNet
	blockedIPv4Ranges    []*net.IPNet
	blockedIPv6Ranges    []*net.IPNet
}

func newSafeDialer(timeout time.Duration, allowPrivateNetworks []string) *safeDialer {
	d := &safeDialer{
		dialer: &net.Dialer{
			Timeout: timeout,
		},
	}

	for _, cidrStr := range allowPrivateNetworks {
		_, ipnet, err := net.ParseCIDR(cidrStr)
		if err == nil {
			d.allowedPrivateRanges = append(d.allowedPrivateRanges, ipnet)
		}
	}

	ipv4Blocked := []string{
		"127.0.0.0/8",
		"169.254.0.0/16",
		"169.254.169.254/32",
		"100.100.100.200/32",
		"192.0.0.0/24",
		"0.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}
	for _, cidr := range ipv4Blocked {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err == nil {
			d.blockedIPv4Ranges = append(d.blockedIPv4Ranges, ipnet)
		}
	}

	ipv6Blocked := []string{
		"::1/128",
		"::/128",
		"fc00::/7",
		"fe80::/10",
		"ff00::/8",
		"::ffff:0:0/96",
		"64:ff9b::/96",
		"2001:db8::/32",
	}
	for _, cidr := range ipv6Blocked {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err == nil {
			d.blockedIPv6Ranges = append(d.blockedIPv6Ranges, ipnet)
		}
	}

	return d
}

func (d *safeDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("DNS lookup failed: %w", err)
	}

	for _, ip := range ips {
		if d.isIPBlocked(ip) {
			return nil, fmt.Errorf("connection to blocked IP address %s is not allowed", ip.String())
		}
	}

	return d.dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
}

func (d *safeDialer) isIPBlocked(ip net.IP) bool {
	if d.isPrivateNetworkAllowed(ip) {
		return false
	}

	if ip4 := ip.To4(); ip4 != nil {
		for _, blocked := range d.blockedIPv4Ranges {
			if blocked.Contains(ip4) {
				return true
			}
		}
	} else {
		for _, blocked := range d.blockedIPv6Ranges {
			if blocked.Contains(ip) {
				return true
			}
		}
	}

	return false
}

func (d *safeDialer) isPrivateNetworkAllowed(ip net.IP) bool {
	for _, allowed := range d.allowedPrivateRanges {
		if allowed.Contains(ip) {
			return true
		}
	}
	return false
}
