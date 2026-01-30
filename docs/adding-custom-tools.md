# Adding Custom Tools - Developer Guide

A comprehensive guide for developers who want to add custom mock tools to MCP Drill's mock server.

## Table of Contents

- [Mock Tool Architecture](#mock-tool-architecture)
- [Adding a New Tool Handler](#adding-a-new-tool-handler)
- [Input Schema Design](#input-schema-design)
- [Testing Your Tool](#testing-your-tool)
- [Configurable Behavior](#configurable-behavior)
- [Performance Considerations](#performance-considerations)

---

## Mock Tool Architecture

### Overview

MCP Drill's mock server implements the MCP (Model Context Protocol) specification for tool execution. The architecture consists of:

```
┌─────────────────────────────────────────┐
│         MCP Client (VU Engine)          │
└──────────────────┬──────────────────────┘
                   │ HTTP POST /mcp (JSON-RPC)
                   ▼
┌─────────────────────────────────────────┐
│          Mock Server Handler            │
│  ┌───────────────────────────────────┐  │
│  │  1. Parse JSON-RPC request        │  │
│  │  2. Extract tool_name & arguments │  │
│  │  3. Lookup tool in registry       │  │
│  │  4. Execute tool handler          │  │
│  │  5. Return JSON-RPC response      │  │
│  └───────────────────────────────────┘  │
└──────────────────┬──────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────┐
│         Tool Registry (tools.go)        │
│  ┌───────────────────────────────────┐  │
│  │  defaultTools map[string]*MockTool│  │
│  │  - fast_echo                      │  │
│  │  - validate_email                 │  │
│  │  - calculate                      │  │
│  │  - ... (20 total)                 │  │
│  └───────────────────────────────────┘  │
└─────────────────────────────────────────┘
```

### Core Types

**MockTool** (`internal/mockserver/tools.go`):
```go
type MockTool struct {
    Name           string          // Tool identifier
    Description    string          // Human-readable description
    InputSchema    json.RawMessage // JSON schema for arguments
    Handler        ToolHandler     // Execution function
    DefaultLatency time.Duration   // Default execution time
}
```

**ToolHandler** function signature:
```go
type ToolHandler func(
    ctx context.Context,
    args map[string]interface{},
    config *Config,
) (*ToolResult, error)
```

**ToolResult** structure:
```go
type ToolResult struct {
    Content []ToolContent // Array of content items
    IsError bool          // True if execution failed
}

type ToolContent struct {
    Type string `json:"type"` // "text", "image", "resource"
    Text string `json:"text,omitempty"`
}
```

---

## Adding a New Tool Handler

### Step 1: Define the Tool

Add your tool to the `defaultTools` map in `internal/mockserver/tools.go`:

```go
var defaultTools = map[string]*MockTool{
    // ... existing tools ...
    
    "my_custom_tool": {
        Name:           "my_custom_tool",
        Description:    "Brief description of what this tool does",
        InputSchema:    json.RawMessage(`{
            "type": "object",
            "properties": {
                "input_field": { "type": "string" }
            },
            "required": ["input_field"]
        }`),
        Handler:        handleMyCustomTool,
        DefaultLatency: 100 * time.Millisecond,
    },
}
```

### Step 2: Implement the Handler Function

Add the handler function in `internal/mockserver/tools.go`:

```go
func handleMyCustomTool(
    ctx context.Context,
    args map[string]interface{},
    config *Config,
) (*ToolResult, error) {
    // 1. Apply configurable latency
    latency := 100 * time.Millisecond
    if tc := config.GetToolConfig("my_custom_tool"); tc != nil && tc.LatencyMs > 0 {
        latency = time.Duration(tc.LatencyMs) * time.Millisecond
    }
    
    // 2. Respect context cancellation
    select {
    case <-time.After(latency):
    case <-ctx.Done():
        return nil, ctx.Err()
    }
    
    // 3. Extract and validate arguments
    inputField, ok := args["input_field"].(string)
    if !ok {
        return &ToolResult{
            Content: []ToolContent{{
                Type: "text",
                Text: "invalid input_field: expected string",
            }},
            IsError: true,
        }, nil
    }
    
    // 4. Perform tool logic
    result := processInput(inputField)
    
    // 5. Return result
    return &ToolResult{
        Content: []ToolContent{{
            Type: "text",
            Text: result,
        }},
    }, nil
}
```

### Step 3: Rebuild and Test

```bash
# Rebuild the mock server
go build -o mcpdrill-server ./cmd/server

# Start the server
./mcpdrill-server --addr :3000

# Test your tool
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "my_custom_tool",
      "arguments": {"input_field": "test"}
    }
  }'
```

---

## Input Schema Design

### JSON Schema Best Practices

**1. Use descriptive property names:**
```go
// ❌ Bad: Unclear names
InputSchema: json.RawMessage(`{
    "type": "object",
    "properties": {
        "a": { "type": "string" },
        "b": { "type": "number" }
    }
}`)

// ✅ Good: Clear, descriptive names
InputSchema: json.RawMessage(`{
    "type": "object",
    "properties": {
        "email_address": { "type": "string" },
        "retry_count": { "type": "number" }
    }
}`)
```

**2. Mark required fields explicitly:**
```go
InputSchema: json.RawMessage(`{
    "type": "object",
    "properties": {
        "required_field": { "type": "string" },
        "optional_field": { "type": "string" }
    },
    "required": ["required_field"]
}`)
```

**3. Add validation constraints:**
```go
InputSchema: json.RawMessage(`{
    "type": "object",
    "properties": {
        "email": {
            "type": "string",
            "pattern": "^[^@]+@[^@]+\\.[^@]+$"
        },
        "age": {
            "type": "integer",
            "minimum": 0,
            "maximum": 150
        },
        "tags": {
            "type": "array",
            "items": { "type": "string" },
            "minItems": 1,
            "maxItems": 10
        }
    }
}`)
```

**4. Use enums for fixed choices:**
```go
InputSchema: json.RawMessage(`{
    "type": "object",
    "properties": {
        "operation": {
            "type": "string",
            "enum": ["create", "update", "delete"]
        }
    }
}`)
```

### Complex Schema Example

```go
"advanced_tool": {
    Name:        "advanced_tool",
    Description: "Demonstrates complex schema with nested objects",
    InputSchema: json.RawMessage(`{
        "type": "object",
        "properties": {
            "user": {
                "type": "object",
                "properties": {
                    "name": { "type": "string", "minLength": 1 },
                    "email": { "type": "string", "pattern": "^[^@]+@[^@]+\\.[^@]+$" },
                    "age": { "type": "integer", "minimum": 0 }
                },
                "required": ["name", "email"]
            },
            "preferences": {
                "type": "object",
                "properties": {
                    "theme": { "type": "string", "enum": ["light", "dark"] },
                    "notifications": { "type": "boolean" }
                }
            },
            "tags": {
                "type": "array",
                "items": { "type": "string" },
                "minItems": 1
            }
        },
        "required": ["user"]
    }`),
    Handler:        handleAdvancedTool,
    DefaultLatency: 150 * time.Millisecond,
},
```

---

## Testing Your Tool

### Unit Tests

Create a test file `internal/mockserver/my_custom_tool_test.go`:

```go
package mockserver

import (
    "context"
    "testing"
    "time"
)

func TestMyCustomTool(t *testing.T) {
    tests := []struct {
        name        string
        args        map[string]interface{}
        wantError   bool
        wantContent string
    }{
        {
            name: "valid input",
            args: map[string]interface{}{
                "input_field": "test",
            },
            wantError:   false,
            wantContent: "processed: test",
        },
        {
            name: "missing required field",
            args: map[string]interface{}{},
            wantError:   true,
            wantContent: "invalid input_field",
        },
        {
            name: "wrong type",
            args: map[string]interface{}{
                "input_field": 123,
            },
            wantError:   true,
            wantContent: "expected string",
        },
    }
    
    config := &Config{}
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ctx := context.Background()
            result, err := handleMyCustomTool(ctx, tt.args, config)
            
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            
            if result.IsError != tt.wantError {
                t.Errorf("IsError = %v, want %v", result.IsError, tt.wantError)
            }
            
            if len(result.Content) == 0 {
                t.Fatal("expected content, got none")
            }
            
            if !strings.Contains(result.Content[0].Text, tt.wantContent) {
                t.Errorf("content = %q, want to contain %q", result.Content[0].Text, tt.wantContent)
            }
        })
    }
}

func TestMyCustomToolLatency(t *testing.T) {
    config := &Config{}
    args := map[string]interface{}{"input_field": "test"}
    
    start := time.Now()
    ctx := context.Background()
    _, err := handleMyCustomTool(ctx, args, config)
    elapsed := time.Since(start)
    
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    
    // Should take at least 100ms (default latency)
    if elapsed < 100*time.Millisecond {
        t.Errorf("latency = %v, want >= 100ms", elapsed)
    }
}

func TestMyCustomToolCancellation(t *testing.T) {
    config := &Config{}
    args := map[string]interface{}{"input_field": "test"}
    
    ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
    defer cancel()
    
    _, err := handleMyCustomTool(ctx, args, config)
    
    if err != context.DeadlineExceeded {
        t.Errorf("expected context.DeadlineExceeded, got %v", err)
    }
}
```

Run tests:
```bash
go test ./internal/mockserver -v -run TestMyCustomTool
```

### E2E Tests

Add E2E tests in `test/e2e/tool_execution_test.go`:

```go
func TestMyCustomToolE2E(t *testing.T) {
    // 1. Start mock server
    mockServer := startMockServer(t)
    defer mockServer.Close()
    
    // 2. Create control plane
    cp := startControlPlane(t)
    defer cp.Shutdown()
    
    // 3. Start worker
    worker := startWorker(t, cp.URL)
    defer worker.Stop()
    
    // 4. Create test config
    config := &Config{
        ScenarioID: "test-my-custom-tool",
        Target: Target{
            Kind:      "server",
            URL:       mockServer.URL,
            Transport: "streamable_http",
        },
        Stages: []Stage{
            {
                StageID:    "stg_0000000000000001",
                Stage:      "ramp",
                Enabled:    true,
                DurationMs: 10000,
                Load:       Load{TargetVUs: 10},
            },
        },
        Workload: Workload{
            OpMix: []Operation{
                {
                    Operation: "tools/call",
                    Weight:    1,
                    ToolName:  "my_custom_tool",
                    Arguments: map[string]interface{}{
                        "input_field": "test",
                    },
                },
            },
        },
    }
    
    // 5. Execute test
    runID := createRun(t, cp.URL, config)
    startRun(t, cp.URL, runID)
    waitForCompletion(t, cp.URL, runID, 30*time.Second)
    
    // 6. Verify results
    metrics := getMetrics(t, cp.URL, runID)
    toolMetrics := metrics.ByTool["my_custom_tool"]
    
    if toolMetrics.TotalCalls == 0 {
        t.Fatal("expected tool calls, got 0")
    }
    
    if toolMetrics.ErrorCount > 0 {
        t.Errorf("expected 0 errors, got %d", toolMetrics.ErrorCount)
    }
    
    if toolMetrics.AvgLatencyMs < 100 {
        t.Errorf("expected latency >= 100ms, got %.2f", toolMetrics.AvgLatencyMs)
    }
}
```

---

## Configurable Behavior

### Using Config.GetToolConfig()

Allow users to customize tool behavior via URL parameters:

```go
func handleMyCustomTool(
    ctx context.Context,
    args map[string]interface{},
    config *Config,
) (*ToolResult, error) {
    // Default values
    latency := 100 * time.Millisecond
    shouldFail := false
    errorMessage := "Tool execution failed"
    
    // Override with config
    if tc := config.GetToolConfig("my_custom_tool"); tc != nil {
        if tc.LatencyMs > 0 {
            latency = time.Duration(tc.LatencyMs) * time.Millisecond
        }
        if tc.ForceError {
            shouldFail = true
        }
        if tc.ErrorMessage != "" {
            errorMessage = tc.ErrorMessage
        }
    }
    
    // Apply latency
    select {
    case <-time.After(latency):
    case <-ctx.Done():
        return nil, ctx.Err()
    }
    
    // Force error if configured
    if shouldFail {
        return &ToolResult{
            Content: []ToolContent{{Type: "text", Text: errorMessage}},
            IsError: true,
        }, nil
    }
    
    // Normal execution
    // ...
}
```

### Configuration Options

Users can configure tools via URL parameters:

```
http://localhost:3000?tool_config=my_custom_tool:latency_ms=500,force_error=true
```

Available options:
- `latency_ms` - Override default latency
- `force_error` - Force tool to fail
- `error_message` - Custom error message
- `disabled` - Disable tool entirely

---

## Performance Considerations

### 1. Context Cancellation

**Always respect context cancellation** to allow graceful shutdown:

```go
// ✅ Good: Respects context
select {
case <-time.After(latency):
case <-ctx.Done():
    return nil, ctx.Err()
}

// ❌ Bad: Ignores context
time.Sleep(latency)
```

### 2. Avoid Blocking Operations

Use non-blocking patterns for I/O:

```go
// ✅ Good: Non-blocking with timeout
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()

result, err := doExternalCall(ctx)
if err != nil {
    return &ToolResult{
        Content: []ToolContent{{Type: "text", Text: err.Error()}},
        IsError: true,
    }, nil
}

// ❌ Bad: Blocking without timeout
result := doExternalCall() // Could hang forever
```

### 3. Memory Management

For tools that generate large payloads:

```go
func handleLargePayload(
    ctx context.Context,
    args map[string]interface{},
    config *Config,
) (*ToolResult, error) {
    sizeKB, _ := args["size_kb"].(float64)
    
    // Enforce maximum size
    if sizeKB > 1024 {
        sizeKB = 1024
    }
    
    // Generate data incrementally
    items := make([]map[string]interface{}, 0, int(sizeKB))
    for i := 0; i < int(sizeKB); i++ {
        // Check context periodically
        if i%100 == 0 {
            select {
            case <-ctx.Done():
                return nil, ctx.Err()
            default:
            }
        }
        
        items = append(items, generateItem(i))
    }
    
    jsonBytes, _ := json.Marshal(items)
    return &ToolResult{
        Content: []ToolContent{{Type: "text", Text: string(jsonBytes)}},
    }, nil
}
```

### 4. Concurrency Safety

If your tool uses shared state, protect it with mutexes:

```go
var (
    callCounter int
    counterMu   sync.Mutex
)

func handleStatefulTool(
    ctx context.Context,
    args map[string]interface{},
    config *Config,
) (*ToolResult, error) {
    counterMu.Lock()
    callCounter++
    currentCount := callCounter
    counterMu.Unlock()
    
    return &ToolResult{
        Content: []ToolContent{{
            Type: "text",
            Text: fmt.Sprintf("Call #%d", currentCount),
        }},
    }, nil
}
```

### 5. Error Handling

Return errors as `ToolResult` with `IsError: true`, not as Go errors:

```go
// ✅ Good: Error as ToolResult
if invalidInput {
    return &ToolResult{
        Content: []ToolContent{{Type: "text", Text: "Invalid input"}},
        IsError: true,
    }, nil
}

// ❌ Bad: Go error (breaks MCP protocol)
if invalidInput {
    return nil, fmt.Errorf("invalid input")
}
```

---

## Advanced Examples

### Tool with Multiple Content Types

```go
func handleMultiContentTool(
    ctx context.Context,
    args map[string]interface{},
    config *Config,
) (*ToolResult, error) {
    return &ToolResult{
        Content: []ToolContent{
            {Type: "text", Text: "Processing complete"},
            {Type: "text", Text: `{"result": "success", "count": 42}`},
        },
    }, nil
}
```

### Tool with Dynamic Behavior

```go
func handleDynamicTool(
    ctx context.Context,
    args map[string]interface{},
    config *Config,
) (*ToolResult, error) {
    mode, _ := args["mode"].(string)
    
    switch mode {
    case "fast":
        time.Sleep(10 * time.Millisecond)
    case "slow":
        time.Sleep(500 * time.Millisecond)
    default:
        time.Sleep(100 * time.Millisecond)
    }
    
    return &ToolResult{
        Content: []ToolContent{{
            Type: "text",
            Text: fmt.Sprintf("Executed in %s mode", mode),
        }},
    }, nil
}
```

---

## Next Steps

- **[Tool Testing Guide](tool-testing-guide.md)** - Learn how to test your custom tools
- **[Example Configurations](../examples/tool-testing/)** - See example test configs
- **[API Reference](../README.md#api-reference)** - Complete API documentation
