.PHONY: all build clean server worker mockserver agent help dev dev-stop dev-logs

BINARY_DIR := .
GO_BUILD := go build

all: build

build: server worker mockserver agent

dev: server worker mockserver
	@echo "Starting MCP Drill development environment..."
	@mkdir -p .dev-logs
	@if [ -f .dev-logs/mockserver.pid ]; then kill $$(cat .dev-logs/mockserver.pid) 2>/dev/null || true; fi
	@if [ -f .dev-logs/server.pid ]; then kill $$(cat .dev-logs/server.pid) 2>/dev/null || true; fi
	@if [ -f .dev-logs/worker.pid ]; then kill $$(cat .dev-logs/worker.pid) 2>/dev/null || true; fi
	@sleep 1
	@./mcpdrill-mockserver --addr 127.0.0.1:3000 > .dev-logs/mockserver.log 2>&1 & echo $$! > .dev-logs/mockserver.pid
	@./mcpdrill-server --dev > .dev-logs/server.log 2>&1 & echo $$! > .dev-logs/server.pid
	@sleep 2
	@./mcpdrill-worker --control-plane http://localhost:8080 --allow-private-networks '127.0.0.0/8,::1/128' > .dev-logs/worker.log 2>&1 & echo $$! > .dev-logs/worker.pid
	@echo ""
	@echo "✓ All services started (loopback only, auth disabled)"
	@echo ""
	@echo "  Mock Server:    http://localhost:3000/mcp"
	@echo "  Control Plane:  http://localhost:8080"
	@echo "  Web UI:         cd web/log-explorer && npm run dev"
	@echo ""
	@echo "  Logs:           .dev-logs/"
	@echo "  Stop:           make dev-stop"
	@echo ""

dev-stop:
	@echo "Stopping development services..."
	@if [ -f .dev-logs/mockserver.pid ]; then kill $$(cat .dev-logs/mockserver.pid) 2>/dev/null || true; rm -f .dev-logs/mockserver.pid; fi
	@if [ -f .dev-logs/server.pid ]; then kill $$(cat .dev-logs/server.pid) 2>/dev/null || true; rm -f .dev-logs/server.pid; fi
	@if [ -f .dev-logs/worker.pid ]; then kill $$(cat .dev-logs/worker.pid) 2>/dev/null || true; rm -f .dev-logs/worker.pid; fi
	@echo "✓ All services stopped"

dev-logs:
	@tail -f .dev-logs/*.log 2>/dev/null || echo "No logs yet. Run 'make dev' first."

server:
	$(GO_BUILD) -o $(BINARY_DIR)/mcpdrill-server ./cmd/server

worker:
	$(GO_BUILD) -o $(BINARY_DIR)/mcpdrill-worker ./cmd/worker

mockserver:
	$(GO_BUILD) -o $(BINARY_DIR)/mcpdrill-mockserver ./cmd/mockserver

agent:
	$(GO_BUILD) -o $(BINARY_DIR)/mcpdrill-agent ./cmd/agent

clean:
	rm -f $(BINARY_DIR)/mcpdrill-server
	rm -f $(BINARY_DIR)/mcpdrill-worker
	rm -f $(BINARY_DIR)/mcpdrill-mockserver
	rm -f $(BINARY_DIR)/mcpdrill-agent

test:
	go test ./...

help:
	@echo "Usage:"
	@echo "  make build      - Build all binaries"
	@echo "  make server     - Build mcpdrill-server"
	@echo "  make worker     - Build mcpdrill-worker"
	@echo "  make mockserver - Build mcpdrill-mockserver"
	@echo "  make agent      - Build mcpdrill-agent"
	@echo "  make clean      - Remove all binaries"
	@echo "  make test       - Run tests"
