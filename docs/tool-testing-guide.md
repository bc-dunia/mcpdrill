# Tool Testing Guide

A comprehensive guide to testing MCP tools using MCP Drill's mock server and tool testing features.

## Table of Contents

- [Introduction](#introduction)
- [Available Mock Tools](#available-mock-tools)
- [Configuring Tool Calls](#configuring-tool-calls)
- [Argument Schema Guide](#argument-schema-guide)
- [Interpreting Tool Metrics](#interpreting-tool-metrics)
- [Troubleshooting Tool Failures](#troubleshooting-tool-failures)

---

## Introduction

MCP Drill includes a comprehensive mock server with 27 built-in tools for testing MCP tool execution workflows. These tools cover:

- **Data manipulation** - Transform JSON, process text, operate on lists
- **Validation** - Email validation, schema validation
- **Computation** - Mathematical expressions, hashing
- **API simulation** - Weather, geocoding, currency conversion
- **File operations** - Read, write, list (simulated)
- **Testing utilities** - Large payloads, random latency, conditional errors
- **Advanced testing** - Degradation, flakiness, rate limits, circuit breakers, backpressure, stateful operations

Tool testing enables you to:
- Validate tool argument schemas
- Measure tool execution latency
- Track payload sizes and argument complexity
- Test error handling and edge cases
- Simulate realistic API behavior

---

## Available Mock Tools

### Original Testing Tools

#### fast_echo

**Description:** Echoes input immediately (no artificial latency)

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "message": { "type": "string" }
  },
  "required": ["message"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "fast_echo",
  "arguments": { "message": "Hello, world!" }
}
```

**Returns:** `{ "type": "text", "text": "echo: Hello, world!" }`

---

#### slow_echo

**Description:** Echoes input with 200ms latency

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "message": { "type": "string" }
  },
  "required": ["message"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "slow_echo",
  "arguments": { "message": "Slow response" }
}
```

**Returns:** `{ "type": "text", "text": "echo: Slow response" }`

---

#### error_tool

**Description:** Always returns an error (for error handling testing)

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "error_message": { "type": "string" }
  }
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "error_tool",
  "arguments": { "error_message": "Custom error message" }
}
```

**Returns:** Error with custom message

---

#### timeout_tool

**Description:** Never responds (for timeout testing)

**Input Schema:**
```json
{
  "type": "object",
  "properties": {}
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "timeout_tool",
  "arguments": {}
}
```

**Returns:** Times out after configured timeout period

---

#### streaming_tool

**Description:** Returns streaming SSE response with configurable chunks

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "chunks": { "type": "integer" },
    "delay_ms": { "type": "integer" }
  }
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "streaming_tool",
  "arguments": { "chunks": 5, "delay_ms": 100 }
}
```

**Returns:** 5 chunks with 100ms delay between each

---

### Data Manipulation Tools

#### json_transform

**Description:** Transforms JSON data with operations: uppercase_keys, lowercase_values, filter

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "operation": {
      "type": "string",
      "enum": ["uppercase_keys", "lowercase_values", "filter"]
    },
    "data": { "type": "object" },
    "filter_key": { "type": "string" }
  },
  "required": ["operation", "data"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "json_transform",
  "arguments": {
    "operation": "uppercase_keys",
    "data": { "name": "Alice", "age": 30 }
  }
}
```

**Returns:** `{ "NAME": "Alice", "AGE": 30 }`

---

#### text_processor

**Description:** Processes text with operations: uppercase, lowercase, reverse

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "text": { "type": "string" },
    "operation": {
      "type": "string",
      "enum": ["uppercase", "lowercase", "reverse"]
    }
  },
  "required": ["text", "operation"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "text_processor",
  "arguments": {
    "text": "Hello World",
    "operation": "uppercase"
  }
}
```

**Returns:** `"HELLO WORLD"`

---

#### list_operations

**Description:** Performs operations on number lists: sum, avg, max, sort, filter

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "list": {
      "type": "array",
      "items": { "type": "number" }
    },
    "operation": {
      "type": "string",
      "enum": ["sum", "avg", "max", "sort", "filter"]
    },
    "filter_value": { "type": "number" }
  },
  "required": ["list", "operation"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "list_operations",
  "arguments": {
    "list": [5, 2, 8, 1, 9],
    "operation": "sort"
  }
}
```

**Returns:** `[1, 2, 5, 8, 9]`

---

### Validation & Computation Tools

#### validate_email

**Description:** Validates email address format using regex pattern matching

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "email": { "type": "string" }
  },
  "required": ["email"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "validate_email",
  "arguments": { "email": "test@example.com" }
}
```

**Returns:** `{ "email": "test@example.com", "valid": true, "message": "Email format is valid" }`

---

#### calculate

**Description:** Evaluates mathematical expressions safely (supports: +, -, *, /, parentheses)

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "expression": { "type": "string" }
  },
  "required": ["expression"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "calculate",
  "arguments": { "expression": "2 + 2 * 3" }
}
```

**Returns:** `"8"` (follows order of operations)

---

#### hash_generator

**Description:** Generates hash of data using specified algorithm: md5, sha256

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "data": { "type": "string" },
    "algorithm": {
      "type": "string",
      "enum": ["md5", "sha256"]
    }
  },
  "required": ["data", "algorithm"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "hash_generator",
  "arguments": {
    "data": "Hello, world!",
    "algorithm": "sha256"
  }
}
```

**Returns:** `{ "algorithm": "sha256", "hash": "315f5bdb76d078c43b8ac0064e4a0164612b1fce77c869345bfc94c75894edd3", "input_len": 13 }`

---

### Simulated API Tools

#### weather_api

**Description:** Returns mock weather data for a city

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "city": { "type": "string" },
    "units": {
      "type": "string",
      "enum": ["celsius", "fahrenheit"],
      "default": "celsius"
    }
  },
  "required": ["city"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "weather_api",
  "arguments": {
    "city": "San Francisco",
    "units": "fahrenheit"
  }
}
```

**Returns:** `{ "city": "San Francisco", "temperature": 68.5, "units": "fahrenheit", "condition": "sunny", "humidity": 65, "wind_speed": 12 }`

---

#### geocode

**Description:** Returns mock latitude/longitude for an address

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "address": { "type": "string" }
  },
  "required": ["address"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "geocode",
  "arguments": { "address": "1600 Amphitheatre Parkway, Mountain View, CA" }
}
```

**Returns:** `{ "address": "1600 Amphitheatre Parkway, Mountain View, CA", "latitude": 37.422408, "longitude": -122.084068, "accuracy": "approximate" }`

---

#### currency_convert

**Description:** Converts currency with fixed mock rates

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "amount": { "type": "number" },
    "from": { "type": "string" },
    "to": { "type": "string" }
  },
  "required": ["amount", "from", "to"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "currency_convert",
  "arguments": {
    "amount": 100,
    "from": "USD",
    "to": "EUR"
  }
}
```

**Returns:** `{ "amount": 100, "from": "USD", "to": "EUR", "converted": 90.0 }`

**Supported currency pairs:** USD↔EUR, USD↔GBP (limited fixed rates)

---

### File Operation Tools (Simulated)

#### read_file

**Description:** Simulates reading file content (returns mock data)

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "path": { "type": "string" }
  },
  "required": ["path"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "read_file",
  "arguments": { "path": "/etc/config.json" }
}
```

**Returns:** `{ "path": "/etc/config.json", "content": "{\"version\": \"1.0\", \"debug\": false}", "size": 32 }`

---

#### write_file

**Description:** Simulates writing content to a file

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "path": { "type": "string" },
    "content": { "type": "string" }
  },
  "required": ["path", "content"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "write_file",
  "arguments": {
    "path": "/tmp/output.txt",
    "content": "Hello, file!"
  }
}
```

**Returns:** `{ "path": "/tmp/output.txt", "bytes_written": 12, "success": true, "message": "File write simulated successfully" }`

---

#### list_directory

**Description:** Returns mock directory listing

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "path": { "type": "string" }
  },
  "required": ["path"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "list_directory",
  "arguments": { "path": "/home/user" }
}
```

**Returns:** `{ "path": "/home/user", "entries": [{"name": "item_1.txt", "type": "file", "size": 1234, "modified": "2026-01-29T10:30:00Z"}], "count": 5 }`

---

### Testing Utility Tools

#### large_payload

**Description:** Generates a large JSON payload for testing (1KB - 1MB)

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "size_kb": {
      "type": "number",
      "minimum": 1,
      "maximum": 1024
    }
  },
  "required": ["size_kb"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "large_payload",
  "arguments": { "size_kb": 100 }
}
```

**Returns:** JSON object with ~100KB of data (array of items with UUIDs, timestamps, nested objects)

---

#### random_latency

**Description:** Responds with random latency between min and max milliseconds

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "min_ms": { "type": "number", "minimum": 0 },
    "max_ms": { "type": "number", "minimum": 1 }
  },
  "required": ["min_ms", "max_ms"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "random_latency",
  "arguments": {
    "min_ms": 100,
    "max_ms": 500
  }
}
```

**Returns:** `{ "min_ms": 100, "max_ms": 500, "actual_ms": 327, "message": "Response delayed successfully" }`

---

#### conditional_error

**Description:** Randomly fails based on error probability (0.0 to 1.0)

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "error_probability": {
      "type": "number",
      "minimum": 0,
      "maximum": 1
    }
  },
  "required": ["error_probability"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "conditional_error",
  "arguments": { "error_probability": 0.3 }
}
```

**Returns:** 30% chance of error, 70% chance of success with `{ "error_probability": 0.3, "triggered": false, "message": "No error this time" }`

---

### Advanced Testing Tools

#### degrading_performance

**Description:** Simulates performance degradation over time - latency increases with each call

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "base_latency_ms": { "type": "integer", "minimum": 0 },
    "increment_ms": { "type": "integer", "minimum": 0 }
  },
  "required": ["base_latency_ms"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "degrading_performance",
  "arguments": { "base_latency_ms": 100, "increment_ms": 50 }
}
```

---

#### flaky_connection

**Description:** Simulates intermittent connection failures with configurable failure rate

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "failure_rate": { "type": "number", "minimum": 0, "maximum": 1 }
  },
  "required": ["failure_rate"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "flaky_connection",
  "arguments": { "failure_rate": 0.2 }
}
```

---

#### rate_limited

**Description:** Simulates rate limiting behavior - returns 429 after threshold

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "requests_per_minute": { "type": "integer", "minimum": 1 }
  },
  "required": ["requests_per_minute"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "rate_limited",
  "arguments": { "requests_per_minute": 60 }
}
```

---

#### circuit_breaker

**Description:** Simulates circuit breaker pattern - opens after consecutive failures

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "failure_threshold": { "type": "integer", "minimum": 1 },
    "reset_timeout_ms": { "type": "integer", "minimum": 0 }
  },
  "required": ["failure_threshold"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "circuit_breaker",
  "arguments": { "failure_threshold": 5, "reset_timeout_ms": 30000 }
}
```

---

#### backpressure

**Description:** Simulates backpressure scenarios - slows down under load

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "queue_depth": { "type": "integer", "minimum": 0 },
    "max_queue": { "type": "integer", "minimum": 1 }
  },
  "required": ["queue_depth", "max_queue"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "backpressure",
  "arguments": { "queue_depth": 10, "max_queue": 100 }
}
```

---

#### stateful_counter

**Description:** Maintains state across calls - useful for testing stateful operations

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "operation": { "type": "string", "enum": ["increment", "decrement", "reset", "get"] },
    "amount": { "type": "integer" }
  },
  "required": ["operation"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "stateful_counter",
  "arguments": { "operation": "increment", "amount": 5 }
}
```

---

#### realistic_latency

**Description:** Simulates realistic latency distribution (normal distribution)

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "mean_ms": { "type": "integer", "minimum": 0 },
    "stddev_ms": { "type": "integer", "minimum": 0 }
  },
  "required": ["mean_ms", "stddev_ms"]
}
```

**Example Usage:**
```json
{
  "operation": "tools/call",
  "tool_name": "realistic_latency",
  "arguments": { "mean_ms": 200, "stddev_ms": 50 }
}
```

---

## Configuring Tool Calls

### Basic Tool Call Configuration

Add tool calls to your test configuration's `workload.op_mix` array:

```json
{
  "workload": {
    "op_mix": [
      {
        "operation": "tools/call",
        "weight": 5,
        "tool_name": "validate_email",
        "arguments": {
          "email": "test@example.com"
        }
      }
    ]
  }
}
```

### Operation Mix Weights

The `weight` field determines the relative frequency of each operation:

```json
{
  "op_mix": [
    { "operation": "tools/list", "weight": 1 },
    { "operation": "tools/call", "weight": 5, "tool_name": "fast_echo", "arguments": {"message": "test"} },
    { "operation": "tools/call", "weight": 3, "tool_name": "calculate", "arguments": {"expression": "2+2"} }
  ]
}
```

In this example:
- `tools/list` runs 1 out of every 9 operations (11%)
- `fast_echo` runs 5 out of every 9 operations (56%)
- `calculate` runs 3 out of every 9 operations (33%)

### Multiple Tools in One Test

Test multiple tools simultaneously by adding multiple `tools/call` operations:

```json
{
  "op_mix": [
    { "operation": "tools/call", "weight": 2, "tool_name": "validate_email", "arguments": {"email": "user@example.com"} },
    { "operation": "tools/call", "weight": 2, "tool_name": "hash_generator", "arguments": {"data": "test", "algorithm": "sha256"} },
    { "operation": "tools/call", "weight": 1, "tool_name": "weather_api", "arguments": {"city": "London", "units": "celsius"} }
  ]
}
```

---

## Argument Schema Guide

### JSON Schema Types

MCP Drill validates tool arguments against JSON schemas. Supported types:

| Type | Description | Example |
|------|-------------|---------|
| `string` | Text value | `"hello"` |
| `number` | Numeric value (integer or float) | `42`, `3.14` |
| `integer` | Whole number | `42` |
| `boolean` | True or false | `true`, `false` |
| `object` | Key-value pairs | `{"name": "Alice"}` |
| `array` | Ordered list | `[1, 2, 3]` |
| `null` | Null value | `null` |

### String Validation

```json
{
  "type": "string",
  "minLength": 3,
  "maxLength": 50,
  "pattern": "^[a-zA-Z]+$"
}
```

**Constraints:**
- `minLength` - Minimum string length
- `maxLength` - Maximum string length
- `pattern` - Regular expression pattern (must match entire string)

**Example:**
```json
{
  "tool_name": "text_processor",
  "arguments": {
    "text": "Hello",
    "operation": "uppercase"
  }
}
```

### Number Validation

```json
{
  "type": "number",
  "minimum": 0,
  "maximum": 100
}
```

**Constraints:**
- `minimum` - Minimum value (inclusive)
- `maximum` - Maximum value (inclusive)
- `type: "integer"` - Restrict to whole numbers

**Example:**
```json
{
  "tool_name": "random_latency",
  "arguments": {
    "min_ms": 100,
    "max_ms": 500
  }
}
```

### Array Validation

```json
{
  "type": "array",
  "items": { "type": "number" },
  "minItems": 1,
  "maxItems": 100
}
```

**Constraints:**
- `items` - Schema for array elements
- `minItems` - Minimum array length
- `maxItems` - Maximum array length

**Example:**
```json
{
  "tool_name": "list_operations",
  "arguments": {
    "list": [5, 2, 8, 1, 9],
    "operation": "sort"
  }
}
```

### Object Validation

```json
{
  "type": "object",
  "properties": {
    "name": { "type": "string" },
    "age": { "type": "integer", "minimum": 0 }
  },
  "required": ["name"]
}
```

**Constraints:**
- `properties` - Schema for each property
- `required` - Array of required property names

**Example:**
```json
{
  "tool_name": "json_transform",
  "arguments": {
    "operation": "uppercase_keys",
    "data": { "name": "Alice", "age": 30 }
  }
}
```

### Nested Objects

Schemas can be nested to any depth:

```json
{
  "type": "object",
  "properties": {
    "user": {
      "type": "object",
      "properties": {
        "profile": {
          "type": "object",
          "properties": {
            "email": { "type": "string" }
          }
        }
      }
    }
  }
}
```

### Enum Validation

Restrict values to a specific set:

```json
{
  "type": "string",
  "enum": ["celsius", "fahrenheit"]
}
```

**Example:**
```json
{
  "tool_name": "weather_api",
  "arguments": {
    "city": "Paris",
    "units": "celsius"
  }
}
```

---

## Interpreting Tool Metrics

### Per-Tool Metrics

After running a test, query tool metrics via the API:

```bash
curl http://localhost:8080/runs/run_0000000000000001/metrics
```

**Response:**
```json
{
  "by_tool": {
    "validate_email": {
      "total_ops": 1000,
      "success_ops": 980,
      "failure_ops": 20,
      "latency_p50": 95,
      "latency_p95": 125,
      "latency_p99": 150,
      "error_rate": 0.02
    }
  }
}
```

### Metric Definitions

| Metric | Description |
|--------|-------------|
| `total_ops` | Total number of tool invocations |
| `success_ops` | Number of successful executions |
| `failure_ops` | Number of failed executions |
| `latency_p50` | 50th percentile latency in milliseconds |
| `latency_p95` | 95th percentile latency in milliseconds |
| `latency_p99` | 99th percentile latency in milliseconds |
| `error_rate` | Ratio of failures to total (0-1) |

### Success Rate Calculation

```
Success Rate = (success_ops / total_ops) * 100
```

Example: 980 successes out of 1000 calls = 98% success rate

### Latency Percentiles

- **P50 (median)**: Half of all calls are faster than this
- **P95**: 95% of calls are faster than this (good for SLA targets)
- **P99**: 99% of calls are faster than this (captures worst-case scenarios)

**Interpreting latency:**
- P95 < 200ms: Excellent performance
- P95 200-500ms: Good performance
- P95 500-1000ms: Acceptable for non-critical operations
- P95 > 1000ms: Investigate performance issues

### Payload Size Tracking

`avg_payload_size` = average of (argument_size + result_size) across all calls

**Use cases:**
- Identify tools with large payloads
- Optimize argument structures
- Plan network capacity

---

## Troubleshooting Tool Failures

### Common Errors

#### 1. Unknown Tool

**Error:** `"unknown tool: my_tool"`

**Cause:** Tool name doesn't exist on the server

**Solution:**
1. Verify tool name spelling
2. Check available tools: `curl -X POST http://localhost:3000/mcp -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'`
3. Ensure mock server is running with correct tools

---

#### 2. Argument Validation Failure

**Error:** `"validation error: field 'email' is required"`

**Cause:** Missing required argument or invalid argument type

**Solution:**
1. Check tool's input schema
2. Verify all required fields are present
3. Ensure argument types match schema (string vs number, etc.)

**Example fix:**
```json
// ❌ Missing required field
{
  "tool_name": "validate_email",
  "arguments": {}
}

// ✅ Correct
{
  "tool_name": "validate_email",
  "arguments": { "email": "test@example.com" }
}
```

---

#### 3. Type Mismatch

**Error:** `"validation error: expected number, got string"`

**Cause:** Argument type doesn't match schema

**Solution:**
1. Check schema type requirements
2. Convert values to correct type (no automatic coercion)

**Example fix:**
```json
// ❌ String instead of number
{
  "tool_name": "random_latency",
  "arguments": { "min_ms": "100", "max_ms": "500" }
}

// ✅ Correct
{
  "tool_name": "random_latency",
  "arguments": { "min_ms": 100, "max_ms": 500 }
}
```

---

#### 4. Timeout

**Error:** `"context deadline exceeded"`

**Cause:** Tool execution exceeded timeout

**Solution:**
1. Increase timeout in target config: `"timeout_ms": 60000`
2. Check if tool is intentionally slow (e.g., `timeout_tool`)
3. Verify network connectivity

---

#### 5. High Error Rate

**Symptom:** `error_count` is high in metrics

**Debugging steps:**
1. Check operation logs for error details:
   ```bash
   curl http://localhost:8080/runs/run_0000000000000001/logs?operation=tools/call&error=true
   ```
2. Look for patterns in error messages
3. Verify argument schemas are correct
4. Check if using `conditional_error` tool with high probability

---

#### 6. Slow Performance

**Symptom:** High P95/P99 latency

**Debugging steps:**
1. Check if using `slow_echo` or `random_latency` tools
2. Verify mock server isn't overloaded (check CPU/memory)
3. Reduce concurrent VUs if server is saturated
4. Check network latency between worker and mock server

---

### Debugging Tips

**1. Enable verbose logging:**
```bash
./mcpdrill-worker --control-plane http://localhost:8080 --debug
```

**2. Test tool individually:**
```bash
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "validate_email",
      "arguments": {"email": "test@example.com"}
    }
  }'
```

**3. Check tool schema:**
```bash
curl -X POST http://localhost:3000/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' | jq '.result.tools[] | select(.name == "validate_email")'
```

**4. Review operation logs:**
```bash
curl http://localhost:8080/runs/run_0000000000000001/logs?limit=100 | jq '.logs[] | select(.operation == "tools/call")'
```

**5. Compare metrics across tools:**
```bash
curl http://localhost:8080/runs/run_0000000000000001/metrics | jq '.by_tool'
```

---

## Next Steps

- **[Adding Custom Tools](adding-custom-tools.md)** - Learn how to create your own mock tools
- **[Example Configurations](../examples/tool-testing/)** - Ready-to-use test configurations
- **[API Reference](../README.md#api-reference)** - Complete API documentation
- **[Web UI Guide](../README.md#web-ui)** - Use the visual tool testing interface
