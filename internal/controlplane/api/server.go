package api

import (
	"context"
	"fmt"
	"log"
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
	"github.com/bc-dunia/mcpdrill/internal/web"
)

const defaultMaxPendingAssignmentsPerWorker = 100

const (
	pendingAckCleanupInterval = 30 * time.Second
	pendingAckTimeout         = 60 * time.Second
)

type deliveredAssignment struct {
	assignment  types.WorkerAssignment
	deliveredAt time.Time
}

type Server struct {
	runManager                     *runmanager.RunManager
	registry                       *scheduler.Registry
	leaseManager                   *scheduler.LeaseManager
	telemetryStore                 *TelemetryStore
	metricsCollector               *metrics.Collector
	server                         *http.Server
	listener                       net.Listener
	mu                             sync.Mutex
	running                        bool
	addr                           string
	pendingAssignments             map[string][]types.WorkerAssignment
	pendingAck                     map[string][]deliveredAssignment
	maxPendingAssignmentsPerWorker int
	customHandlers                 map[string]http.HandlerFunc
	authConfig                     *auth.Config
	authMiddleware                 *auth.Middleware
	allowPrivateNets               bool
	workerTokens                   map[string]string
	workerAuthEnabled              bool
	redactAssignmentSecrets        bool
	rateLimiter                    *rateLimiter
	rateLimiterConfig              *RateLimiterConfig
	agentStore                     *AgentStore
	agentAuthConfig                *AgentAuthConfig
	stopCh                         chan struct{}
}

func NewServer(addr string, rm *runmanager.RunManager) *Server {
	return &Server{
		runManager:                     rm,
		addr:                           addr,
		authConfig:                     auth.DefaultConfig(),
		rateLimiterConfig:              DefaultRateLimiterConfig(),
		maxPendingAssignmentsPerWorker: defaultMaxPendingAssignmentsPerWorker,
		workerAuthEnabled:              true,
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

// SetAllowPrivateNetworks controls whether discovery endpoints can access private IP ranges.
// This is disabled by default to reduce SSRF risk.
func (s *Server) SetAllowPrivateNetworks(allow bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowPrivateNets = allow
}

// SetWorkerAuthEnabled controls whether worker endpoints require a worker token.
// Disable only for legacy or local testing scenarios.
func (s *Server) SetWorkerAuthEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workerAuthEnabled = enabled
}

// SetRedactAssignmentSecrets controls whether assignment responses redact sensitive values.
func (s *Server) SetRedactAssignmentSecrets(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.redactAssignmentSecrets = enabled
}

// SetMaxPendingAssignmentsPerWorker configures the max queue size per worker.
func (s *Server) SetMaxPendingAssignmentsPerWorker(limit int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = defaultMaxPendingAssignmentsPerWorker
	}
	s.maxPendingAssignmentsPerWorker = limit
}

func (s *Server) addAssignmentLocked(workerID string, assignment types.WorkerAssignment) {
	if s.pendingAssignments == nil {
		s.pendingAssignments = make(map[string][]types.WorkerAssignment)
	}

	limit := s.maxPendingAssignmentsPerWorker
	if limit <= 0 {
		limit = defaultMaxPendingAssignmentsPerWorker
	}

	queue := s.pendingAssignments[workerID]
	if len(queue) >= limit {
		dropCount := len(queue) - limit + 1
		queue = queue[dropCount:]
		log.Printf("[Server] Pending assignments limit hit for worker %s: dropped %d oldest", workerID, dropCount)
	}

	queue = append(queue, assignment)
	s.pendingAssignments[workerID] = queue
}

func (s *Server) requeueExpiredPendingAcks(now time.Time, timeout time.Duration) int {
	if timeout <= 0 {
		return 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.pendingAck) == 0 {
		return 0
	}

	requeued := 0
	for workerID, pending := range s.pendingAck {
		if len(pending) == 0 {
			delete(s.pendingAck, workerID)
			continue
		}
		remaining := pending[:0]
		for _, delivered := range pending {
			if now.Sub(delivered.deliveredAt) > timeout {
				s.addAssignmentLocked(workerID, delivered.assignment)
				requeued++
				continue
			}
			remaining = append(remaining, delivered)
		}
		if len(remaining) == 0 {
			delete(s.pendingAck, workerID)
			continue
		}
		s.pendingAck[workerID] = remaining
	}

	return requeued
}

func (s *Server) initAuthMiddlewareLocked() {
	if s.authMiddleware != nil {
		return
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
}

func (s *Server) getAuthMiddleware() *auth.Middleware {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initAuthMiddlewareLocked()
	return s.authMiddleware
}

func (s *Server) SetRegistry(r *scheduler.Registry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registry = r
}

func (s *Server) SetLeaseManager(lm *scheduler.LeaseManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.leaseManager = lm
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

	// Initialize auth middleware before route registration to avoid deadlock
	// (rbacMiddleware calls getAuthMiddleware which would try to acquire s.mu again)
	s.initAuthMiddlewareLocked()

	mux := http.NewServeMux()

	mux.HandleFunc("/runs", s.rateLimitMiddleware(s.rbacMiddleware(http.HandlerFunc(s.handleCreateRun))).ServeHTTP)
	mux.HandleFunc("/runs/", s.rateLimitMiddleware(s.rbacMiddleware(http.HandlerFunc(s.routeRuns))).ServeHTTP)
	mux.HandleFunc("/workers/register", s.rateLimitMiddleware(s.rbacMiddleware(http.HandlerFunc(s.handleRegisterWorker))).ServeHTTP)
	mux.HandleFunc("/workers/", s.rateLimitMiddleware(s.rbacMiddleware(http.HandlerFunc(s.routeWorkers))).ServeHTTP)
	mux.HandleFunc("/agents/v1/register", s.rateLimitMiddleware(s.agentAuthMiddleware(http.HandlerFunc(s.handleAgentRegister))).ServeHTTP)
	mux.HandleFunc("/agents/v1/metrics", s.rateLimitMiddleware(s.agentAuthMiddleware(http.HandlerFunc(s.handleAgentMetrics))).ServeHTTP)
	mux.HandleFunc("/agents", s.rateLimitMiddleware(s.rbacMiddleware(http.HandlerFunc(s.handleListAgents))).ServeHTTP)
	mux.HandleFunc("/agents/", s.rateLimitMiddleware(s.rbacMiddleware(http.HandlerFunc(s.routeAgents))).ServeHTTP)
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/discover-tools", s.rateLimitMiddleware(s.rbacMiddleware(http.HandlerFunc(s.handleDiscoverTools))).ServeHTTP)
	mux.HandleFunc("/test-connection", s.rateLimitMiddleware(s.rbacMiddleware(http.HandlerFunc(s.handleTestConnection))).ServeHTTP)
	mux.HandleFunc("/test-tool", s.rateLimitMiddleware(s.rbacMiddleware(http.HandlerFunc(s.handleTestTool))).ServeHTTP)

	for pattern, handler := range s.customHandlers {
		mux.HandleFunc(pattern, s.rateLimitMiddleware(s.rbacMiddleware(http.HandlerFunc(handler))).ServeHTTP)
	}

	if web.HasAssets() {
		mux.Handle(web.Prefix(), web.Handler())
	}

	if s.authConfig == nil {
		s.authConfig = auth.DefaultConfig()
	}
	if s.authConfig.Mode == auth.AuthModeNone && !s.authConfig.InsecureMode && !isLoopbackBindAddr(s.addr) {
		return fmt.Errorf("refusing to bind to non-loopback address without authentication (use --insecure to override)")
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
	s.stopCh = make(chan struct{})
	stopCh := s.stopCh
	s.startPendingAckRequeueLoop(stopCh)

	srv := s.server
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Printf("server error: %v\n", err)
		}
	}()

	return nil
}

func (s *Server) startPendingAckRequeueLoop(stopCh <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(pendingAckCleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				requeued := s.requeueExpiredPendingAcks(time.Now(), pendingAckTimeout)
				if requeued > 0 {
					log.Printf("[Server] Requeued %d expired assignments", requeued)
				}
			}
		}
	}()
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}

	s.running = false
	srv := s.server
	stopCh := s.stopCh
	s.server = nil
	s.stopCh = nil
	s.mu.Unlock()

	if stopCh != nil {
		close(stopCh)
	}

	if srv != nil {
		return srv.Shutdown(ctx)
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

func isLoopbackBindAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
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

	if path == "validate" {
		s.handleValidateConfig(w, r)
		return
	}

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
	case "clone":
		s.handleCloneRun(w, r, runID)
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
		if len(parts) == 3 && parts[2] == "ack" {
			s.handleAckAssignments(w, r, workerID)
			return
		}
		if len(parts) == 2 {
			s.handleGetAssignments(w, r, workerID)
			return
		}
		s.writeError(w, http.StatusNotFound, &ErrorResponse{
			ErrorType:    ErrorTypeNotFound,
			ErrorCode:    "ENDPOINT_NOT_FOUND",
			ErrorMessage: "Endpoint not found",
			Retryable:    false,
			Details:      map[string]interface{}{"path": r.URL.Path},
		})
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
	if s.authMiddleware != nil {
		return s.authMiddleware.Handler(next)
	}
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

		key := s.rateLimitKey(r)
		if !rl.allowKey(key) {
			log.Printf("[RateLimiter] Rate limit exceeded for %s", key)
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

func (s *Server) rateLimitKey(r *http.Request) string {
	if user := auth.GetUserFromContext(r.Context()); user != nil && user.ID != "" {
		return "user:" + user.ID
	}
	ip := clientIPFromRequest(r)
	if ip == "" {
		ip = "unknown"
	}
	return "ip:" + ip
}

func (s *Server) allowPrivateDiscoveryNetworks() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.allowPrivateNets
}

func (s *Server) isWorkerAuthEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.workerAuthEnabled
}

func (s *Server) shouldRedactAssignmentSecrets() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.redactAssignmentSecrets
}

func clientIPFromRequest(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// StartTestServer creates a test server and returns it with a cleanup function.
// Returns an error if the server fails to start.
// Auth is disabled for testing purposes.
func StartTestServer(rm *runmanager.RunManager) (*Server, func(), error) {
	server := NewServer("127.0.0.1:0", rm)
	server.SetAuthConfig(&auth.Config{
		Mode:      auth.AuthModeNone,
		SkipPaths: []string{"/healthz", "/readyz"},
	})
	server.SetAllowPrivateNetworks(true)
	server.SetWorkerAuthEnabled(false)
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
