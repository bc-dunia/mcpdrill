# Development Guide

Guide for contributing to MCP Drill.

## Prerequisites

- Go 1.22+
- Node.js 18+ (for Web UI)
- Git

## Project Structure

```
mcpdrill/
├── cmd/                        # Application entry points
│   ├── mcpdrill/              # CLI tool
│   ├── server/                # Control plane server
│   ├── worker/                # Worker node
│   ├── agent/                 # Server telemetry agent
│   └── mockserver/            # Mock MCP server
├── internal/                   # Internal packages
│   ├── controlplane/          # Control plane internals
│   │   ├── api/               # HTTP API, SSE
│   │   ├── runmanager/        # Run lifecycle
│   │   ├── scheduler/         # Worker registry
│   │   └── stopconditions/    # Stop condition evaluation
│   ├── worker/                # Worker runtime
│   ├── vu/                    # VU engine
│   ├── session/               # Session management
│   ├── transport/             # MCP transport
│   ├── validation/            # Config validation
│   ├── analysis/              # Metrics aggregation
│   └── mockserver/            # Mock server tools
├── web/log-explorer/          # Web UI (React)
├── examples/                  # Example configurations
├── schemas/                   # JSON schemas
└── docs/                      # Documentation
```

## Building

```bash
# Build all binaries
go build -o mcpdrill ./cmd/mcpdrill
go build -o mcpdrill-server ./cmd/server
go build -o mcpdrill-worker ./cmd/worker
go build -o mcpdrill-agent ./cmd/agent
go build -o mcpdrill-mockserver ./cmd/mockserver

# Build with race detector (development)
go build -race -o mcpdrill ./cmd/mcpdrill

# Build optimized (production)
go build -ldflags="-s -w" -o mcpdrill ./cmd/mcpdrill
```

## Testing

```bash
# Unit tests
go test ./...

# With coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run specific package tests
go test ./internal/vu/...

# E2E tests
go test ./test/e2e/...
```

## Web UI Development

```bash
cd web/log-explorer

# Install dependencies
npm install

# Development server (hot reload)
npm run dev

# Build for production
npm run build

# Type check
npm run typecheck
```

## Code Style

- Follow Go best practices and conventions
- Use `gofmt` for formatting
- Run `go vet` before committing
- Keep functions focused and small
- Add tests for new functionality

## Contributing

1. **Fork** the repository
2. **Create** a feature branch (`git checkout -b feature/amazing-feature`)
3. **Make** your changes with tests
4. **Commit** with clear messages (`git commit -m 'Add amazing feature'`)
5. **Push** to the branch (`git push origin feature/amazing-feature`)
6. **Open** a Pull Request

## Pull Request Guidelines

- Keep PRs focused on a single change
- Include tests for new functionality
- Update documentation as needed
- Ensure all tests pass
- Follow existing code style

## Specifications

| Metric | Value |
|--------|-------|
| Max VUs per worker | 10,000 |
| Telemetry batch size | 1,000 records |
| Heartbeat interval | 10 seconds |
| Default timeout | 30 seconds |
| Supported transports | StreamableHTTP |
| Session modes | 4 |
| Stop condition metrics | 6 |
| Validation rules | 18 semantic + 12 SSRF |

## Resource Guidelines

| Component | VUs | CPU | Memory |
|-----------|-----|-----|--------|
| Control Plane | Any | 1 core | 512 MB |
| Worker | 100 | 0.5 core | 256 MB |
| Worker | 1,000 | 1 core | 512 MB |
| Worker | 5,000 | 2 cores | 1 GB |
| Worker | 10,000 | 4 cores | 2 GB |
