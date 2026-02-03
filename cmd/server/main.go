package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
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
	authMode := flag.String("auth-mode", "api_key", "Authentication mode: none, api_key, jwt")
	apiKeys := flag.String("api-keys", "", "Comma-separated API keys (for api_key mode)")
	jwtSecret := flag.String("jwt-secret", "", "JWT secret (for jwt mode)")
	insecure := flag.Bool("insecure", false, "Allow unauthenticated mode (only safe on loopback)")
	enableAgentIngest := flag.Bool("enable-agent-ingest", false, "Enable agent telemetry ingestion endpoints")
	agentTokens := flag.String("agent-tokens", "", "Comma-separated tokens for agent authentication")
	allowPrivateNetworks := flag.String("allow-private-networks", "", "Comma-separated CIDR ranges to allow (e.g., '127.0.0.0/8,10.0.0.0/8' for local testing)")
	allowPrivateDiscovery := flag.Bool("allow-private-discovery", false, "Allow discovery endpoints to access private networks")
	insecureWorkerAuth := flag.Bool("insecure-worker-auth", false, "Disable worker token authentication (not recommended)")
	redactAssignmentSecrets := flag.Bool("redact-assignment-secrets", false, "Redact sensitive headers and tokens in worker assignments")
	rateLimit := flag.Float64("rate-limit", 100, "API rate limit in requests/second (0 to disable)")
	rateBurst := flag.Int("rate-burst", 200, "API rate limit burst size")
	maxOpsPerRun := flag.Int("max-ops-per-run", 20000000, "Max operations stored per run (0=unlimited)")
	maxLogsPerRun := flag.Int("max-logs-per-run", 20000000, "Max logs stored per run (0=unlimited)")
	maxTotalRuns := flag.Int("max-total-runs", 100, "Max runs in memory before eviction (0=unlimited)")
	devMode := flag.Bool("dev", false, "Development mode: binds to loopback, disables auth, allows private networks")
	flag.Parse()

	if *maxOpsPerRun < 0 || *maxLogsPerRun < 0 || *maxTotalRuns < 0 {
		slog.Error("telemetry limits cannot be negative")
		os.Exit(1)
	}
	if *maxOpsPerRun == 0 || *maxLogsPerRun == 0 {
		slog.Warn("unlimited telemetry storage enabled, monitor memory usage to avoid OOM")
	}
	if *maxOpsPerRun > 50000000 || *maxLogsPerRun > 50000000 {
		slog.Warn("telemetry limits exceed recommended maximum (50M), very high memory usage expected (25GB+)")
	}

	if *devMode {
		*addr = "127.0.0.1:8080"
		*insecure = true
		*insecureWorkerAuth = true
		*allowPrivateDiscovery = true
		*allowPrivateNetworks = "127.0.0.0/8,::1/128,fe80::/10,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,fc00::/7,169.254.0.0/16"
		*enableAgentIngest = true
		*agentTokens = "dev-token"
		*rateLimit = 0 // Disable rate limiting in dev mode
		fmt.Println("")
		fmt.Println("╔════════════════════════════════════════════════════════════╗")
		fmt.Println("║  DEVELOPMENT MODE - DO NOT USE IN PRODUCTION               ║")
		fmt.Println("║  Auth disabled, rate limiting disabled, private nets OK    ║")
		fmt.Println("║  Bound to loopback only (127.0.0.1:8080)                   ║")
		fmt.Println("╚════════════════════════════════════════════════════════════╝")
		fmt.Println("")
	}

	// Build system policy with optional private network allowlist
	systemPolicy := validation.DefaultSystemPolicy()
	if *allowPrivateNetworks != "" {
		cidrs := strings.Split(*allowPrivateNetworks, ",")
		for i, cidr := range cidrs {
			cidrs[i] = strings.TrimSpace(cidr)
		}
		systemPolicy.AllowPrivateNetworks = cidrs
	}

	validator, err := validation.NewUnifiedValidator(systemPolicy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating validator: %v\n", err)
		os.Exit(1)
	}

	rm := runmanager.NewRunManager(validator)

	registry := scheduler.NewRegistry()
	leaseManager := scheduler.NewLeaseManager(60000)
	allocator := scheduler.NewAllocator(registry, leaseManager)
	rm.SetScheduler(registry, allocator, leaseManager)

	heartbeatMonitor := scheduler.NewHeartbeatMonitor(registry, leaseManager, 0, 0)
	heartbeatMonitor.SetOnWorkerLost(func(workerID scheduler.WorkerID, affectedRunIDs []string) {
		for _, runID := range affectedRunIDs {
			if err := rm.HandleWorkerCapacityLost(runID, string(workerID)); err != nil {
				slog.Error("failed to handle worker capacity loss", "worker_id", workerID, "run_id", runID, "error", err)
			}
		}
	})
	heartbeatMonitor.Start()

	server := api.NewServer(*addr, rm)
	server.SetRegistry(registry)
	server.SetLeaseManager(leaseManager)
	telemetryStore := api.NewTelemetryStoreWithConfig(&api.TelemetryStoreConfig{
		MaxOperationsPerRun: *maxOpsPerRun,
		MaxLogsPerRun:       *maxLogsPerRun,
		MaxTotalRuns:        *maxTotalRuns,
	})
	server.SetTelemetryStore(telemetryStore)
	rm.SetTelemetryStore(telemetryStore)
	server.SetMetricsCollector(metrics.NewCollector())
	rm.SetAssignmentSender(api.NewServerAssignmentAdapter(server))
	server.SetAllowPrivateNetworks(*allowPrivateDiscovery)
	server.SetWorkerAuthEnabled(!*insecureWorkerAuth)
	server.SetRedactAssignmentSecrets(*redactAssignmentSecrets)

	server.SetRateLimiterConfig(&api.RateLimiterConfig{
		RequestsPerSecond: *rateLimit,
		BurstSize:         *rateBurst,
		Enabled:           *rateLimit > 0,
	})

	if strings.EqualFold(*authMode, string(auth.AuthModeNone)) && !*insecure {
		fmt.Fprintln(os.Stderr, "Refusing to start with auth disabled without --insecure")
		os.Exit(1)
	}

	authConfig := &auth.Config{
		Mode:         auth.AuthMode(*authMode),
		InsecureMode: *insecure,
		SkipPaths:    []string{"/healthz", "/readyz"},
	}
	if *insecure {
		authConfig.Mode = auth.AuthModeNone
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

	heartbeatMonitor.Stop()
	registry.Close()
	fmt.Println("Server stopped")
}
