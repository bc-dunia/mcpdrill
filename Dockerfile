# MCP Drill - Multi-stage Docker build
# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git make

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binaries
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /build/bin/mcpdrill ./cmd/mcpdrill
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /build/bin/mcpdrill-server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /build/bin/mcpdrill-worker ./cmd/worker

# Runtime stage
FROM alpine:latest

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy binaries from builder
COPY --from=builder /build/bin/* /usr/local/bin/

# Copy web UI (if built)
COPY --from=builder /build/web/log-explorer/dist /app/web/log-explorer/dist

# Create non-root user
RUN addgroup -g 1000 mcpdrill && \
    adduser -D -u 1000 -G mcpdrill mcpdrill && \
    chown -R mcpdrill:mcpdrill /app

USER mcpdrill

# Expose ports
EXPOSE 8080

# Default command (server)
CMD ["mcpdrill-server", "--addr", ":8080"]
