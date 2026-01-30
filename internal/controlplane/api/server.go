package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/auth"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/runmanager"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/scheduler"
	"github.com/bc-dunia/mcpdrill/internal/metrics"
	"github.com/bc-dunia/mcpdrill/internal/types"
)

type Server struct {
	runManager         *runmanager.RunManager
	registry           *scheduler.Registry
	telemetryStore     *TelemetryStore
	metricsCollector   *metrics.Collector
	server             *http.Server
	listener           net.Listener
	mu                 sync.Mutex
	running            bool
	addr               string
	pendingAssignments map[string][]types.WorkerAssignment
	customHandlers     map[string]http.HandlerFunc
	authConfig         *auth.Config
	authMiddleware     *auth.Middleware
	rateLimiter        *rateLimiter
	rateLimiterConfig  *RateLimiterConfig
	agentStore         *AgentStore
	agentAuthConfig    *AgentAuthConfig
}

func NewServer(addr string, rm *runmanager.RunManager) *Server {
	return &Server{
		runManager:        rm,
		addr:              addr,
		authConfig:        auth.DefaultConfig(),
		rateLimiterConfig: DefaultRateLimiterConfig(),
	}
}

func (s *Server) SetAuthConfig(config *auth.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authConfig = config
	s.authMiddleware = nil
}

// SetRateLimiterConfig configures the rate limiter.
// Must be called before Start() for changes to take effect.
func (s *Server) SetRateLimiterConfig(config *RateLimiterConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rateLimiterConfig = config
	s.rateLimiter = nil // Reset to pick up new config
}

func (s *Server) getAuthMiddleware() *auth.Middleware {
	if s.authMiddleware != nil {
		return s.authMiddleware
	}

	if s.authConfig == nil {
		s.authConfig = auth.DefaultConfig()
	}

	var authenticator auth.Authenticator
	switch s.authConfig.Mode {
	case auth.AuthModeAPIKey:
		authenticator = auth.NewAPIKeyAuthenticator(s.authConfig)
	case auth.AuthModeJWT:
		authenticator = auth.NewJWTAuthenticator(s.authConfig)
	default:
		authenticator = nil
	}

	s.authMiddleware = auth.NewMiddleware(s.authConfig, authenticator)
	return s.authMiddleware
}

func (s *Server) SetRegistry(r *scheduler.Registry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registry = r
}

func (s *Server) SetTelemetryStore(ts *TelemetryStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.telemetryStore = ts
}

func (s *Server) GetTelemetryStore() *TelemetryStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.telemetryStore
}

func (s *Server) SetMetricsCollector(mc *metrics.Collector) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metricsCollector = mc
}

func (s *Server) GetMetricsCollector() *metrics.Collector {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metricsCollector
}

func (s *Server) SetAgentStore(store *AgentStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentStore = store
}

func (s *Server) GetAgentStore() *AgentStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.agentStore
}

func (s *Server) SetAgentAuthConfig(config *AgentAuthConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentAuthConfig = config
}

func (s *Server) SetCustomHandler(pattern string, handler http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.customHandlers == nil {
		s.customHandlers = make(map[string]http.HandlerFunc)
	}
	s.customHandlers[pattern] = handler
}

func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server already running")
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/runs", s.rbacMiddleware(s.rateLimitMiddleware(http.HandlerFunc(s.handleCreateRun))).ServeHTTP)
	mux.HandleFunc("/runs/", s.rbacMiddleware(s.rateLimitMiddleware(http.HandlerFunc(s.routeRuns))).ServeHTTP)
	mux.HandleFunc("/workers/register", s.rbacMiddleware(s.rateLimitMiddleware(http.HandlerFunc(s.handleRegisterWorker))).ServeHTTP)
	mux.HandleFunc("/workers/", s.rbacMiddleware(s.rateLimitMiddleware(http.HandlerFunc(s.routeWorkers))).ServeHTTP)
	mux.HandleFunc("/agents/v1/register", s.agentAuthMiddleware(s.rateLimitMiddleware(http.HandlerFunc(s.handleAgentRegister))).ServeHTTP)
	mux.HandleFunc("/agents/v1/metrics", s.agentAuthMiddleware(s.rateLimitMiddleware(http.HandlerFunc(s.handleAgentMetrics))).ServeHTTP)
	mux.HandleFunc("/agents", s.rbacMiddleware(s.rateLimitMiddleware(http.HandlerFunc(s.handleListAgents))).ServeHTTP)
	mux.HandleFunc("/agents/", s.rbacMiddleware(s.rateLimitMiddleware(http.HandlerFunc(s.routeAgents))).ServeHTTP)
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/discover-tools", s.rateLimitMiddleware(http.HandlerFunc(s.handleDiscoverTools)).ServeHTTP)
	mux.HandleFunc("/test-connection", s.rateLimitMiddleware(http.HandlerFunc(s.handleTestConnection)).ServeHTTP)

	for pattern, handler := range s.customHandlers {
		mux.HandleFunc(pattern, s.rbacMiddleware(s.rateLimitMiddleware(http.HandlerFunc(handler))).ServeHTTP)
	}

	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.listener = listener

	s.server = &http.Server{
		Handler:           mux,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 10 * time.Second, // Protect against slowloris attacks
	}

	s.running = true

	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Printf("server error: %v\n", err)
		}
	}()

	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false

	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.addr
}

func (s *Server) URL() string {
	return fmt.Sprintf("http://%s", s.Addr())
}

func (s *Server) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Server) routeRuns(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/runs/")
	if path == "" || path == "/" {
		if r.Method == http.MethodGet {
			s.handleListRuns(w, r)
			return
		}
		s.handleCreateRun(w, r)
		return
	}

	// Handle /runs/{id}/compare/{id2}
	if strings.Contains(path, "/compare/") {
		s.handleCompareRuns(w, r)
		return
	}

	parts := strings.Split(path, "/")
	runID := parts[0]

	if len(parts) == 1 {
		s.handleGetRun(w, r, runID)
		return
	}

	action := parts[1]
	switch action {
	case "validate":
		s.handleValidateConfig(w, r)
	case "start":
		s.handleStartRun(w, r, runID)
	case "stop":
		s.handleStopRun(w, r, runID)
	case "emergency-stop":
		s.handleEmergencyStop(w, r, runID)
	case "events":
		s.handleStreamEvents(w, r, runID)
	case "logs":
		s.handleGetLogs(w, r, runID)
	case "metrics":
		s.handleGetRunMetrics(w, r, runID)
	case "stability":
		s.handleGetRunStability(w, r, runID)
	case "server-metrics":
		s.handleGetServerMetrics(w, r, runID)
	case "errors":
		if len(parts) >= 3 && parts[2] == "signatures" {
			s.handleGetErrorSignatures(w, r, runID)
		} else {
			s.writeError(w, http.StatusNotFound, &ErrorResponse{
				ErrorType:    ErrorTypeNotFound,
				ErrorCode:    "ENDPOINT_NOT_FOUND",
				ErrorMessage: "Endpoint not found",
				Retryable:    false,
				Details:      map[string]interface{}{"path": r.URL.Path},
			})
		}
	default:
		s.writeError(w, http.StatusNotFound, &ErrorResponse{
			ErrorType:    ErrorTypeNotFound,
			ErrorCode:    "ENDPOINT_NOT_FOUND",
			ErrorMessage: "Endpoint not found",
			Retryable:    false,
			Details: map[string]interface{}{
				"path": r.URL.Path,
			},
		})
	}
}

func (s *Server) routeWorkers(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/workers/")
	if path == "" || path == "/" || path == "register" {
		s.handleRegisterWorker(w, r)
		return
	}

	parts := strings.Split(path, "/")
	workerID := parts[0]

	if len(parts) == 1 {
		s.writeError(w, http.StatusNotFound, &ErrorResponse{
			ErrorType:    ErrorTypeNotFound,
			ErrorCode:    "ENDPOINT_NOT_FOUND",
			ErrorMessage: "Endpoint not found",
			Retryable:    false,
			Details:      map[string]interface{}{"path": r.URL.Path},
		})
		return
	}

	action := parts[1]
	switch action {
	case "heartbeat":
		s.handleWorkerHeartbeat(w, r, workerID)
	case "telemetry":
		s.handleWorkerTelemetry(w, r, workerID)
	case "assignments":
		s.handleGetAssignments(w, r, workerID)
	default:
		s.writeError(w, http.StatusNotFound, &ErrorResponse{
			ErrorType:    ErrorTypeNotFound,
			ErrorCode:    "ENDPOINT_NOT_FOUND",
			ErrorMessage: "Endpoint not found",
			Retryable:    false,
			Details:      map[string]interface{}{"path": r.URL.Path},
		})
	}
}

func (s *Server) rbacMiddleware(next http.Handler) http.Handler {
	return s.getAuthMiddleware().Handler(next)
}

func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Lazy initialize rate limiter
		s.mu.Lock()
		if s.rateLimiter == nil {
			s.rateLimiter = newRateLimiter(s.rateLimiterConfig)
		}
		rl := s.rateLimiter
		config := s.rateLimiterConfig
		s.mu.Unlock()

		if !rl.allow() {
			// Set rate limit headers per spec
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", config.BurstSize))
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(time.Second).Unix()))
			w.Header().Set("Retry-After", "1")

			s.writeError(w, http.StatusTooManyRequests, &ErrorResponse{
				ErrorType:    "rate_limited",
				ErrorCode:    "RATE_LIMIT_EXCEEDED",
				ErrorMessage: "Too many requests. Please slow down.",
				Retryable:    true,
				Details: map[string]interface{}{
					"retry_after_seconds": 1,
				},
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// StartTestServer creates a test server and returns it with a cleanup function.
// Returns an error if the server fails to start.
func StartTestServer(rm *runmanager.RunManager) (*Server, func(), error) {
	server := NewServer("127.0.0.1:0", rm)
	if err := server.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start test server: %w", err)
	}
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}
	return server, cleanup, nil
}
