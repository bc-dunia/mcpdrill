# Plugin System

MCP Drill supports custom operations through an in-tree plugin architecture.

> **Note:** External Go plugins are not supported due to `internal/` package usage. Plugins must be built into the binary.

## Creating a Custom Operation

### 1. Fork or Clone the Repository

```bash
git clone https://github.com/bc-dunia/mcpdrill.git
cd mcpdrill
```

### 2. Create Your Operation

Create a file in `internal/plugin/`:

```go
// internal/plugin/custom_myop.go
package plugin

import (
    "context"
    "github.com/bc-dunia/mcpdrill/internal/transport"
)

func init() {
    MustRegister(&CustomOperation{})
}

type CustomOperation struct{}

func (o *CustomOperation) Name() string {
    return "custom/myop"
}

func (o *CustomOperation) Execute(ctx context.Context, conn transport.Connection, params map[string]interface{}) (*transport.OperationOutcome, error) {
    // Your custom logic here
    return &transport.OperationOutcome{OK: true}, nil
}

func (o *CustomOperation) Validate(params map[string]interface{}) error {
    // Validate parameters
    return nil
}
```

### 3. Rebuild the Worker

```bash
go build -o mcpdrill-worker ./cmd/worker
```

### 4. Use in Configuration

```json
{
  "workload": {
    "op_mix": [
      { "operation": "custom/myop", "weight": 1, "arguments": {"key": "value"} }
    ]
  }
}
```

## Operation Interface

```go
type Operation interface {
    Name() string
    Execute(ctx context.Context, conn transport.Connection, params map[string]interface{}) (*transport.OperationOutcome, error)
    Validate(params map[string]interface{}) error
}
```

## Built-in Operations

| Operation | Description |
|-----------|-------------|
| `tools/list` | List available tools from server |
| `tools/call` | Call a specific tool |
| `ping` | Simple connectivity check |

## See Also

- [Adding Custom Tools](adding-custom-tools.md) - For mock server tools
