package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// AgentAuthConfig holds configuration for agent authentication
type AgentAuthConfig struct {
	Enabled bool     `json:"enabled"`
	Tokens  []string `json:"tokens"` // allowed tokens
}

// AgentRegisterRequest is the request body for POST /agents/v1/register
type AgentRegisterRequest struct {
	PairKey  string            `json:"pair_key"`
	Hostname string            `json:"hostname"`
	OS       string            `json:"os"`
	Arch     string            `json:"arch"`
	Version  string            `json:"version"`
	Tags     map[string]string `json:"tags,omitempty"`
}

// AgentRegisterResponse is the response body for POST /agents/v1/register
type AgentRegisterResponse struct {
	AgentID    string `json:"agent_id"`
	ServerTime int64  `json:"server_time"` // Unix milliseconds
}

// AgentMetricsRequest is the request body for POST /agents/v1/metrics
type AgentMetricsRequest struct {
	AgentID string               `json:"agent_id"`
	PairKey string               `json:"pair_key"`
	Samples []AgentMetricsSample `json:"samples"`
}

// AgentMetricsResponse is the response body for POST /agents/v1/metrics
type AgentMetricsResponse struct {
	Accepted int `json:"accepted"`
}

// ListAgentsResponse is the response body for GET /agents
type ListAgentsResponse struct {
	Agents []*AgentInfo `json:"agents"`
}

// GetAgentResponse is the response body for GET /agents/{id}
type GetAgentResponse struct {
	*AgentInfo
}

// AggregatedMetrics holds aggregated metrics across agents
type AggregatedMetrics struct {
	CPUMax      float64 `json:"cpu_max,omitempty"`
	CPUAvg      float64 `json:"cpu_avg,omitempty"`
	CPUSum      float64 `json:"cpu_sum,omitempty"`
	MemMax      uint64  `json:"mem_max,omitempty"`
	MemAvg      uint64  `json:"mem_avg,omitempty"`
	MemSum      uint64  `json:"mem_sum,omitempty"`
	SampleCount int     `json:"sample_count"`
}

// ServerMetricsResponse is the response body for GET /runs/{id}/server-metrics
type ServerMetricsResponse struct {
	RunID      string               `json:"run_id"`
	Samples    []AgentMetricsSample `json:"samples"`
	Aggregated *AggregatedMetrics   `json:"aggregated,omitempty"`
}

func generateAgentID() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate agent ID: %w", err)
	}
	return "agent_" + hex.EncodeToString(bytes), nil
}

// handleAgentRegister handles POST /agents/v1/register
func (s *Server) handleAgentRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w, r.Method, "POST")
		return
	}

	var req AgentRegisterRequest
	if err := json.NewDecoder(limitedBody(w, r)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"Invalid JSON request body",
			map[string]interface{}{"parse_error": err.Error()},
		))
		return
	}

	if req.PairKey == "" {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"pair_key is required",
			map[string]interface{}{"field": "pair_key"},
		))
		return
	}

	if req.Hostname == "" {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"hostname is required",
			map[string]interface{}{"field": "hostname"},
		))
		return
	}

	if s.agentStore == nil {
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse("agent store not configured"))
		return
	}

	agentID, err := generateAgentID()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(err.Error()))
		return
	}
	err = s.agentStore.Register(agentID, req.PairKey, req.Hostname, req.OS, req.Arch, req.Version, req.Tags)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(err.Error()))
		return
	}

	s.writeJSON(w, http.StatusCreated, &AgentRegisterResponse{
		AgentID:    agentID,
		ServerTime: time.Now().UnixMilli(),
	})
}

// handleAgentMetrics handles POST /agents/v1/metrics
func (s *Server) handleAgentMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w, r.Method, "POST")
		return
	}

	var req AgentMetricsRequest
	if err := json.NewDecoder(limitedBody(w, r)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"Invalid JSON request body",
			map[string]interface{}{"parse_error": err.Error()},
		))
		return
	}

	if req.AgentID == "" {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"agent_id is required",
			map[string]interface{}{"field": "agent_id"},
		))
		return
	}

	if s.agentStore == nil {
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse("agent store not configured"))
		return
	}

	// Validate agent exists
	if _, ok := s.agentStore.GetAgent(req.AgentID); !ok {
		s.writeError(w, http.StatusNotFound, &ErrorResponse{
			ErrorType:    ErrorTypeNotFound,
			ErrorCode:    "AGENT_NOT_FOUND",
			ErrorMessage: "Agent not found",
			Retryable:    false,
			Details:      map[string]interface{}{"agent_id": req.AgentID},
		})
		return
	}

	err := s.agentStore.IngestMetrics(req.AgentID, req.Samples)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(err.Error()))
		return
	}

	s.writeJSON(w, http.StatusOK, &AgentMetricsResponse{
		Accepted: len(req.Samples),
	})
}

// handleListAgents handles GET /agents
func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r.Method, "GET")
		return
	}

	if s.agentStore == nil {
		s.writeJSON(w, http.StatusOK, &ListAgentsResponse{Agents: []*AgentInfo{}})
		return
	}

	// Check for pair_key filter
	pairKey := r.URL.Query().Get("pair_key")

	var agents []*AgentInfo
	if pairKey != "" {
		agents = s.agentStore.GetAgentsByPairKey(pairKey)
	} else {
		agents = s.agentStore.ListAgents()
	}

	if agents == nil {
		agents = []*AgentInfo{}
	}

	s.writeJSON(w, http.StatusOK, &ListAgentsResponse{Agents: agents})
}

// handleGetAgent handles GET /agents/{id}
func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r.Method, "GET")
		return
	}

	if s.agentStore == nil {
		s.writeError(w, http.StatusNotFound, &ErrorResponse{
			ErrorType:    ErrorTypeNotFound,
			ErrorCode:    "AGENT_NOT_FOUND",
			ErrorMessage: "Agent not found",
			Retryable:    false,
			Details:      map[string]interface{}{"agent_id": agentID},
		})
		return
	}

	agent, ok := s.agentStore.GetAgent(agentID)
	if !ok {
		s.writeError(w, http.StatusNotFound, &ErrorResponse{
			ErrorType:    ErrorTypeNotFound,
			ErrorCode:    "AGENT_NOT_FOUND",
			ErrorMessage: "Agent not found",
			Retryable:    false,
			Details:      map[string]interface{}{"agent_id": agentID},
		})
		return
	}

	s.writeJSON(w, http.StatusOK, &GetAgentResponse{AgentInfo: agent})
}

// handleGetServerMetrics handles GET /runs/{id}/server-metrics
func (s *Server) handleGetServerMetrics(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r.Method, "GET")
		return
	}

	// Validate run exists
	if _, err := s.runManager.GetRun(runID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.writeError(w, http.StatusNotFound, NewNotFoundErrorResponse(runID))
			return
		}
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(err.Error()))
		return
	}

	if s.agentStore == nil {
		s.writeJSON(w, http.StatusOK, &ServerMetricsResponse{
			RunID:   runID,
			Samples: []AgentMetricsSample{},
		})
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	pairKey := query.Get("pair_key")
	agentID := query.Get("agent_id")
	aggregate := query.Get("aggregate") // max, avg, sum

	var from, to int64
	if fromStr := query.Get("from"); fromStr != "" {
		from, _ = strconv.ParseInt(fromStr, 10, 64)
	}
	if toStr := query.Get("to"); toStr != "" {
		to, _ = strconv.ParseInt(toStr, 10, 64)
	}

	// Get samples
	var samples []AgentMetricsSample
	if agentID != "" {
		samples = s.agentStore.GetMetrics(agentID, from, to)
	} else if pairKey != "" {
		samples = s.agentStore.GetMetricsByPairKey(pairKey, from, to)
	} else {
		// Look up the run's configured server_telemetry.pair_key
		runPairKey := s.runManager.GetRunServerTelemetryPairKey(runID)
		if runPairKey != "" {
			samples = s.agentStore.GetMetricsByPairKey(runPairKey, from, to)
		}
	}

	if samples == nil {
		samples = []AgentMetricsSample{}
	}

	response := &ServerMetricsResponse{
		RunID:   runID,
		Samples: samples,
	}

	// Compute aggregations if requested
	if aggregate != "" && len(samples) > 0 {
		agg := computeAggregation(samples, aggregate)
		response.Aggregated = agg
	}

	s.writeJSON(w, http.StatusOK, response)
}

// computeAggregation computes aggregated metrics from samples
func computeAggregation(samples []AgentMetricsSample, aggregate string) *AggregatedMetrics {
	agg := &AggregatedMetrics{
		SampleCount: len(samples),
	}

	var cpuSum, cpuMax float64
	var memSum, memMax uint64
	cpuCount := 0
	memCount := 0

	for _, sample := range samples {
		if sample.Host != nil {
			cpuSum += sample.Host.CPUPercent
			if sample.Host.CPUPercent > cpuMax {
				cpuMax = sample.Host.CPUPercent
			}
			cpuCount++

			memSum += sample.Host.MemUsed
			if sample.Host.MemUsed > memMax {
				memMax = sample.Host.MemUsed
			}
			memCount++
		}
	}

	switch aggregate {
	case "max":
		agg.CPUMax = cpuMax
		agg.MemMax = memMax
	case "avg":
		if cpuCount > 0 {
			agg.CPUAvg = cpuSum / float64(cpuCount)
		}
		if memCount > 0 {
			agg.MemAvg = memSum / uint64(memCount)
		}
	case "sum":
		agg.CPUSum = cpuSum
		agg.MemSum = memSum
	default:
		// Include all aggregations
		agg.CPUMax = cpuMax
		agg.MemMax = memMax
		if cpuCount > 0 {
			agg.CPUAvg = cpuSum / float64(cpuCount)
		}
		if memCount > 0 {
			agg.MemAvg = memSum / uint64(memCount)
		}
		agg.CPUSum = cpuSum
		agg.MemSum = memSum
	}

	return agg
}

// routeAgents dispatches /agents/* routes
func (s *Server) routeAgents(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/agents/")
	if path == "" || path == "/" {
		s.handleListAgents(w, r)
		return
	}

	// Handle /agents/{id}
	parts := strings.Split(path, "/")
	agentID := parts[0]

	if len(parts) == 1 {
		s.handleGetAgent(w, r, agentID)
		return
	}

	// Unknown sub-path
	s.writeError(w, http.StatusNotFound, &ErrorResponse{
		ErrorType:    ErrorTypeNotFound,
		ErrorCode:    "ENDPOINT_NOT_FOUND",
		ErrorMessage: "Endpoint not found",
		Retryable:    false,
		Details:      map[string]interface{}{"path": r.URL.Path},
	})
}

// agentAuthMiddleware checks Bearer token from Authorization header for agent endpoints
func (s *Server) agentAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If agent auth is not configured or not enabled, pass through
		if s.agentAuthConfig == nil || !s.agentAuthConfig.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Extract Bearer token
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			s.writeError(w, http.StatusUnauthorized, &ErrorResponse{
				ErrorType:    ErrorTypeUnauthorized,
				ErrorCode:    "MISSING_AUTH_TOKEN",
				ErrorMessage: "Authorization header is required",
				Retryable:    false,
			})
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			s.writeError(w, http.StatusUnauthorized, &ErrorResponse{
				ErrorType:    ErrorTypeUnauthorized,
				ErrorCode:    "INVALID_AUTH_FORMAT",
				ErrorMessage: "Authorization header must use Bearer scheme",
				Retryable:    false,
			})
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")

		// Check if token is in allowed list
		valid := false
		for _, allowed := range s.agentAuthConfig.Tokens {
			if token == allowed {
				valid = true
				break
			}
		}

		if !valid {
			s.writeError(w, http.StatusUnauthorized, &ErrorResponse{
				ErrorType:    ErrorTypeUnauthorized,
				ErrorCode:    "INVALID_AUTH_TOKEN",
				ErrorMessage: "Invalid or expired token",
				Retryable:    false,
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}
