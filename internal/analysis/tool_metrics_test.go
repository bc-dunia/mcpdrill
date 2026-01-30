package analysis

import (
	"testing"
)

func TestCalculateArgumentDepth(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected int
	}{
		{
			name:     "nil value",
			input:    nil,
			expected: 0,
		},
		{
			name:     "string primitive",
			input:    "hello",
			expected: 0,
		},
		{
			name:     "int primitive",
			input:    42,
			expected: 0,
		},
		{
			name:     "bool primitive",
			input:    true,
			expected: 0,
		},
		{
			name:     "float primitive",
			input:    3.14,
			expected: 0,
		},
		{
			name:     "empty object",
			input:    map[string]any{},
			expected: 1,
		},
		{
			name:     "empty array",
			input:    []any{},
			expected: 1,
		},
		{
			name:     "flat object with primitives",
			input:    map[string]any{"a": 1, "b": "test"},
			expected: 1,
		},
		{
			name:     "depth 2 nested object",
			input:    map[string]any{"a": map[string]any{"b": 1}},
			expected: 2,
		},
		{
			name:     "depth 3 nested object",
			input:    map[string]any{"a": map[string]any{"b": map[string]any{"c": 1}}},
			expected: 3,
		},
		{
			name:     "depth 4 nested object",
			input:    map[string]any{"a": map[string]any{"b": map[string]any{"c": map[string]any{"d": 1}}}},
			expected: 4,
		},
		{
			name:     "array with primitives",
			input:    []any{1, 2, 3},
			expected: 1,
		},
		{
			name:     "array with nested objects",
			input:    []any{map[string]any{"a": 1}, map[string]any{"b": 2}},
			expected: 2,
		},
		{
			name:     "array with deeply nested objects",
			input:    []any{map[string]any{"a": map[string]any{"b": map[string]any{"c": 1}}}},
			expected: 4,
		},
		{
			name:     "mixed depth - max wins",
			input:    map[string]any{"shallow": 1, "deep": map[string]any{"level2": map[string]any{"level3": 1}}},
			expected: 3,
		},
		{
			name:     "nested arrays",
			input:    []any{[]any{[]any{1}}},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateArgumentDepth(tt.input)
			if result != tt.expected {
				t.Errorf("CalculateArgumentDepth() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestAggregateToolMetrics_Empty(t *testing.T) {
	result := AggregateToolMetrics(nil)
	if len(result) != 0 {
		t.Errorf("Expected empty map for nil input, got %d entries", len(result))
	}

	result = AggregateToolMetrics([]OperationLog{})
	if len(result) != 0 {
		t.Errorf("Expected empty map for empty input, got %d entries", len(result))
	}
}

func TestAggregateToolMetrics_SingleTool(t *testing.T) {
	logs := []OperationLog{
		{Operation: "tools/call", ToolName: "echo", LatencyMs: 100, OK: true, ArgumentSize: 50, ResultSize: 100},
		{Operation: "tools/call", ToolName: "echo", LatencyMs: 200, OK: true, ArgumentSize: 50, ResultSize: 100},
		{Operation: "tools/call", ToolName: "echo", LatencyMs: 300, OK: false, ArgumentSize: 50, ResultSize: 0, ExecutionError: true},
	}

	result := AggregateToolMetrics(logs)

	if len(result) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(result))
	}

	metrics, ok := result["echo"]
	if !ok {
		t.Fatal("Expected metrics for 'echo' tool")
	}

	if metrics.TotalCalls != 3 {
		t.Errorf("TotalCalls = %d, want 3", metrics.TotalCalls)
	}
	if metrics.SuccessCount != 2 {
		t.Errorf("SuccessCount = %d, want 2", metrics.SuccessCount)
	}
	if metrics.ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", metrics.ErrorCount)
	}
	if metrics.AvgLatencyMs != 200 {
		t.Errorf("AvgLatencyMs = %f, want 200", metrics.AvgLatencyMs)
	}
}

func TestAggregateToolMetrics_MultipleTools(t *testing.T) {
	logs := []OperationLog{
		{Operation: "tools/call", ToolName: "echo", LatencyMs: 100, OK: true, ArgumentSize: 50, ResultSize: 100},
		{Operation: "tools/call", ToolName: "add", LatencyMs: 50, OK: true, ArgumentSize: 20, ResultSize: 10},
		{Operation: "tools/call", ToolName: "echo", LatencyMs: 200, OK: true, ArgumentSize: 60, ResultSize: 120},
		{Operation: "tools/call", ToolName: "multiply", LatencyMs: 75, OK: false, ArgumentSize: 30, ResultSize: 0},
	}

	result := AggregateToolMetrics(logs)

	if len(result) != 3 {
		t.Fatalf("Expected 3 tools, got %d", len(result))
	}

	echoMetrics := result["echo"]
	if echoMetrics.TotalCalls != 2 {
		t.Errorf("echo.TotalCalls = %d, want 2", echoMetrics.TotalCalls)
	}
	if echoMetrics.SuccessCount != 2 {
		t.Errorf("echo.SuccessCount = %d, want 2", echoMetrics.SuccessCount)
	}
	if echoMetrics.AvgLatencyMs != 150 {
		t.Errorf("echo.AvgLatencyMs = %f, want 150", echoMetrics.AvgLatencyMs)
	}
	expectedAvgPayload := (50 + 100 + 60 + 120) / 2
	if echoMetrics.AvgPayloadSize != expectedAvgPayload {
		t.Errorf("echo.AvgPayloadSize = %d, want %d", echoMetrics.AvgPayloadSize, expectedAvgPayload)
	}

	addMetrics := result["add"]
	if addMetrics.TotalCalls != 1 {
		t.Errorf("add.TotalCalls = %d, want 1", addMetrics.TotalCalls)
	}

	multiplyMetrics := result["multiply"]
	if multiplyMetrics.ErrorCount != 1 {
		t.Errorf("multiply.ErrorCount = %d, want 1", multiplyMetrics.ErrorCount)
	}
}

func TestAggregateToolMetrics_Percentiles(t *testing.T) {
	logs := make([]OperationLog, 100)
	for i := 0; i < 100; i++ {
		logs[i] = OperationLog{
			Operation: "tools/call",
			ToolName:  "latency_test",
			LatencyMs: int64(i + 1),
			OK:        true,
		}
	}

	result := AggregateToolMetrics(logs)
	metrics := result["latency_test"]

	if metrics.TotalCalls != 100 {
		t.Errorf("TotalCalls = %d, want 100", metrics.TotalCalls)
	}

	expectedAvg := 50.5
	if metrics.AvgLatencyMs != expectedAvg {
		t.Errorf("AvgLatencyMs = %f, want %f", metrics.AvgLatencyMs, expectedAvg)
	}

	if metrics.P95LatencyMs < 95 || metrics.P95LatencyMs > 96 {
		t.Errorf("P95LatencyMs = %f, want ~95", metrics.P95LatencyMs)
	}

	if metrics.P99LatencyMs < 99 || metrics.P99LatencyMs > 100 {
		t.Errorf("P99LatencyMs = %f, want ~99", metrics.P99LatencyMs)
	}
}

func TestAggregateToolMetrics_LargePayloads(t *testing.T) {
	largePayload1KB := 1024
	largePayload10KB := 10240

	logs := []OperationLog{
		{Operation: "tools/call", ToolName: "large_tool", LatencyMs: 100, OK: true, ArgumentSize: largePayload1KB, ResultSize: 500},
		{Operation: "tools/call", ToolName: "large_tool", LatencyMs: 200, OK: true, ArgumentSize: largePayload10KB, ResultSize: 5000},
	}

	result := AggregateToolMetrics(logs)
	metrics := result["large_tool"]

	expectedTotalPayload := largePayload1KB + 500 + largePayload10KB + 5000
	expectedAvgPayload := expectedTotalPayload / 2

	if metrics.AvgPayloadSize != expectedAvgPayload {
		t.Errorf("AvgPayloadSize = %d, want %d", metrics.AvgPayloadSize, expectedAvgPayload)
	}
}

func TestAggregateToolMetrics_ErrorScenarios(t *testing.T) {
	logs := []OperationLog{
		{Operation: "tools/call", ToolName: "error_tool", LatencyMs: 50, OK: false, ParseError: true},
		{Operation: "tools/call", ToolName: "error_tool", LatencyMs: 100, OK: false, ExecutionError: true},
		{Operation: "tools/call", ToolName: "error_tool", LatencyMs: 75, OK: true},
	}

	result := AggregateToolMetrics(logs)
	metrics := result["error_tool"]

	if metrics.TotalCalls != 3 {
		t.Errorf("TotalCalls = %d, want 3", metrics.TotalCalls)
	}
	if metrics.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1", metrics.SuccessCount)
	}
	if metrics.ErrorCount != 2 {
		t.Errorf("ErrorCount = %d, want 2", metrics.ErrorCount)
	}
}

func TestAggregateToolMetrics_IgnoresEmptyToolName(t *testing.T) {
	logs := []OperationLog{
		{Operation: "tools/list", ToolName: "", LatencyMs: 100, OK: true},
		{Operation: "ping", ToolName: "", LatencyMs: 50, OK: true},
		{Operation: "tools/call", ToolName: "echo", LatencyMs: 75, OK: true},
	}

	result := AggregateToolMetrics(logs)

	if len(result) != 1 {
		t.Fatalf("Expected 1 tool (non-empty names only), got %d", len(result))
	}

	if _, ok := result["echo"]; !ok {
		t.Error("Expected metrics for 'echo' tool")
	}
}

func TestAggregateToolMetrics_PayloadSizeCalculation(t *testing.T) {
	logs := []OperationLog{
		{Operation: "tools/call", ToolName: "payload_test", LatencyMs: 100, OK: true, ArgumentSize: 100, ResultSize: 200},
		{Operation: "tools/call", ToolName: "payload_test", LatencyMs: 100, OK: true, ArgumentSize: 300, ResultSize: 400},
	}

	result := AggregateToolMetrics(logs)
	metrics := result["payload_test"]

	expectedAvg := (100 + 200 + 300 + 400) / 2
	if metrics.AvgPayloadSize != expectedAvg {
		t.Errorf("AvgPayloadSize = %d, want %d", metrics.AvgPayloadSize, expectedAvg)
	}
}

func TestAggregateToolMetrics_AllToolsHaveCorrectName(t *testing.T) {
	logs := []OperationLog{
		{Operation: "tools/call", ToolName: "tool_a", LatencyMs: 100, OK: true},
		{Operation: "tools/call", ToolName: "tool_b", LatencyMs: 100, OK: true},
		{Operation: "tools/call", ToolName: "tool_c", LatencyMs: 100, OK: true},
	}

	result := AggregateToolMetrics(logs)

	for name, metrics := range result {
		if metrics.ToolName != name {
			t.Errorf("Tool %s has mismatched ToolName field: %s", name, metrics.ToolName)
		}
	}
}

func TestCalculateArgumentDepth_LargeStructure(t *testing.T) {
	depth := 10
	var current any = "leaf"

	for i := 0; i < depth; i++ {
		current = map[string]any{"nested": current}
	}

	result := CalculateArgumentDepth(current)
	if result != depth {
		t.Errorf("CalculateArgumentDepth() = %d, want %d for depth %d", result, depth, depth)
	}
}

func TestAggregateToolMetrics_VeryLargeDataset(t *testing.T) {
	logs := make([]OperationLog, 10000)
	for i := 0; i < 10000; i++ {
		logs[i] = OperationLog{
			Operation:    "tools/call",
			ToolName:     "stress_tool",
			LatencyMs:    int64(i%1000 + 1),
			OK:           i%10 != 0,
			ArgumentSize: 100,
			ResultSize:   200,
		}
	}

	result := AggregateToolMetrics(logs)
	metrics := result["stress_tool"]

	if metrics.TotalCalls != 10000 {
		t.Errorf("TotalCalls = %d, want 10000", metrics.TotalCalls)
	}
	if metrics.SuccessCount != 9000 {
		t.Errorf("SuccessCount = %d, want 9000", metrics.SuccessCount)
	}
	if metrics.ErrorCount != 1000 {
		t.Errorf("ErrorCount = %d, want 1000", metrics.ErrorCount)
	}
}

func TestCalculateArgumentDepth_ComplexMixedStructure(t *testing.T) {
	input := map[string]any{
		"users": []any{
			map[string]any{
				"name": "Alice",
				"metadata": map[string]any{
					"tags": []any{"admin", "active"},
					"settings": map[string]any{
						"theme": "dark",
					},
				},
			},
		},
		"config": map[string]any{
			"version": 1,
		},
	}

	result := CalculateArgumentDepth(input)
	if result != 5 {
		t.Errorf("CalculateArgumentDepth() = %d, want 5", result)
	}
}

func BenchmarkCalculateArgumentDepth(b *testing.B) {
	input := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": map[string]any{
					"d": "value",
				},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateArgumentDepth(input)
	}
}

func BenchmarkAggregateToolMetrics(b *testing.B) {
	logs := make([]OperationLog, 1000)
	for i := 0; i < 1000; i++ {
		logs[i] = OperationLog{
			Operation:    "tools/call",
			ToolName:     "bench_tool",
			LatencyMs:    int64(i + 1),
			OK:           true,
			ArgumentSize: 100,
			ResultSize:   200,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		AggregateToolMetrics(logs)
	}
}

func TestToolMetrics_JSONTags(t *testing.T) {
	metrics := &ToolMetrics{
		ToolName:       "test",
		TotalCalls:     10,
		SuccessCount:   8,
		ErrorCount:     2,
		AvgLatencyMs:   150.5,
		P95LatencyMs:   200.0,
		P99LatencyMs:   250.0,
		AvgPayloadSize: 1024,
	}

	if metrics.ToolName != "test" {
		t.Errorf("ToolName mismatch")
	}
	if metrics.TotalCalls != 10 {
		t.Errorf("TotalCalls mismatch")
	}
}

func TestOperationLog_AllFields(t *testing.T) {
	log := OperationLog{
		Operation:      "tools/call",
		ToolName:       "test_tool",
		LatencyMs:      100,
		OK:             true,
		ArgumentSize:   500,
		ResultSize:     1000,
		ArgumentDepth:  3,
		ParseError:     false,
		ExecutionError: false,
	}

	if log.Operation != "tools/call" {
		t.Error("Operation field mismatch")
	}
	if log.ToolName != "test_tool" {
		t.Error("ToolName field mismatch")
	}
	if log.LatencyMs != 100 {
		t.Error("LatencyMs field mismatch")
	}
	if !log.OK {
		t.Error("OK field mismatch")
	}
	if log.ArgumentSize != 500 {
		t.Error("ArgumentSize field mismatch")
	}
	if log.ResultSize != 1000 {
		t.Error("ResultSize field mismatch")
	}
	if log.ArgumentDepth != 3 {
		t.Error("ArgumentDepth field mismatch")
	}
	if log.ParseError {
		t.Error("ParseError field mismatch")
	}
	if log.ExecutionError {
		t.Error("ExecutionError field mismatch")
	}
}

func TestAggregateToolMetrics_ZeroLatency(t *testing.T) {
	logs := []OperationLog{
		{Operation: "tools/call", ToolName: "fast_tool", LatencyMs: 0, OK: true},
		{Operation: "tools/call", ToolName: "fast_tool", LatencyMs: 0, OK: true},
	}

	result := AggregateToolMetrics(logs)
	metrics := result["fast_tool"]

	if metrics.AvgLatencyMs != 0 {
		t.Errorf("AvgLatencyMs = %f, want 0", metrics.AvgLatencyMs)
	}
	if metrics.P95LatencyMs != 0 {
		t.Errorf("P95LatencyMs = %f, want 0", metrics.P95LatencyMs)
	}
	if metrics.P99LatencyMs != 0 {
		t.Errorf("P99LatencyMs = %f, want 0", metrics.P99LatencyMs)
	}
}

func TestAggregateToolMetrics_SingleCall(t *testing.T) {
	logs := []OperationLog{
		{Operation: "tools/call", ToolName: "single_tool", LatencyMs: 50, OK: true, ArgumentSize: 100, ResultSize: 200},
	}

	result := AggregateToolMetrics(logs)
	metrics := result["single_tool"]

	if metrics.TotalCalls != 1 {
		t.Errorf("TotalCalls = %d, want 1", metrics.TotalCalls)
	}
	if metrics.AvgLatencyMs != 50 {
		t.Errorf("AvgLatencyMs = %f, want 50", metrics.AvgLatencyMs)
	}
	if metrics.P95LatencyMs != 50 {
		t.Errorf("P95LatencyMs = %f, want 50", metrics.P95LatencyMs)
	}
	if metrics.P99LatencyMs != 50 {
		t.Errorf("P99LatencyMs = %f, want 50", metrics.P99LatencyMs)
	}
	if metrics.AvgPayloadSize != 300 {
		t.Errorf("AvgPayloadSize = %d, want 300", metrics.AvgPayloadSize)
	}
}
