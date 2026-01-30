package transport

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
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
)

const (
	TransportIDStreamableHTTP = "streamable_http"

	HeaderContentType   = "Content-Type"
	HeaderAccept        = "Accept"
	HeaderMCPSessionID  = "Mcp-Session-Id"
	HeaderLastEventID   = "Last-Event-ID"
	HeaderAuthorization = "Authorization"

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
		client:       client,
		transport:    transport,
		config:       config,
		sseHandler:   NewSSEResponseHandler(config.Timeouts.StreamStallTimeout),
		requestCount: 0,
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
	if originalURL != nil {
		originalHostname = strings.ToLower(originalURL.Hostname())
	}

	return func(req *http.Request, via []*http.Request) error {
		// Check max redirects - use > to allow exactly maxRedirects redirects
		if len(via) > maxRedirects {
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
	client       *http.Client
	transport    *http.Transport
	config       *TransportConfig
	sseHandler   *SSEResponseHandler
	sessionID    string
	lastEventID  string
	requestCount int64
	mu           sync.RWMutex
	closed       int32
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

func (c *StreamableHTTPConnection) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return nil
	}
	c.transport.CloseIdleConnections()
	return nil
}

func (c *StreamableHTTPConnection) Initialize(ctx context.Context, params *InitializeParams) (*OperationOutcome, error) {
	requestID := c.nextRequestID()
	req := NewInitializeRequest(requestID)

	outcome := c.doRequest(ctx, req, OpInitialize, requestID)
	return outcome, nil
}

func (c *StreamableHTTPConnection) SendInitialized(ctx context.Context) (*OperationOutcome, error) {
	req := NewInitializedNotification()

	outcome := c.doNotification(ctx, req, OpInitialized)
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
	req := NewToolsCallRequest(requestID, params.Name, params.Arguments)

	outcome := c.doRequest(ctx, req, OpToolsCall, requestID)
	outcome.ToolName = params.Name
	return outcome, nil
}

func (c *StreamableHTTPConnection) Ping(ctx context.Context) (*OperationOutcome, error) {
	requestID := c.nextRequestID()
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
	req := NewResourcesReadRequest(requestID, params.URI)

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
	req := NewPromptsGetRequest(requestID, params.Name, params.Arguments)

	outcome := c.doRequest(ctx, req, OpPromptsGet, requestID)
	return outcome, nil
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
) *OperationOutcome {
	outcome := &OperationOutcome{
		Operation: opType,
		JSONRPCID: requestID,
		Transport: TransportIDStreamableHTTP,
		StartTime: time.Now(),
	}

	ctx, cancel := context.WithTimeout(ctx, c.config.Timeouts.RequestTimeout)
	defer cancel()

	tracedCtx, phaseTracker := createTracedContext(ctx)

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

	c.setHeaders(httpReq, false)

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

	if httpErr := MapHTTPStatus(resp.StatusCode); httpErr != nil {
		outcome.OK = false
		outcome.Error = httpErr
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

	c.setHeaders(httpReq, false)

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
	case http.StatusOK, http.StatusAccepted, http.StatusNoContent:
		outcome.OK = true
	default:
		outcome.OK = false
		outcome.Error = MapHTTPStatus(resp.StatusCode)
	}

	endTime := time.Now()
	outcome.LatencyMs = endTime.Sub(outcome.StartTime).Milliseconds()
	outcome.PhaseTiming = phaseTracker.computePhaseTiming(endTime)
	return outcome
}

func (c *StreamableHTTPConnection) setHeaders(req *http.Request, includeLastEventID bool) {
	req.Header.Set(HeaderContentType, ContentTypeJSON)
	req.Header.Set(HeaderAccept, AcceptBoth)

	c.mu.RLock()
	sessionID := c.sessionID
	lastEventID := c.lastEventID
	c.mu.RUnlock()

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
	// Limit response body to prevent memory exhaustion (100MB max)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024*1024))
	if err != nil {
		outcome.OK = false
		outcome.Error = MapError(err)
		return
	}
	outcome.BytesIn = int64(len(body))

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

	if outcome.Operation == OpToolsCall {
		var toolResult ToolsCallResult
		if err := json.Unmarshal(jsonrpcResp.Result, &toolResult); err == nil {
			if toolErr := CheckToolError(&toolResult, outcome.ToolName); toolErr != nil {
				outcome.OK = false
				outcome.Error = toolErr
			}
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

	if outcome.Operation == OpToolsCall {
		var toolResult ToolsCallResult
		if err := json.Unmarshal(jsonrpcResp.Result, &toolResult); err == nil {
			if toolErr := CheckToolError(&toolResult, outcome.ToolName); toolErr != nil {
				outcome.OK = false
				outcome.Error = toolErr
			}
		}
	}
}

func isSSEContentType(contentType string) bool {
	return contentType == ContentTypeSSE ||
		len(contentType) > len(ContentTypeSSE) && contentType[:len(ContentTypeSSE)] == ContentTypeSSE
}

type safeDialer struct {
	dialer               *net.Dialer
	allowPrivateNetworks []string
	blockedIPv4Ranges    []*net.IPNet
	blockedIPv6Ranges    []*net.IPNet
}

func newSafeDialer(timeout time.Duration, allowPrivateNetworks []string) *safeDialer {
	d := &safeDialer{
		dialer: &net.Dialer{
			Timeout: timeout,
		},
		allowPrivateNetworks: allowPrivateNetworks,
	}

	ipv4Blocked := []string{
		"127.0.0.0/8",
		"169.254.0.0/16",
		"169.254.169.254/32",
		"100.100.100.200/32",
		"192.0.0.0/24",
		"0.0.0.0/8",
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
	// First check if IP is explicitly allowed
	if d.isPrivateNetworkAllowed(ip) {
		return false
	}

	if ip4 := ip.To4(); ip4 != nil {
		for _, blocked := range d.blockedIPv4Ranges {
			if blocked.Contains(ip4) {
				return true
			}
		}

		rfc1918 := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
		for _, cidr := range rfc1918 {
			_, ipnet, _ := net.ParseCIDR(cidr)
			if ipnet.Contains(ip4) {
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
	for _, cidrStr := range d.allowPrivateNetworks {
		_, cidr, err := net.ParseCIDR(cidrStr)
		if err != nil {
			continue
		}
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}
