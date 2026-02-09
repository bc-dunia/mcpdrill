.PHONY: all build clean server worker mockserver agent help dev dev-stop dev-logs \
	frontend docker-build docker-push docker-server docker-worker docker-mockserver docker-agent

BINARY_DIR := .
GO_BUILD := go build
VERSION ?= latest
DOCKER_REGISTRY ?= mcpdrill

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

frontend:
	cd web/log-explorer && npm ci --no-audit --no-fund && npm run build
	rm -rf internal/web/dist/assets
	cp -r web/log-explorer/dist/* internal/web/dist/

docker-server:
	docker build -f docker/Dockerfile.server -t $(DOCKER_REGISTRY)/server:$(VERSION) .

docker-worker:
	docker build -f docker/Dockerfile.worker -t $(DOCKER_REGISTRY)/worker:$(VERSION) .

docker-mockserver:
	docker build -f docker/Dockerfile.mockserver -t $(DOCKER_REGISTRY)/mockserver:$(VERSION) .

docker-agent:
	docker build -f docker/Dockerfile.agent -t $(DOCKER_REGISTRY)/agent:$(VERSION) .

docker-build: docker-server docker-worker docker-mockserver docker-agent

docker-push:
	docker push $(DOCKER_REGISTRY)/server:$(VERSION)
	docker push $(DOCKER_REGISTRY)/worker:$(VERSION)
	docker push $(DOCKER_REGISTRY)/mockserver:$(VERSION)
	docker push $(DOCKER_REGISTRY)/agent:$(VERSION)

help:
	@echo "Usage:"
	@echo "  make build            - Build all binaries"
	@echo "  make server           - Build mcpdrill-server"
	@echo "  make worker           - Build mcpdrill-worker"
	@echo "  make mockserver       - Build mcpdrill-mockserver"
	@echo "  make agent            - Build mcpdrill-agent"
	@echo "  make frontend         - Build frontend and copy to embed dir"
	@echo "  make clean            - Remove all binaries"
	@echo "  make test             - Run tests"
	@echo ""
	@echo "Docker:"
	@echo "  make docker-build     - Build all Docker images"
	@echo "  make docker-server    - Build server image"
	@echo "  make docker-worker    - Build worker image"
	@echo "  make docker-mockserver - Build mockserver image"
	@echo "  make docker-agent     - Build agent image"
	@echo "  make docker-push      - Push all images"
	@echo ""
	@echo "Variables:"
	@echo "  VERSION=v0.1.0        - Image tag (default: latest)"
	@echo "  DOCKER_REGISTRY=ghcr.io/bc-dunia/mcpdrill - Registry prefix"
