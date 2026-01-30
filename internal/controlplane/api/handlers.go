package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/auth"
	"github.com/bc-dunia/mcpdrill/internal/controlplane/runmanager"
	"github.com/bc-dunia/mcpdrill/internal/validation"
)

const (
	// SSE configuration
	sseHeartbeatInterval = 15 * time.Second
	sseEventBatchLimit   = 100 // Max events per poll
	ssePollInterval      = 100 * time.Millisecond
)

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// GET /runs -> list all runs
		s.handleListRuns(w, r)
		return
	}
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w, r.Method, "GET, POST")
		return
	}

	// Check role - require operator or admin
	if s.authConfig != nil && s.authConfig.Mode != auth.AuthModeNone {
		if !auth.HasAnyRole(r.Context(), auth.RoleAdmin, auth.RoleOperator) {
			s.writeError(w, http.StatusForbidden, &ErrorResponse{
				ErrorType:    ErrorTypeForbidden,
				ErrorCode:    "INSUFFICIENT_PERMISSIONS",
				ErrorMessage: "This action requires operator or admin role",
			})
			return
		}
	}

	var req CreateRunRequest
	if err := json.NewDecoder(limitedBody(w, r)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"Invalid JSON request body",
			map[string]interface{}{"parse_error": err.Error()},
		))
		return
	}

	if len(req.Config) == 0 {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"Config is required",
			map[string]interface{}{"field": "config"},
		))
		return
	}

	if req.Actor == "" {
		req.Actor = "api"
	}

	runID, err := s.runManager.CreateRun(req.Config, req.Actor)
	if err != nil {
		if validationErr, ok := err.(*validation.ValidationError); ok {
			s.writeError(w, http.StatusBadRequest, NewValidationErrorResponse(validationErr.Report))
			return
		}
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(err.Error()))
		return
	}

	s.writeJSON(w, http.StatusCreated, &CreateRunResponse{RunID: runID})
}

func (s *Server) handleValidateConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w, r.Method, "POST")
		return
	}

	var req ValidateConfigRequest
	if err := json.NewDecoder(limitedBody(w, r)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"Invalid JSON request body",
			map[string]interface{}{"parse_error": err.Error()},
		))
		return
	}

	if len(req.Config) == 0 {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"Config is required",
			map[string]interface{}{"field": "config"},
		))
		return
	}

	report := s.runManager.ValidateRunConfig(req.Config)
	s.writeJSON(w, http.StatusOK, &ValidateConfigResponse{
		OK:       report.OK,
		Errors:   report.Errors,
		Warnings: report.Warnings,
	})
}

func (s *Server) handleStartRun(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w, r.Method, "POST")
		return
	}

	// Check role - require operator or admin
	if s.authConfig != nil && s.authConfig.Mode != auth.AuthModeNone {
		if !auth.HasAnyRole(r.Context(), auth.RoleAdmin, auth.RoleOperator) {
			s.writeError(w, http.StatusForbidden, &ErrorResponse{
				ErrorType:    ErrorTypeForbidden,
				ErrorCode:    "INSUFFICIENT_PERMISSIONS",
				ErrorMessage: "This action requires operator or admin role",
			})
			return
		}
	}

	var req StartRunRequest
	if err := json.NewDecoder(limitedBody(w, r)).Decode(&req); err != nil && err.Error() != "EOF" {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"Invalid JSON request body",
			map[string]interface{}{"parse_error": err.Error()},
		))
		return
	}

	if req.Actor == "" {
		req.Actor = "api"
	}

	err := s.runManager.StartRun(runID, req.Actor)
	if err != nil {
		s.handleRunManagerError(w, runID, "start", err)
		return
	}

	run, err := s.runManager.GetRun(runID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(err.Error()))
		return
	}

	s.writeJSON(w, http.StatusOK, &StartRunResponse{
		RunID: runID,
		State: string(run.State),
	})
}

func (s *Server) handleStopRun(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w, r.Method, "POST")
		return
	}

	// Check role - require operator or admin
	if s.authConfig != nil && s.authConfig.Mode != auth.AuthModeNone {
		if !auth.HasAnyRole(r.Context(), auth.RoleAdmin, auth.RoleOperator) {
			s.writeError(w, http.StatusForbidden, &ErrorResponse{
				ErrorType:    ErrorTypeForbidden,
				ErrorCode:    "INSUFFICIENT_PERMISSIONS",
				ErrorMessage: "This action requires operator or admin role",
			})
			return
		}
	}

	var req StopRunRequest
	if err := json.NewDecoder(limitedBody(w, r)).Decode(&req); err != nil && err.Error() != "EOF" {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"Invalid JSON request body",
			map[string]interface{}{"parse_error": err.Error()},
		))
		return
	}

	if req.Actor == "" {
		req.Actor = "api"
	}

	var mode runmanager.StopMode
	switch strings.ToLower(req.Mode) {
	case "drain", "":
		mode = runmanager.StopModeDrain
	case "immediate":
		mode = runmanager.StopModeImmediate
	default:
		s.writeError(w, http.StatusBadRequest, &ErrorResponse{
			ErrorType:    ErrorTypeInvalidArgument,
			ErrorCode:    ErrorCodeInvalidStopMode,
			ErrorMessage: "Invalid stop mode",
			Retryable:    false,
			Details: map[string]interface{}{
				"mode":          req.Mode,
				"allowed_modes": []string{"drain", "immediate"},
			},
		})
		return
	}

	err := s.runManager.RequestStop(runID, mode, req.Actor)
	if err != nil {
		s.handleRunManagerError(w, runID, "stop", err)
		return
	}

	run, err := s.runManager.GetRun(runID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(err.Error()))
		return
	}

	s.writeJSON(w, http.StatusOK, &StopRunResponse{
		RunID: runID,
		State: string(run.State),
	})
}

func (s *Server) handleEmergencyStop(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w, r.Method, "POST")
		return
	}

	// Check role - require operator or admin
	if s.authConfig != nil && s.authConfig.Mode != auth.AuthModeNone {
		if !auth.HasAnyRole(r.Context(), auth.RoleAdmin, auth.RoleOperator) {
			s.writeError(w, http.StatusForbidden, &ErrorResponse{
				ErrorType:    ErrorTypeForbidden,
				ErrorCode:    "INSUFFICIENT_PERMISSIONS",
				ErrorMessage: "This action requires operator or admin role",
			})
			return
		}
	}

	var req EmergencyStopRequest
	if err := json.NewDecoder(limitedBody(w, r)).Decode(&req); err != nil && err.Error() != "EOF" {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"Invalid JSON request body",
			map[string]interface{}{"parse_error": err.Error()},
		))
		return
	}

	if req.Actor == "" {
		req.Actor = "api"
	}

	err := s.runManager.EmergencyStop(runID, req.Actor)
	if err != nil {
		s.handleRunManagerError(w, runID, "emergency_stop", err)
		return
	}

	run, err := s.runManager.GetRun(runID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(err.Error()))
		return
	}

	s.writeJSON(w, http.StatusOK, &EmergencyStopResponse{
		RunID: runID,
		State: string(run.State),
	})
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r.Method, "GET")
		return
	}

	run, err := s.runManager.GetRun(runID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.writeError(w, http.StatusNotFound, NewNotFoundErrorResponse(runID))
			return
		}
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(err.Error()))
		return
	}

	s.writeJSON(w, http.StatusOK, &GetRunResponse{RunView: run})
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r.Method, "GET")
		return
	}
	s.writeJSON(w, http.StatusOK, &HealthResponse{Status: "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r.Method, "GET")
		return
	}

	ready := s.runManager != nil
	status := "ready"
	if !ready {
		status = "not_ready"
	}

	s.writeJSON(w, http.StatusOK, &ReadyResponse{
		Status: status,
		Ready:  ready,
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r.Method, "GET")
		return
	}

	if s.metricsCollector == nil {
		s.writeError(w, http.StatusServiceUnavailable, &ErrorResponse{
			ErrorType:    ErrorTypeInternal,
			ErrorCode:    "METRICS_NOT_CONFIGURED",
			ErrorMessage: "Metrics collector not configured",
			Retryable:    false,
		})
		return
	}

	s.metricsCollector.SyncFromProviders()
	output := s.metricsCollector.Expose()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(output))
}

func (s *Server) handleRunManagerError(w http.ResponseWriter, runID, operation string, err error) {
	// Try typed error first (preferred path)
	if rmErr := runmanager.AsRunManagerError(err); rmErr != nil {
		switch rmErr.Kind {
		case runmanager.ErrKindNotFound:
			s.writeError(w, http.StatusNotFound, NewNotFoundErrorResponse(rmErr.RunID))
			return
		case runmanager.ErrKindTerminalState:
			s.writeError(w, http.StatusConflict, NewTerminalStateErrorResponse(rmErr.RunID, string(rmErr.State), operation))
			return
		case runmanager.ErrKindInvalidState, runmanager.ErrKindInvalidTransition:
			s.writeError(w, http.StatusConflict, NewInvalidStateErrorResponse(rmErr.RunID, string(rmErr.State), operation))
			return
		default:
			s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(rmErr.Message))
			return
		}
	}

	// Fallback: string matching for backward compatibility with legacy errors
	errMsg := err.Error()

	if strings.Contains(errMsg, "not found") {
		s.writeError(w, http.StatusNotFound, NewNotFoundErrorResponse(runID))
		return
	}

	if strings.Contains(errMsg, "cannot") && strings.Contains(errMsg, "state") {
		if strings.Contains(errMsg, "terminal state") {
			s.writeError(w, http.StatusConflict, NewTerminalStateErrorResponse(runID, extractState(errMsg), operation))
			return
		}
		s.writeError(w, http.StatusConflict, NewInvalidStateErrorResponse(runID, extractState(errMsg), operation))
		return
	}

	if strings.Contains(errMsg, "invalid state transition") {
		s.writeError(w, http.StatusConflict, NewInvalidStateErrorResponse(runID, extractState(errMsg), operation))
		return
	}

	s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(errMsg))
}

func extractState(errMsg string) string {
	// Check for lowercase state strings (actual RunState values)
	states := []string{
		"created", "preflight_running", "preflight_passed", "preflight_failed",
		"baseline_running", "ramp_running", "soak_running",
		"stopping", "analyzing", "completed", "failed", "aborted",
	}
	for _, state := range states {
		if strings.Contains(errMsg, state) {
			return state
		}
	}
	// Fallback: check for uppercase tokens (legacy)
	upperStates := []string{
		"CREATED", "PREFLIGHT_RUNNING", "PREFLIGHT_PASSED", "PREFLIGHT_FAILED",
		"BASELINE_RUNNING", "RAMP_RUNNING", "SOAK_RUNNING",
		"STOPPING", "ANALYZING", "COMPLETED", "FAILED", "ABORTED",
	}
	for _, state := range upperStates {
		if strings.Contains(errMsg, state) {
			return strings.ToLower(state)
		}
	}
	return "unknown"
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) writeError(w http.ResponseWriter, status int, errResp *ErrorResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errResp)
}

func (s *Server) writeMethodNotAllowed(w http.ResponseWriter, method, allowed string) {
	w.Header().Set("Allow", allowed)
	s.writeError(w, http.StatusMethodNotAllowed, &ErrorResponse{
		ErrorType:    ErrorTypeInvalidArgument,
		ErrorCode:    ErrorCodeMethodNotAllowed,
		ErrorMessage: "Method not allowed",
		Retryable:    false,
		Details: map[string]interface{}{
			"method":  method,
			"allowed": allowed,
		},
	})
}

// eventIDPattern validates event IDs: evt_<hex> format
var eventIDPattern = regexp.MustCompile(`^evt_[0-9a-f]+$`)

func (s *Server) handleStreamEvents(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r.Method, "GET")
		return
	}

	if _, err := s.runManager.GetRun(runID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.writeError(w, http.StatusNotFound, NewNotFoundErrorResponse(runID))
			return
		}
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(err.Error()))
		return
	}

	cursor := 0
	cursorSet := false

	// Handle Last-Event-ID header (highest precedence per SSE spec)
	if lastEventID := r.Header.Get("Last-Event-ID"); lastEventID != "" {
		// Validate format: must be evt_<hex>
		if !eventIDPattern.MatchString(lastEventID) {
			s.writeError(w, http.StatusBadRequest, &ErrorResponse{
				ErrorType:    ErrorTypeInvalidArgument,
				ErrorCode:    "INVALID_LAST_EVENT_ID",
				ErrorMessage: "Invalid Last-Event-ID format: must be evt_<hex>",
				Retryable:    false,
				Details:      map[string]interface{}{"last_event_id": lastEventID},
			})
			return
		}
		idx := s.runManager.FindEventIndex(runID, lastEventID)
		if idx < 0 {
			s.writeError(w, http.StatusBadRequest, &ErrorResponse{
				ErrorType:    ErrorTypeInvalidArgument,
				ErrorCode:    "INVALID_LAST_EVENT_ID",
				ErrorMessage: "Last-Event-ID not found in event log",
				Retryable:    false,
				Details:      map[string]interface{}{"last_event_id": lastEventID},
			})
			return
		}
		cursor = idx + 1
		cursorSet = true
	}

	// Handle cursor query parameter (only if Last-Event-ID not provided)
	if !cursorSet {
		if cursorParam := r.URL.Query().Get("cursor"); cursorParam != "" {
			// Validate format: must be evt_<hex>
			if !eventIDPattern.MatchString(cursorParam) {
				s.writeError(w, http.StatusBadRequest, &ErrorResponse{
					ErrorType:    ErrorTypeInvalidArgument,
					ErrorCode:    "INVALID_CURSOR",
					ErrorMessage: "Invalid cursor format: must be evt_<hex>",
					Retryable:    false,
					Details:      map[string]interface{}{"cursor": cursorParam},
				})
				return
			}
			idx := s.runManager.FindEventIndex(runID, cursorParam)
			if idx < 0 {
				s.writeError(w, http.StatusBadRequest, &ErrorResponse{
					ErrorType:    ErrorTypeInvalidArgument,
					ErrorCode:    "INVALID_CURSOR",
					ErrorMessage: "Cursor not found in event log",
					Retryable:    false,
					Details:      map[string]interface{}{"cursor": cursorParam},
				})
				return
			}
			cursor = idx + 1
			cursorSet = true
		}
	}

	// Handle legacy since parameter (only if neither of above provided)
	if !cursorSet {
		if sinceParam := r.URL.Query().Get("since"); sinceParam != "" {
			parsed, err := strconv.Atoi(sinceParam)
			if err != nil || parsed < 0 {
				s.writeError(w, http.StatusBadRequest, &ErrorResponse{
					ErrorType:    ErrorTypeInvalidArgument,
					ErrorCode:    "INVALID_SINCE_PARAM",
					ErrorMessage: "Invalid since parameter: must be non-negative integer",
					Retryable:    false,
					Details:      map[string]interface{}{"since": sinceParam},
				})
				return
			}
			cursor = parsed
		}
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse("Streaming not supported"))
		return
	}

	// Set SSE headers per spec: text/event-stream; charset=utf-8
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	heartbeatTicker := time.NewTicker(sseHeartbeatInterval)
	defer heartbeatTicker.Stop()

	pollTicker := time.NewTicker(ssePollInterval)
	defer pollTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeatTicker.C:
			// Per spec: :keepalive comment every 15s
			fmt.Fprintf(w, ":keepalive\n\n")
			flusher.Flush()
		case <-pollTicker.C:
			events, err := s.runManager.TailEvents(runID, cursor, sseEventBatchLimit)
			if err != nil {
				return
			}

			for _, event := range events {
				eventData, err := json.Marshal(event)
				if err != nil {
					continue
				}

				// Emit SSE event per spec: event, id, data fields
				fmt.Fprintf(w, "event: run_event\n")
				fmt.Fprintf(w, "id: %s\n", event.EventID)
				fmt.Fprintf(w, "data: %s\n\n", eventData)
			}

			if len(events) > 0 {
				flusher.Flush()
				cursor += len(events)
			}
		}
	}
}

// maxRequestBodySize is the maximum allowed request body size (10MB default).
const maxRequestBodySize = 10 * 1024 * 1024

// limitedBody returns a reader that limits the body size.
// Use this before json.NewDecoder to prevent memory exhaustion.
func limitedBody(w http.ResponseWriter, r *http.Request) io.Reader {
	return http.MaxBytesReader(w, r.Body, maxRequestBodySize)
}
