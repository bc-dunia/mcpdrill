package analysis

import (
	"testing"
)

func TestNormalizeError_UUID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase uuid",
			input:    "failed to find resource 550e8400-e29b-41d4-a716-446655440000",
			expected: "failed to find resource <UUID>",
		},
		{
			name:     "uppercase uuid",
			input:    "error with ID 550e8400-e29b-41d4-a716-446655440000",
			expected: "error with ID <UUID>",
		},
		{
			name:     "mixed case uuid",
			input:    "session 550e8400-e29b-41d4-a716-446655440000 expired",
			expected: "session <UUID> expired",
		},
		{
			name:     "multiple uuids",
			input:    "copy 550e8400-e29b-41d4-a716-446655440000 to 660e8400-e29b-41d4-a716-446655440001",
			expected: "copy <UUID> to <UUID>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeError(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeError(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeError_Numbers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "port number",
			input:    "connection refused to localhost:3000",
			expected: "connection refused to localhost:<NUM>",
		},
		{
			name:     "multiple numbers",
			input:    "error at line 42, column 15",
			expected: "error at line <NUM>, column <NUM>",
		},
		{
			name:     "large number",
			input:    "timeout after 30000ms",
			expected: "timeout after <NUM>ms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeError(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeError(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeError_Timestamps(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "iso timestamp",
			input:    "event at 2024-01-27T15:30:45 failed",
			expected: "event at <TS> failed",
		},
		{
			name:     "multiple timestamps",
			input:    "from 2024-01-27T10:00:00 to 2024-01-27T12:00:00",
			expected: "from <TS> to <TS>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeError(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeError(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeError_IPAddresses(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "ipv4 address",
			input:    "connection to 192.168.1.100 failed",
			expected: "connection to <IP> failed",
		},
		{
			name:     "localhost ip",
			input:    "bind to 127.0.0.1:8080 failed",
			expected: "bind to <IP>:<NUM> failed",
		},
		{
			name:     "multiple ips",
			input:    "route from 10.0.0.1 to 10.0.0.2 unreachable",
			expected: "route from <IP> to <IP> unreachable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeError(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeError(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeError_FilePaths(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "unix path",
			input:    "file not found: /var/log/app.log",
			expected: "file not found: <PATH>",
		},
		{
			name:     "nested path",
			input:    "cannot read /home/user/data/config.json",
			expected: "cannot read <PATH>",
		},
		{
			name:     "path with numbers",
			input:    "error in /tmp/session_12345/data",
			expected: "error in <PATH>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeError(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeError(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeError_Combined(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "uuid and number",
			input:    "request 550e8400-e29b-41d4-a716-446655440000 failed after 5000ms",
			expected: "request <UUID> failed after <NUM>ms",
		},
		{
			name:     "ip and port",
			input:    "connection refused to 192.168.1.1:8080",
			expected: "connection refused to <IP>:<NUM>",
		},
		{
			name:     "timestamp and path",
			input:    "2024-01-27T10:30:00 error reading /var/log/app.log",
			expected: "<TS> error reading <PATH>",
		},
		{
			name:     "all patterns",
			input:    "2024-01-27T10:30:00 session 550e8400-e29b-41d4-a716-446655440000 from 192.168.1.1:3000 failed reading /tmp/data after 100ms",
			expected: "<TS> session <UUID> from <IP>:<NUM> failed reading <PATH> after <NUM>ms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeError(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeError(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractSignatures_Empty(t *testing.T) {
	result := ExtractSignatures([]ErrorLog{}, 10)
	if len(result) != 0 {
		t.Errorf("ExtractSignatures([]) = %d signatures, want 0", len(result))
	}
}

func TestExtractSignatures_SingleError(t *testing.T) {
	errors := []ErrorLog{
		{
			TimestampMs: 1000,
			Operation:   "tools/call",
			ToolName:    "api_client",
			ErrorType:   "connection refused to localhost:3000",
		},
	}

	result := ExtractSignatures(errors, 10)

	if len(result) != 1 {
		t.Fatalf("ExtractSignatures() = %d signatures, want 1", len(result))
	}

	sig := result[0]
	if sig.Pattern != "connection refused to localhost:<NUM>" {
		t.Errorf("Pattern = %q, want %q", sig.Pattern, "connection refused to localhost:<NUM>")
	}
	if sig.Count != 1 {
		t.Errorf("Count = %d, want 1", sig.Count)
	}
	if sig.FirstSeenMs != 1000 {
		t.Errorf("FirstSeenMs = %d, want 1000", sig.FirstSeenMs)
	}
	if sig.LastSeenMs != 1000 {
		t.Errorf("LastSeenMs = %d, want 1000", sig.LastSeenMs)
	}
	if len(sig.AffectedOperations) != 1 || sig.AffectedOperations[0] != "tools/call" {
		t.Errorf("AffectedOperations = %v, want [tools/call]", sig.AffectedOperations)
	}
	if len(sig.AffectedTools) != 1 || sig.AffectedTools[0] != "api_client" {
		t.Errorf("AffectedTools = %v, want [api_client]", sig.AffectedTools)
	}
	if sig.SampleError != "connection refused to localhost:3000" {
		t.Errorf("SampleError = %q, want %q", sig.SampleError, "connection refused to localhost:3000")
	}
}

func TestExtractSignatures_Grouping(t *testing.T) {
	errors := []ErrorLog{
		{TimestampMs: 1000, Operation: "tools/call", ToolName: "api_client", ErrorType: "connection refused to localhost:3000"},
		{TimestampMs: 2000, Operation: "tools/call", ToolName: "api_client", ErrorType: "connection refused to localhost:3001"},
		{TimestampMs: 3000, Operation: "resources/read", ToolName: "file_reader", ErrorType: "connection refused to localhost:8080"},
	}

	result := ExtractSignatures(errors, 10)

	if len(result) != 1 {
		t.Fatalf("ExtractSignatures() = %d signatures, want 1 (all should group)", len(result))
	}

	sig := result[0]
	if sig.Count != 3 {
		t.Errorf("Count = %d, want 3", sig.Count)
	}
	if sig.FirstSeenMs != 1000 {
		t.Errorf("FirstSeenMs = %d, want 1000", sig.FirstSeenMs)
	}
	if sig.LastSeenMs != 3000 {
		t.Errorf("LastSeenMs = %d, want 3000", sig.LastSeenMs)
	}
	if len(sig.AffectedOperations) != 2 {
		t.Errorf("AffectedOperations = %v, want 2 operations", sig.AffectedOperations)
	}
	if len(sig.AffectedTools) != 2 {
		t.Errorf("AffectedTools = %v, want 2 tools", sig.AffectedTools)
	}
}

func TestExtractSignatures_Ranking(t *testing.T) {
	errors := []ErrorLog{
		{TimestampMs: 1000, Operation: "op1", ErrorType: "error A"},
		{TimestampMs: 2000, Operation: "op1", ErrorType: "error B"},
		{TimestampMs: 3000, Operation: "op1", ErrorType: "error B"},
		{TimestampMs: 4000, Operation: "op1", ErrorType: "error B"},
		{TimestampMs: 5000, Operation: "op1", ErrorType: "error C"},
		{TimestampMs: 6000, Operation: "op1", ErrorType: "error C"},
	}

	result := ExtractSignatures(errors, 10)

	if len(result) != 3 {
		t.Fatalf("ExtractSignatures() = %d signatures, want 3", len(result))
	}

	if result[0].Pattern != "error B" || result[0].Count != 3 {
		t.Errorf("First signature = %q (count %d), want 'error B' (count 3)", result[0].Pattern, result[0].Count)
	}
	if result[1].Pattern != "error C" || result[1].Count != 2 {
		t.Errorf("Second signature = %q (count %d), want 'error C' (count 2)", result[1].Pattern, result[1].Count)
	}
	if result[2].Pattern != "error A" || result[2].Count != 1 {
		t.Errorf("Third signature = %q (count %d), want 'error A' (count 1)", result[2].Pattern, result[2].Count)
	}
}

func TestExtractSignatures_TopN(t *testing.T) {
	errors := []ErrorLog{
		{TimestampMs: 1000, Operation: "op1", ErrorType: "error A"},
		{TimestampMs: 2000, Operation: "op1", ErrorType: "error B"},
		{TimestampMs: 3000, Operation: "op1", ErrorType: "error C"},
		{TimestampMs: 4000, Operation: "op1", ErrorType: "error D"},
		{TimestampMs: 5000, Operation: "op1", ErrorType: "error E"},
	}

	result := ExtractSignatures(errors, 3)

	if len(result) != 3 {
		t.Errorf("ExtractSignatures(topN=3) = %d signatures, want 3", len(result))
	}
}

func TestExtractSignatures_SkipsEmptyErrorType(t *testing.T) {
	errors := []ErrorLog{
		{TimestampMs: 1000, Operation: "op1", ErrorType: ""},
		{TimestampMs: 2000, Operation: "op1", ErrorType: "real error"},
	}

	result := ExtractSignatures(errors, 10)

	if len(result) != 1 {
		t.Fatalf("ExtractSignatures() = %d signatures, want 1", len(result))
	}
	if result[0].Pattern != "real error" {
		t.Errorf("Pattern = %q, want 'real error'", result[0].Pattern)
	}
}

func TestExtractSignatures_DeterministicOrdering(t *testing.T) {
	errors := []ErrorLog{
		{TimestampMs: 1000, Operation: "op1", ErrorType: "error Z"},
		{TimestampMs: 2000, Operation: "op1", ErrorType: "error A"},
		{TimestampMs: 3000, Operation: "op1", ErrorType: "error M"},
	}

	result := ExtractSignatures(errors, 10)

	if len(result) != 3 {
		t.Fatalf("ExtractSignatures() = %d signatures, want 3", len(result))
	}

	if result[0].Pattern != "error A" {
		t.Errorf("First signature = %q, want 'error A' (alphabetical for same count)", result[0].Pattern)
	}
	if result[1].Pattern != "error M" {
		t.Errorf("Second signature = %q, want 'error M'", result[1].Pattern)
	}
	if result[2].Pattern != "error Z" {
		t.Errorf("Third signature = %q, want 'error Z'", result[2].Pattern)
	}
}

func TestExtractSignatures_NoToolOrOperation(t *testing.T) {
	errors := []ErrorLog{
		{TimestampMs: 1000, ErrorType: "some error"},
	}

	result := ExtractSignatures(errors, 10)

	if len(result) != 1 {
		t.Fatalf("ExtractSignatures() = %d signatures, want 1", len(result))
	}

	sig := result[0]
	if len(sig.AffectedOperations) != 0 {
		t.Errorf("AffectedOperations = %v, want empty", sig.AffectedOperations)
	}
	if len(sig.AffectedTools) != 0 {
		t.Errorf("AffectedTools = %v, want empty", sig.AffectedTools)
	}
}
