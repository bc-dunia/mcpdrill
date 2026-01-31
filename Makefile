.PHONY: all build clean server worker mockserver agent help

BINARY_DIR := .
GO_BUILD := go build

all: build

build: server worker mockserver agent

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
