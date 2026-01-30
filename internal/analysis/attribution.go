// Package analysis provides telemetry aggregation and metrics computation.
package analysis

import (
	"fmt"
	"strings"
)

const (
	OriginClientNetwork = "client_network"
	OriginGateway       = "gateway"
	OriginMCPServer     = "mcp_server"
	OriginUpstreamAPI   = "upstream_api"
	OriginUnknown       = "unknown"
)

type Attribution struct {
	Origin     string
	Confidence float64
	Rationale  string
}

func AttributeFailure(op OperationResult) Attribution {
	if op.OK {
		return Attribution{
			Origin:     OriginUnknown,
			Confidence: 0.0,
			Rationale:  "Operation succeeded - no failure to attribute",
		}
	}

	if op.ErrorType == "" {
		return Attribution{
			Origin:     OriginUnknown,
			Confidence: 0.1,
			Rationale:  "No error type provided - unable to classify",
		}
	}

	errorType := strings.ToLower(op.ErrorType)

	if errorType == "dns_error" || strings.Contains(errorType, "dns") {
		return Attribution{
			Origin:     OriginClientNetwork,
			Confidence: 0.9,
			Rationale:  fmt.Sprintf("DNS resolution failed (%s) - network connectivity issue", op.ErrorType),
		}
	}

	if errorType == "connect_error" || strings.Contains(errorType, "connect") ||
		strings.Contains(errorType, "connection") {
		return Attribution{
			Origin:     OriginClientNetwork,
			Confidence: 0.9,
			Rationale:  fmt.Sprintf("Connection failed (%s) - network connectivity issue", op.ErrorType),
		}
	}

	if errorType == "tls_error" || strings.Contains(errorType, "tls") ||
		strings.Contains(errorType, "ssl") || strings.Contains(errorType, "certificate") {
		return Attribution{
			Origin:     OriginClientNetwork,
			Confidence: 0.9,
			Rationale:  fmt.Sprintf("TLS/SSL error (%s) - network security issue", op.ErrorType),
		}
	}

	if errorType == "timeout" || strings.Contains(errorType, "timeout") {
		return Attribution{
			Origin:     OriginUnknown,
			Confidence: 0.3,
			Rationale:  fmt.Sprintf("Timeout occurred (%s) - could be network, gateway, server, or upstream", op.ErrorType),
		}
	}

	if errorType == "http_error" || strings.HasPrefix(errorType, "http_") {
		return classifyHTTPError(op.ErrorType)
	}

	if errorType == "jsonrpc_error" || strings.Contains(errorType, "jsonrpc") ||
		strings.Contains(errorType, "json-rpc") || strings.Contains(errorType, "json_rpc") {
		return Attribution{
			Origin:     OriginMCPServer,
			Confidence: 0.7,
			Rationale:  fmt.Sprintf("JSON-RPC error (%s) - MCP server protocol issue", op.ErrorType),
		}
	}

	if errorType == "mcp_error" || strings.HasPrefix(errorType, "mcp_") ||
		strings.Contains(errorType, "mcp") {
		return Attribution{
			Origin:     OriginMCPServer,
			Confidence: 0.8,
			Rationale:  fmt.Sprintf("MCP error (%s) - MCP server issue", op.ErrorType),
		}
	}

	if errorType == "tool_error" || strings.Contains(errorType, "tool") {
		return Attribution{
			Origin:     OriginUpstreamAPI,
			Confidence: 0.7,
			Rationale:  fmt.Sprintf("Tool returned error (%s) - upstream API failure", op.ErrorType),
		}
	}

	if errorType == "protocol_error" || strings.Contains(errorType, "protocol") {
		return Attribution{
			Origin:     OriginMCPServer,
			Confidence: 0.6,
			Rationale:  fmt.Sprintf("Protocol error (%s) - MCP server protocol issue", op.ErrorType),
		}
	}

	return Attribution{
		Origin:     OriginUnknown,
		Confidence: 0.1,
		Rationale:  fmt.Sprintf("Unknown error type (%s) - unable to classify origin", op.ErrorType),
	}
}

func classifyHTTPError(errorType string) Attribution {
	statusCode := extractHTTPStatusCode(errorType)

	if statusCode == 0 {
		return Attribution{
			Origin:     OriginGateway,
			Confidence: 0.5,
			Rationale:  fmt.Sprintf("HTTP error (%s) - likely gateway or server issue", errorType),
		}
	}

	if statusCode == 429 {
		return Attribution{
			Origin:     OriginGateway,
			Confidence: 0.8,
			Rationale:  fmt.Sprintf("HTTP 429 response (%s) - gateway rate limiting", errorType),
		}
	}

	if statusCode >= 400 && statusCode < 500 {
		return Attribution{
			Origin:     OriginGateway,
			Confidence: 0.7,
			Rationale:  fmt.Sprintf("HTTP %d response (%s) - gateway client error", statusCode, errorType),
		}
	}

	if statusCode >= 500 && statusCode < 600 {
		return Attribution{
			Origin:     OriginGateway,
			Confidence: 0.6,
			Rationale:  fmt.Sprintf("HTTP %d response (%s) - likely gateway or server issue", statusCode, errorType),
		}
	}

	return Attribution{
		Origin:     OriginGateway,
		Confidence: 0.5,
		Rationale:  fmt.Sprintf("HTTP %d response (%s) - gateway response", statusCode, errorType),
	}
}

func extractHTTPStatusCode(errorType string) int {
	var code int
	for i := 0; i < len(errorType)-2; i++ {
		if errorType[i] >= '0' && errorType[i] <= '9' &&
			errorType[i+1] >= '0' && errorType[i+1] <= '9' &&
			errorType[i+2] >= '0' && errorType[i+2] <= '9' {
			code = int(errorType[i]-'0')*100 + int(errorType[i+1]-'0')*10 + int(errorType[i+2]-'0')
			if code >= 100 && code < 600 {
				return code
			}
		}
	}
	return 0
}
