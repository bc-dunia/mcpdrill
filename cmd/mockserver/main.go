// Package main provides the mcpdrill-mockserver CLI binary.
// This starts a mock MCP server with 27 built-in tools for testing.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/mockserver"
)

func main() {
	addr := flag.String("addr", ":3000", "HTTP server address")
	flag.Parse()

	config := mockserver.DefaultConfig()
	config.Addr = *addr

	server := mockserver.New(config)

	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting mock server: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Mock MCP server listening on %s\n", server.Addr())
	fmt.Printf("MCP endpoint: %s\n", server.MCPURL())
	fmt.Println("Press Ctrl+C to stop")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Stop(ctx)
	fmt.Println("Mock server stopped")
}
