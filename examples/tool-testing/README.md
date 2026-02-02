# Tool Testing Examples

These example configurations are **simplified references** that demonstrate tool configuration patterns. They are not directly usable with the API as-is.

## Using These Examples

**Option 1: Use the Web UI (Recommended)**

The Web UI wizard at http://localhost:5173 generates fully valid configurations:
1. Start the backend: `make dev`
2. Start the frontend: `cd web/log-explorer && npm run dev`
3. Click "New Run" and configure your test visually

**Option 2: Start from quick-start.json**

Copy `examples/quick-start.json` as a base and modify:
- It contains all required fields for `schema_version: "run-config/v1"`
- Update `workload.tools.templates` for your tool selection
- Adjust stages, safety caps, etc. as needed

## Example Patterns

| File | Demonstrates |
|------|-------------|
| `simple-tool-test.json` | Basic tool call configuration |
| `mixed-tools.json` | Multiple tools with weights |
| `complex-arguments.json` | Tools with nested arguments |
| `error-testing.json` | Error handling tools |
| `performance-test.json` | High-throughput configuration |

## Schema Requirements

The `run-config/v1` schema requires:
- `schema_version: "run-config/v1"`
- `workload.operation_mix` (not `op_mix`)
- Underscore operation names (`tools_call`, not `tools/call`)
- Tool definitions in `workload.tools.templates`
- Complete `stages`, `safety`, `session_policy`, etc.

See `examples/quick-start.json` for a complete valid example.
