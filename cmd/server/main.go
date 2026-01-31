package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/auth"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/api"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/runmanager"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/scheduler"
	"github.com/bc-dunia/mcpdrill/internal/metrics"
	"github.com/bc-dunia/mcpdrill/internal/validation"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP server address")
	authMode := flag.String("auth-mode", "none", "Authentication mode: none, api_key, jwt")
	apiKeys := flag.String("api-keys", "", "Comma-separated API keys (for api_key mode)")
	jwtSecret := flag.String("jwt-secret", "", "JWT secret (for jwt mode)")
	enableAgentIngest := flag.Bool("enable-agent-ingest", false, "Enable agent telemetry ingestion endpoints")
	agentTokens := flag.String("agent-tokens", "", "Comma-separated tokens for agent authentication")
	flag.Parse()

	validator, err := validation.NewUnifiedValidator(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating validator: %v\n", err)
		os.Exit(1)
	}

	rm := runmanager.NewRunManager(validator)

	registry := scheduler.NewRegistry()
	leaseManager := scheduler.NewLeaseManager(60000)
	allocator := scheduler.NewAllocator(registry, leaseManager)
	rm.SetScheduler(registry, allocator, leaseManager)

	server := api.NewServer(*addr, rm)
	server.SetRegistry(registry)
	server.SetTelemetryStore(api.NewTelemetryStore())
	server.SetMetricsCollector(metrics.NewCollector())

	authConfig := &auth.Config{
		Mode:      auth.AuthMode(*authMode),
		SkipPaths: []string{"/healthz", "/readyz"},
	}
	if *apiKeys != "" {
		authConfig.APIKeys = strings.Split(*apiKeys, ",")
	}
	if *jwtSecret != "" {
		authConfig.JWTSecret = []byte(*jwtSecret)
	}
	server.SetAuthConfig(authConfig)

	if *enableAgentIngest {
		server.SetAgentStore(api.NewAgentStore())
		if *agentTokens != "" {
			tokens := strings.Split(*agentTokens, ",")
			server.SetAgentAuthConfig(&api.AgentAuthConfig{
				Enabled: true,
				Tokens:  tokens,
			})
		}
	}

	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting server: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("MCP Drill control plane listening on %s\n", server.URL())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error during shutdown: %v\n", err)
	}

	registry.Close()
	fmt.Println("Server stopped")
}
