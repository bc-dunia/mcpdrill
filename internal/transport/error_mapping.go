package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"syscall"
)

func MapError(err error) *OperationError {
	if err == nil {
		return nil
	}

	if opErr, ok := err.(*OperationError); ok {
		return opErr
	}

	if errors.Is(err, context.Canceled) {
		return &OperationError{
			Type:    ErrorTypeCancelled,
			Code:    CodeCancelled,
			Message: "operation cancelled",
		}
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return &OperationError{
			Type:    ErrorTypeTimeout,
			Code:    CodeRequestTimeout,
			Message: "request timeout exceeded",
		}
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return mapDNSError(dnsErr)
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return mapNetOpError(opErr)
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return &OperationError{
				Type:    ErrorTypeTimeout,
				Code:    CodeRequestTimeout,
				Message: fmt.Sprintf("request timeout: %s", urlErr.Op),
			}
		}
		return MapError(urlErr.Err)
	}

	var tlsRecordErr *tls.RecordHeaderError
	if errors.As(err, &tlsRecordErr) {
		return &OperationError{
			Type:    ErrorTypeTLS,
			Code:    CodeTLSHandshakeFailed,
			Message: "TLS record header error",
		}
	}

	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return &OperationError{
			Type:    ErrorTypeTLS,
			Code:    CodeTLSCertificateError,
			Message: fmt.Sprintf("certificate verification failed: %v", certErr.Err),
		}
	}

	var unknownAuthErr x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthErr) {
		return &OperationError{
			Type:    ErrorTypeTLS,
			Code:    CodeTLSCertificateError,
			Message: "certificate signed by unknown authority",
		}
	}

	var certInvalidErr x509.CertificateInvalidError
	if errors.As(err, &certInvalidErr) {
		return &OperationError{
			Type:    ErrorTypeTLS,
			Code:    CodeTLSCertificateError,
			Message: fmt.Sprintf("certificate invalid: %s", certInvalidErr.Detail),
		}
	}

	var hostErr x509.HostnameError
	if errors.As(err, &hostErr) {
		return &OperationError{
			Type:    ErrorTypeTLS,
			Code:    CodeTLSCertificateError,
			Message: fmt.Sprintf("certificate hostname mismatch: %s", hostErr.Host),
		}
	}

	errStr := err.Error()
	if strings.Contains(errStr, "tls:") || strings.Contains(errStr, "TLS") {
		return &OperationError{
			Type:    ErrorTypeTLS,
			Code:    CodeTLSHandshakeFailed,
			Message: errStr,
		}
	}

	return &OperationError{
		Type:    ErrorTypeUnknown,
		Code:    ErrorCode("UNKNOWN"),
		Message: err.Error(),
	}
}

func mapDNSError(err *net.DNSError) *OperationError {
	code := CodeDNSLookupFailed
	if err.IsTimeout {
		code = CodeDNSTimeout
	}
	return &OperationError{
		Type:    ErrorTypeDNS,
		Code:    code,
		Message: fmt.Sprintf("DNS lookup failed for %s: %s", err.Name, err.Err),
		Details: map[string]interface{}{
			"host":       err.Name,
			"is_timeout": err.IsTimeout,
		},
	}
}

func mapNetOpError(err *net.OpError) *OperationError {
	if err.Timeout() {
		code := CodeRequestTimeout
		if err.Op == "dial" {
			code = CodeConnectTimeout
		}
		return &OperationError{
			Type:    ErrorTypeTimeout,
			Code:    code,
			Message: fmt.Sprintf("%s timeout", err.Op),
		}
	}

	if err.Op == "dial" {
		return mapDialError(err)
	}

	if err.Op == "read" || err.Op == "write" {
		return mapIOError(err)
	}

	return &OperationError{
		Type:    ErrorTypeConnect,
		Code:    ErrorCode(fmt.Sprintf("NET_%s_ERROR", strings.ToUpper(err.Op))),
		Message: err.Error(),
	}
}

func mapDialError(err *net.OpError) *OperationError {
	if err.Err != nil {
		var errno syscall.Errno
		if errors.As(err.Err, &errno) {
			return mapSyscallError(errno, err)
		}

		var opErr *net.OpError
		if errors.As(err.Err, &opErr) {
			return mapNetOpError(opErr)
		}

		errStr := err.Err.Error()
		if strings.Contains(errStr, "connection refused") {
			return &OperationError{
				Type:    ErrorTypeConnect,
				Code:    CodeConnectionRefused,
				Message: fmt.Sprintf("connection refused to %s", err.Addr),
				Details: map[string]interface{}{"address": err.Addr.String()},
			}
		}
		if strings.Contains(errStr, "connection reset") {
			return &OperationError{
				Type:    ErrorTypeConnect,
				Code:    CodeConnectionReset,
				Message: fmt.Sprintf("connection reset by %s", err.Addr),
				Details: map[string]interface{}{"address": err.Addr.String()},
			}
		}
		if strings.Contains(errStr, "network is unreachable") {
			return &OperationError{
				Type:    ErrorTypeConnect,
				Code:    CodeNetworkUnreachable,
				Message: "network is unreachable",
			}
		}
	}

	return &OperationError{
		Type:    ErrorTypeConnect,
		Code:    ErrorCode("CONNECT_FAILED"),
		Message: err.Error(),
	}
}

func mapIOError(err *net.OpError) *OperationError {
	if err.Err != nil {
		errStr := err.Err.Error()
		if strings.Contains(errStr, "connection reset") {
			return &OperationError{
				Type:    ErrorTypeConnect,
				Code:    CodeConnectionReset,
				Message: "connection reset during " + err.Op,
			}
		}
	}

	return &OperationError{
		Type:    ErrorTypeConnect,
		Code:    ErrorCode(fmt.Sprintf("%s_ERROR", strings.ToUpper(err.Op))),
		Message: err.Error(),
	}
}

func mapSyscallError(errno syscall.Errno, opErr *net.OpError) *OperationError {
	switch errno {
	case syscall.ECONNREFUSED:
		return &OperationError{
			Type:    ErrorTypeConnect,
			Code:    CodeConnectionRefused,
			Message: "connection refused",
			Details: map[string]interface{}{"address": opErr.Addr.String()},
		}
	case syscall.ECONNRESET:
		return &OperationError{
			Type:    ErrorTypeConnect,
			Code:    CodeConnectionReset,
			Message: "connection reset by peer",
		}
	case syscall.ENETUNREACH:
		return &OperationError{
			Type:    ErrorTypeConnect,
			Code:    CodeNetworkUnreachable,
			Message: "network is unreachable",
		}
	case syscall.ETIMEDOUT:
		return &OperationError{
			Type:    ErrorTypeTimeout,
			Code:    CodeConnectTimeout,
			Message: "connection timed out",
		}
	default:
		return &OperationError{
			Type:    ErrorTypeConnect,
			Code:    ErrorCode(fmt.Sprintf("SYSCALL_%d", errno)),
			Message: errno.Error(),
		}
	}
}

func MapHTTPStatus(status int) *OperationError {
	return MapHTTPStatusWithBody(status, "")
}

func MapHTTPStatusWithBody(status int, responseBody string) *OperationError {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == 400:
		msg := "bad request"
		if responseBody != "" {
			msg = fmt.Sprintf("bad request: %s", responseBody)
		}
		return &OperationError{
			Type:    ErrorTypeHTTP,
			Code:    CodeHTTPBadRequest,
			Message: msg,
			Details: map[string]interface{}{"http_status": status},
		}
	case status == 401:
		msg := "unauthorized - authentication required"
		if responseBody != "" {
			msg = fmt.Sprintf("unauthorized: %s", responseBody)
		}
		return &OperationError{
			Type:    ErrorTypeHTTP,
			Code:    CodeHTTPUnauthorized,
			Message: msg,
			Details: map[string]interface{}{"http_status": status},
		}
	case status == 403:
		msg := "forbidden - access denied"
		if responseBody != "" {
			msg = fmt.Sprintf("forbidden: %s", responseBody)
		}
		return &OperationError{
			Type:    ErrorTypeHTTP,
			Code:    CodeHTTPForbidden,
			Message: msg,
			Details: map[string]interface{}{"http_status": status},
		}
	case status == 404:
		msg := "not found"
		if responseBody != "" {
			msg = fmt.Sprintf("not found: %s", responseBody)
		}
		return &OperationError{
			Type:    ErrorTypeHTTP,
			Code:    CodeHTTPNotFound,
			Message: msg,
			Details: map[string]interface{}{"http_status": status},
		}
	case status == 429:
		msg := "rate limited"
		if responseBody != "" {
			msg = fmt.Sprintf("rate limited: %s", responseBody)
		}
		return &OperationError{
			Type:    ErrorTypeRateLimited,
			Code:    CodeHTTPRateLimited,
			Message: msg,
			Details: map[string]interface{}{"http_status": status},
		}
	case status >= 500:
		msg := fmt.Sprintf("server error: %d", status)
		if responseBody != "" {
			msg = fmt.Sprintf("server error (%d): %s", status, responseBody)
		}
		return &OperationError{
			Type:    ErrorTypeHTTP,
			Code:    CodeHTTPServerError,
			Message: msg,
			Details: map[string]interface{}{"http_status": status},
		}
	default:
		msg := fmt.Sprintf("HTTP error: %d", status)
		if responseBody != "" {
			msg = fmt.Sprintf("HTTP error (%d): %s", status, responseBody)
		}
		return &OperationError{
			Type:    ErrorTypeHTTP,
			Code:    ErrorCode(fmt.Sprintf("HTTP_%d", status)),
			Message: msg,
			Details: map[string]interface{}{"http_status": status},
		}
	}
}

func MapJSONRPCError(code int, message string, data []byte) *OperationError {
	var errCode ErrorCode
	switch code {
	case -32700:
		errCode = CodeJSONRPCParseError
	case -32600:
		errCode = CodeJSONRPCInvalidRequest
	case -32601:
		errCode = CodeJSONRPCMethodNotFound
	case -32602:
		errCode = CodeJSONRPCInvalidParams
	case -32603:
		errCode = CodeJSONRPCInternalError
	default:
		errCode = ErrorCode(fmt.Sprintf("JSONRPC_%d", code))
	}

	details := map[string]interface{}{
		"jsonrpc_code": code,
	}
	if len(data) > 0 {
		details["data"] = string(data)
	}

	return &OperationError{
		Type:    ErrorTypeJSONRPC,
		Code:    errCode,
		Message: message,
		Details: details,
	}
}

func MapMCPError(code string, message string) *OperationError {
	return &OperationError{
		Type:    ErrorTypeMCP,
		Code:    CodeMCPError,
		Message: message,
		Details: map[string]interface{}{
			"mcp_error_code": code,
		},
	}
}

func MapToolError(toolName string, content []ToolContent) *OperationError {
	var messages []string
	for _, c := range content {
		if c.Text != "" {
			messages = append(messages, c.Text)
		}
	}

	return &OperationError{
		Type:    ErrorTypeTool,
		Code:    CodeToolError,
		Message: fmt.Sprintf("tool %s returned error", toolName),
		Details: map[string]interface{}{
			"tool_name": toolName,
			"content":   messages,
		},
	}
}

func MapProtocolError(message string) *OperationError {
	return &OperationError{
		Type:    ErrorTypeProtocol,
		Code:    CodeJSONParseError,
		Message: message,
	}
}

func NewStreamStallError(stallDuration int) *OperationError {
	return &OperationError{
		Type:    ErrorTypeStreamStall,
		Code:    CodeStreamStallTimeout,
		Message: fmt.Sprintf("stream stalled for %dms", stallDuration),
		Details: map[string]interface{}{
			"stall_duration_ms": stallDuration,
		},
	}
}

func NewEOFError(context string) *OperationError {
	return &OperationError{
		Type:    ErrorTypeConnect,
		Code:    CodeConnectionEOF,
		Message: fmt.Sprintf("unexpected EOF during %s", context),
		Details: map[string]interface{}{
			"context": context,
		},
	}
}

func NewSSEDisconnectError(eventsReceived int, lastEventID string) *OperationError {
	details := map[string]interface{}{
		"events_received": eventsReceived,
	}
	if lastEventID != "" {
		details["last_event_id"] = lastEventID
	}
	return &OperationError{
		Type:    ErrorTypeConnect,
		Code:    CodeSSEDisconnect,
		Message: fmt.Sprintf("SSE stream disconnected after %d events", eventsReceived),
		Details: details,
	}
}
