package analysis

import (
	"testing"
)

func TestAttributeFailure_SuccessfulOperation(t *testing.T) {
	op := OperationResult{
		Operation: "tools_call",
		OK:        true,
	}

	attr := AttributeFailure(op)

	if attr.Origin != OriginUnknown {
		t.Errorf("expected origin %s, got %s", OriginUnknown, attr.Origin)
	}
	if attr.Confidence != 0.0 {
		t.Errorf("expected confidence 0.0, got %f", attr.Confidence)
	}
}

func TestAttributeFailure_EmptyErrorType(t *testing.T) {
	op := OperationResult{
		Operation: "tools_call",
		OK:        false,
		ErrorType: "",
	}

	attr := AttributeFailure(op)

	if attr.Origin != OriginUnknown {
		t.Errorf("expected origin %s, got %s", OriginUnknown, attr.Origin)
	}
	if attr.Confidence != 0.1 {
		t.Errorf("expected confidence 0.1, got %f", attr.Confidence)
	}
}

func TestAttributeFailure_DNSError(t *testing.T) {
	tests := []struct {
		name      string
		errorType string
	}{
		{"exact match", "dns_error"},
		{"contains dns", "dns_lookup_failed"},
		{"uppercase", "DNS_ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := OperationResult{
				Operation: "initialize",
				OK:        false,
				ErrorType: tt.errorType,
			}

			attr := AttributeFailure(op)

			if attr.Origin != OriginClientNetwork {
				t.Errorf("expected origin %s, got %s", OriginClientNetwork, attr.Origin)
			}
			if attr.Confidence != 0.9 {
				t.Errorf("expected confidence 0.9, got %f", attr.Confidence)
			}
		})
	}
}

func TestAttributeFailure_ConnectError(t *testing.T) {
	tests := []struct {
		name      string
		errorType string
	}{
		{"exact match", "connect_error"},
		{"contains connect", "connection_refused"},
		{"connection reset", "connection_reset"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := OperationResult{
				Operation: "initialize",
				OK:        false,
				ErrorType: tt.errorType,
			}

			attr := AttributeFailure(op)

			if attr.Origin != OriginClientNetwork {
				t.Errorf("expected origin %s, got %s", OriginClientNetwork, attr.Origin)
			}
			if attr.Confidence != 0.9 {
				t.Errorf("expected confidence 0.9, got %f", attr.Confidence)
			}
		})
	}
}

func TestAttributeFailure_TLSError(t *testing.T) {
	tests := []struct {
		name      string
		errorType string
	}{
		{"exact match", "tls_error"},
		{"contains tls", "tls_handshake_failed"},
		{"ssl error", "ssl_error"},
		{"certificate error", "certificate_expired"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := OperationResult{
				Operation: "initialize",
				OK:        false,
				ErrorType: tt.errorType,
			}

			attr := AttributeFailure(op)

			if attr.Origin != OriginClientNetwork {
				t.Errorf("expected origin %s, got %s", OriginClientNetwork, attr.Origin)
			}
			if attr.Confidence != 0.9 {
				t.Errorf("expected confidence 0.9, got %f", attr.Confidence)
			}
		})
	}
}

func TestAttributeFailure_Timeout(t *testing.T) {
	tests := []struct {
		name      string
		errorType string
	}{
		{"exact match", "timeout"},
		{"read timeout", "read_timeout"},
		{"request timeout", "request_timeout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := OperationResult{
				Operation: "tools_call",
				OK:        false,
				ErrorType: tt.errorType,
			}

			attr := AttributeFailure(op)

			if attr.Origin != OriginUnknown {
				t.Errorf("expected origin %s, got %s", OriginUnknown, attr.Origin)
			}
			if attr.Confidence != 0.3 {
				t.Errorf("expected confidence 0.3, got %f", attr.Confidence)
			}
		})
	}
}

func TestAttributeFailure_HTTP429(t *testing.T) {
	tests := []struct {
		name      string
		errorType string
	}{
		{"http_429", "http_429"},
		{"http_error_429", "http_error_429"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := OperationResult{
				Operation: "tools_call",
				OK:        false,
				ErrorType: tt.errorType,
			}

			attr := AttributeFailure(op)

			if attr.Origin != OriginGateway {
				t.Errorf("expected origin %s, got %s", OriginGateway, attr.Origin)
			}
			if attr.Confidence != 0.8 {
				t.Errorf("expected confidence 0.8, got %f", attr.Confidence)
			}
		})
	}
}

func TestAttributeFailure_HTTP4xx(t *testing.T) {
	tests := []struct {
		name       string
		errorType  string
		confidence float64
	}{
		{"http_400", "http_400", 0.7},
		{"http_401", "http_401", 0.7},
		{"http_403", "http_403", 0.7},
		{"http_404", "http_404", 0.7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := OperationResult{
				Operation: "tools_call",
				OK:        false,
				ErrorType: tt.errorType,
			}

			attr := AttributeFailure(op)

			if attr.Origin != OriginGateway {
				t.Errorf("expected origin %s, got %s", OriginGateway, attr.Origin)
			}
			if attr.Confidence != tt.confidence {
				t.Errorf("expected confidence %f, got %f", tt.confidence, attr.Confidence)
			}
		})
	}
}

func TestAttributeFailure_HTTP5xx(t *testing.T) {
	tests := []struct {
		name      string
		errorType string
	}{
		{"http_500", "http_500"},
		{"http_502", "http_502"},
		{"http_503", "http_503"},
		{"http_504", "http_504"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := OperationResult{
				Operation: "tools_call",
				OK:        false,
				ErrorType: tt.errorType,
			}

			attr := AttributeFailure(op)

			if attr.Origin != OriginGateway {
				t.Errorf("expected origin %s, got %s", OriginGateway, attr.Origin)
			}
			if attr.Confidence != 0.6 {
				t.Errorf("expected confidence 0.6, got %f", attr.Confidence)
			}
		})
	}
}

func TestAttributeFailure_GenericHTTPError(t *testing.T) {
	op := OperationResult{
		Operation: "tools_call",
		OK:        false,
		ErrorType: "http_error",
	}

	attr := AttributeFailure(op)

	if attr.Origin != OriginGateway {
		t.Errorf("expected origin %s, got %s", OriginGateway, attr.Origin)
	}
	if attr.Confidence != 0.5 {
		t.Errorf("expected confidence 0.5, got %f", attr.Confidence)
	}
}

func TestAttributeFailure_JSONRPCError(t *testing.T) {
	tests := []struct {
		name      string
		errorType string
	}{
		{"exact match", "jsonrpc_error"},
		{"contains jsonrpc", "jsonrpc_parse_error"},
		{"json-rpc", "json-rpc_error"},
		{"json_rpc", "json_rpc_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := OperationResult{
				Operation: "tools_call",
				OK:        false,
				ErrorType: tt.errorType,
			}

			attr := AttributeFailure(op)

			if attr.Origin != OriginMCPServer {
				t.Errorf("expected origin %s, got %s", OriginMCPServer, attr.Origin)
			}
			if attr.Confidence != 0.7 {
				t.Errorf("expected confidence 0.7, got %f", attr.Confidence)
			}
		})
	}
}

func TestAttributeFailure_MCPError(t *testing.T) {
	tests := []struct {
		name      string
		errorType string
	}{
		{"exact match", "mcp_error"},
		{"mcp prefix", "mcp_invalid_request"},
		{"contains mcp", "invalid_mcp_response"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := OperationResult{
				Operation: "tools_call",
				OK:        false,
				ErrorType: tt.errorType,
			}

			attr := AttributeFailure(op)

			if attr.Origin != OriginMCPServer {
				t.Errorf("expected origin %s, got %s", OriginMCPServer, attr.Origin)
			}
			if attr.Confidence != 0.8 {
				t.Errorf("expected confidence 0.8, got %f", attr.Confidence)
			}
		})
	}
}

func TestAttributeFailure_ToolError(t *testing.T) {
	tests := []struct {
		name      string
		errorType string
	}{
		{"exact match", "tool_error"},
		{"contains tool", "tool_execution_failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := OperationResult{
				Operation: "tools_call",
				OK:        false,
				ErrorType: tt.errorType,
			}

			attr := AttributeFailure(op)

			if attr.Origin != OriginUpstreamAPI {
				t.Errorf("expected origin %s, got %s", OriginUpstreamAPI, attr.Origin)
			}
			if attr.Confidence != 0.7 {
				t.Errorf("expected confidence 0.7, got %f", attr.Confidence)
			}
		})
	}
}

func TestAttributeFailure_ProtocolError(t *testing.T) {
	tests := []struct {
		name      string
		errorType string
	}{
		{"exact match", "protocol_error"},
		{"contains protocol", "protocol_violation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := OperationResult{
				Operation: "tools_call",
				OK:        false,
				ErrorType: tt.errorType,
			}

			attr := AttributeFailure(op)

			if attr.Origin != OriginMCPServer {
				t.Errorf("expected origin %s, got %s", OriginMCPServer, attr.Origin)
			}
			if attr.Confidence != 0.6 {
				t.Errorf("expected confidence 0.6, got %f", attr.Confidence)
			}
		})
	}
}

func TestAttributeFailure_UnknownErrorType(t *testing.T) {
	op := OperationResult{
		Operation: "tools_call",
		OK:        false,
		ErrorType: "some_random_error",
	}

	attr := AttributeFailure(op)

	if attr.Origin != OriginUnknown {
		t.Errorf("expected origin %s, got %s", OriginUnknown, attr.Origin)
	}
	if attr.Confidence != 0.1 {
		t.Errorf("expected confidence 0.1, got %f", attr.Confidence)
	}
}

func TestAttributeFailure_RationaleContainsErrorType(t *testing.T) {
	tests := []struct {
		name      string
		errorType string
	}{
		{"dns_error", "dns_error"},
		{"http_500", "http_500"},
		{"tool_error", "tool_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := OperationResult{
				Operation: "tools_call",
				OK:        false,
				ErrorType: tt.errorType,
			}

			attr := AttributeFailure(op)

			if attr.Rationale == "" {
				t.Error("expected non-empty rationale")
			}
			if len(attr.Rationale) < 10 {
				t.Errorf("rationale too short: %s", attr.Rationale)
			}
		})
	}
}

func TestExtractHTTPStatusCode(t *testing.T) {
	tests := []struct {
		name      string
		errorType string
		expected  int
	}{
		{"http_429", "http_429", 429},
		{"http_500", "http_500", 500},
		{"http_error_401", "http_error_401", 401},
		{"http200", "http200", 200},
		{"no code", "http_error", 0},
		{"invalid code 999", "http_999", 0},
		{"code 100", "http_100", 100},
		{"code 599", "http_599", 599},
		{"embedded code", "error_http_503_gateway", 503},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := extractHTTPStatusCode(tt.errorType)
			if code != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, code)
			}
		})
	}
}

func TestOriginConstants(t *testing.T) {
	if OriginClientNetwork != "client_network" {
		t.Errorf("OriginClientNetwork = %s, want client_network", OriginClientNetwork)
	}
	if OriginGateway != "gateway" {
		t.Errorf("OriginGateway = %s, want gateway", OriginGateway)
	}
	if OriginMCPServer != "mcp_server" {
		t.Errorf("OriginMCPServer = %s, want mcp_server", OriginMCPServer)
	}
	if OriginUpstreamAPI != "upstream_api" {
		t.Errorf("OriginUpstreamAPI = %s, want upstream_api", OriginUpstreamAPI)
	}
	if OriginUnknown != "unknown" {
		t.Errorf("OriginUnknown = %s, want unknown", OriginUnknown)
	}
}

func TestAttributeFailure_AllOperationTypes(t *testing.T) {
	operations := []string{"initialize", "tools_list", "tools_call", "ping"}

	for _, op := range operations {
		t.Run(op, func(t *testing.T) {
			result := OperationResult{
				Operation: op,
				OK:        false,
				ErrorType: "dns_error",
			}

			attr := AttributeFailure(result)

			if attr.Origin != OriginClientNetwork {
				t.Errorf("expected origin %s, got %s", OriginClientNetwork, attr.Origin)
			}
		})
	}
}

func TestAttributeFailure_ConfidenceRange(t *testing.T) {
	errorTypes := []string{
		"dns_error",
		"connect_error",
		"tls_error",
		"timeout",
		"http_429",
		"http_400",
		"http_500",
		"http_error",
		"jsonrpc_error",
		"mcp_error",
		"tool_error",
		"protocol_error",
		"unknown_error",
		"",
	}

	for _, errType := range errorTypes {
		t.Run(errType, func(t *testing.T) {
			op := OperationResult{
				Operation: "tools_call",
				OK:        false,
				ErrorType: errType,
			}

			attr := AttributeFailure(op)

			if attr.Confidence < 0.0 || attr.Confidence > 1.0 {
				t.Errorf("confidence %f out of range [0.0, 1.0]", attr.Confidence)
			}
		})
	}
}

func TestAttributeFailure_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name           string
		errorType      string
		expectedOrigin string
	}{
		{"uppercase DNS", "DNS_ERROR", OriginClientNetwork},
		{"mixed case TLS", "TLS_Error", OriginClientNetwork},
		{"uppercase HTTP", "HTTP_500", OriginGateway},
		{"mixed case MCP", "MCP_Error", OriginMCPServer},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := OperationResult{
				Operation: "tools_call",
				OK:        false,
				ErrorType: tt.errorType,
			}

			attr := AttributeFailure(op)

			if attr.Origin != tt.expectedOrigin {
				t.Errorf("expected origin %s, got %s", tt.expectedOrigin, attr.Origin)
			}
		})
	}
}
