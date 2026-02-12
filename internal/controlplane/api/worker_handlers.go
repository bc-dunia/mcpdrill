package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/controlplane/scheduler"
	"github.com/bc-dunia/mcpdrill/internal/types"
)

// Regex patterns for ID validation
var (
	runIDPattern       = regexp.MustCompile(`^run_[0-9a-f]{16,64}$`)
	executionIDPattern = regexp.MustCompile(`^exe_[0-9a-f]{8,64}$`)
	workerIDPattern    = regexp.MustCompile(`^wkr_[0-9a-f]{8,64}$`)
	stageIDPattern     = regexp.MustCompile(`^stg_[0-9a-f]{3,81}$`)
)

// Allowed stage values
var allowedStages = map[string]bool{
	"preflight": true,
	"baseline":  true,
	"ramp":      true,
	"soak":      true,
	"spike":     true,
	"custom":    true,
}

func (s *Server) handleListWorkers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r.Method, "GET")
		return
	}

	if s.registry == nil {
		s.writeJSON(w, http.StatusOK, &ListWorkersResponse{Workers: []*scheduler.WorkerInfo{}})
		return
	}

	workers := s.registry.ListWorkers()
	sort.Slice(workers, func(i, j int) bool {
		return workers[i].WorkerID < workers[j].WorkerID
	})

	s.writeJSON(w, http.StatusOK, &ListWorkersResponse{Workers: workers})
}

func (s *Server) handleRegisterWorker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w, r.Method, "POST")
		return
	}

	var req RegisterWorkerRequest
	if err := json.NewDecoder(limitedBody(w, r)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"Invalid JSON request body",
			map[string]interface{}{"parse_error": err.Error()},
		))
		return
	}

	if req.HostInfo.Hostname == "" {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"host_info.hostname is required",
			map[string]interface{}{"field": "host_info.hostname"},
		))
		return
	}

	if req.Capacity.MaxVUs <= 0 {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"capacity.max_vus must be positive",
			map[string]interface{}{"field": "capacity.max_vus"},
		))
		return
	}

	if s.registry == nil {
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse("registry not configured"))
		return
	}

	workerID, err := s.registry.RegisterWorker(req.HostInfo, req.Capacity)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(err.Error()))
		return
	}

	workerToken, err := s.issueWorkerToken(string(workerID))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse("failed to issue worker token"))
		return
	}

	s.writeJSON(w, http.StatusCreated, &RegisterWorkerResponse{
		WorkerID:    string(workerID),
		WorkerToken: workerToken,
	})
}

func (s *Server) handleWorkerHeartbeat(w http.ResponseWriter, r *http.Request, workerID string) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w, r.Method, "POST")
		return
	}

	if !s.verifyWorkerToken(w, r, workerID) {
		return
	}

	var req HeartbeatRequest
	if err := json.NewDecoder(limitedBody(w, r)).Decode(&req); err != nil && err.Error() != "EOF" {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"Invalid JSON request body",
			map[string]interface{}{"parse_error": err.Error()},
		))
		return
	}

	if s.registry == nil {
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse("registry not configured"))
		return
	}

	err := s.registry.Heartbeat(scheduler.WorkerID(workerID), req.Health)
	if err != nil {
		if err == scheduler.ErrWorkerNotFound {
			s.writeError(w, http.StatusNotFound, &ErrorResponse{
				ErrorType:    ErrorTypeNotFound,
				ErrorCode:    ErrorCodeWorkerNotFound,
				ErrorMessage: "Worker not found",
				Retryable:    false,
				Details:      map[string]interface{}{"worker_id": workerID},
			})
			return
		}
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(err.Error()))
		return
	}

	if s.leaseManager != nil {
		_ = s.leaseManager.RenewWorkerLeases(scheduler.WorkerID(workerID))
	}

	stopRunIDs := s.getStoppingRunsForWorker(workerID)
	immediateStopRunIDs := s.getImmediateStopRunsForWorker(workerID)

	s.writeJSON(w, http.StatusOK, &HeartbeatResponse{
		OK:                  true,
		StopRunIDs:          stopRunIDs,
		ImmediateStopRunIDs: immediateStopRunIDs,
	})
}

func (s *Server) getStoppingRunsForWorker(workerID string) []string {
	if s.runManager == nil {
		return nil
	}
	return s.runManager.ListStoppingRunsForWorker(workerID)
}

func (s *Server) getImmediateStopRunsForWorker(workerID string) []string {
	if s.runManager == nil {
		return nil
	}
	return s.runManager.ListImmediateStopRunsForWorker(workerID)
}

func (s *Server) handleWorkerTelemetry(w http.ResponseWriter, r *http.Request, workerID string) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w, r.Method, "POST")
		return
	}

	if !s.verifyWorkerToken(w, r, workerID) {
		return
	}

	var req TelemetryBatchRequest
	if err := json.NewDecoder(limitedBody(w, r)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"Invalid JSON request body",
			map[string]interface{}{"parse_error": err.Error()},
		))
		return
	}

	if validationErr := s.validateTelemetryCorrelationKeys(req, workerID); validationErr != nil {
		s.writeError(w, http.StatusBadRequest, validationErr)
		return
	}

	if s.registry == nil {
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse("registry not configured"))
		return
	}

	_, err := s.registry.GetWorker(scheduler.WorkerID(workerID))
	if err != nil {
		if err == scheduler.ErrWorkerNotFound {
			s.writeError(w, http.StatusNotFound, &ErrorResponse{
				ErrorType:    ErrorTypeNotFound,
				ErrorCode:    ErrorCodeWorkerNotFound,
				ErrorMessage: "Worker not found",
				Retryable:    false,
				Details:      map[string]interface{}{"worker_id": workerID},
			})
			return
		}
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(err.Error()))
		return
	}

	if req.Health != nil {
		_ = s.registry.Heartbeat(scheduler.WorkerID(workerID), req.Health)
	}

	if s.telemetryStore != nil && len(req.Operations) > 0 {
		// Add worker context to each operation before storing
		for i := range req.Operations {
			if req.Operations[i].WorkerID == "" {
				req.Operations[i].WorkerID = workerID
			}
		}
		runID := s.extractRunIDFromTelemetry(req)
		if runID != "" {
			s.telemetryStore.AddTelemetryBatch(runID, req)
		}
	}

	s.writeJSON(w, http.StatusOK, &TelemetryBatchResponse{Accepted: len(req.Operations)})
}

// validateTelemetryCorrelationKeys validates required correlation keys in telemetry batch.
// Required keys: run_id (batch level), execution_id, stage, stage_id, worker_id (per operation or inferred).
// Also validates format of IDs and stage against allowed enum.
func (s *Server) validateTelemetryCorrelationKeys(req TelemetryBatchRequest, workerID string) *ErrorResponse {
	var missingKeys []string
	var invalidKeys []string

	// run_id is required at batch level
	if req.RunID == "" {
		missingKeys = append(missingKeys, "run_id")
	} else if !runIDPattern.MatchString(req.RunID) {
		invalidKeys = append(invalidKeys, "run_id")
	}

	// Check each operation for required keys
	for i, op := range req.Operations {
		var opMissing []string
		var opInvalid []string

		// worker_id must match URL path workerID if provided (prevent spoofing)
		// Reject if op.WorkerID is set but does not match the authenticated workerID
		if op.WorkerID != "" && op.WorkerID != workerID {
			return &ErrorResponse{
				ErrorType:    ErrorTypeInvalidArgument,
				ErrorCode:    "INVALID_TELEMETRY",
				ErrorMessage: "worker_id mismatch: operation worker_id does not match authenticated worker",
				Retryable:    false,
				Details: map[string]interface{}{
					"operation_index": i,
					"expected":        workerID,
					"got":             op.WorkerID,
				},
			}
		}
		effectiveWorkerID := op.WorkerID
		if effectiveWorkerID == "" {
			effectiveWorkerID = workerID
		}
		if effectiveWorkerID == "" {
			opMissing = append(opMissing, "worker_id")
		} else if !workerIDPattern.MatchString(effectiveWorkerID) {
			opInvalid = append(opInvalid, "worker_id")
		}

		// execution_id is required
		if op.ExecutionID == "" {
			opMissing = append(opMissing, "execution_id")
		} else if !executionIDPattern.MatchString(op.ExecutionID) {
			opInvalid = append(opInvalid, "execution_id")
		}

		// stage is required and must be valid enum
		if op.Stage == "" {
			opMissing = append(opMissing, "stage")
		} else if !allowedStages[op.Stage] {
			opInvalid = append(opInvalid, "stage")
		}

		// stage_id is required
		if op.StageID == "" {
			opMissing = append(opMissing, "stage_id")
		} else if !stageIDPattern.MatchString(op.StageID) {
			opInvalid = append(opInvalid, "stage_id")
		}

		if len(opMissing) > 0 {
			return &ErrorResponse{
				ErrorType:    ErrorTypeInvalidArgument,
				ErrorCode:    "INVALID_TELEMETRY",
				ErrorMessage: "Missing required correlation keys in telemetry",
				Retryable:    false,
				Details: map[string]interface{}{
					"operation_index": i,
					"missing_keys":    opMissing,
				},
			}
		}

		if len(opInvalid) > 0 {
			return &ErrorResponse{
				ErrorType:    ErrorTypeInvalidArgument,
				ErrorCode:    "INVALID_TELEMETRY",
				ErrorMessage: "Invalid correlation key format in telemetry",
				Retryable:    false,
				Details: map[string]interface{}{
					"operation_index": i,
					"invalid_keys":    opInvalid,
				},
			}
		}
	}

	if len(missingKeys) > 0 {
		return &ErrorResponse{
			ErrorType:    ErrorTypeInvalidArgument,
			ErrorCode:    "INVALID_TELEMETRY",
			ErrorMessage: "Missing required correlation keys in telemetry batch",
			Retryable:    false,
			Details: map[string]interface{}{
				"missing_keys": missingKeys,
			},
		}
	}

	if len(invalidKeys) > 0 {
		return &ErrorResponse{
			ErrorType:    ErrorTypeInvalidArgument,
			ErrorCode:    "INVALID_TELEMETRY",
			ErrorMessage: "Invalid correlation key format in telemetry batch",
			Retryable:    false,
			Details: map[string]interface{}{
				"invalid_keys": invalidKeys,
			},
		}
	}

	return nil
}

func (s *Server) handleGetAssignments(w http.ResponseWriter, r *http.Request, workerID string) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r.Method, "GET")
		return
	}

	if !s.verifyWorkerToken(w, r, workerID) {
		return
	}

	if s.registry == nil {
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse("registry not configured"))
		return
	}

	_, err := s.registry.GetWorker(scheduler.WorkerID(workerID))
	if err != nil {
		if err == scheduler.ErrWorkerNotFound {
			s.writeError(w, http.StatusNotFound, &ErrorResponse{
				ErrorType:    ErrorTypeNotFound,
				ErrorCode:    ErrorCodeWorkerNotFound,
				ErrorMessage: "Worker not found",
				Retryable:    false,
				Details:      map[string]interface{}{"worker_id": workerID},
			})
			return
		}
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(err.Error()))
		return
	}

	assignments := s.getAssignmentsForWorker(workerID)
	if s.shouldRedactAssignmentSecrets() {
		assignments = redactAssignments(assignments)
	}
	s.writeJSON(w, http.StatusOK, &GetAssignmentsResponse{Assignments: assignments})
}

func (s *Server) handleAckAssignments(w http.ResponseWriter, r *http.Request, workerID string) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w, r.Method, "POST")
		return
	}

	if !s.verifyWorkerToken(w, r, workerID) {
		return
	}

	if s.registry == nil {
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse("registry not configured"))
		return
	}

	_, err := s.registry.GetWorker(scheduler.WorkerID(workerID))
	if err != nil {
		if err == scheduler.ErrWorkerNotFound {
			s.writeError(w, http.StatusNotFound, &ErrorResponse{
				ErrorType:    ErrorTypeNotFound,
				ErrorCode:    ErrorCodeWorkerNotFound,
				ErrorMessage: "Worker not found",
				Retryable:    false,
				Details:      map[string]interface{}{"worker_id": workerID},
			})
			return
		}
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(err.Error()))
		return
	}

	var req AckAssignmentsRequest
	if err := json.NewDecoder(limitedBody(w, r)).Decode(&req); err != nil && err.Error() != "EOF" {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"Invalid JSON request body",
			map[string]interface{}{"parse_error": err.Error()},
		))
		return
	}

	acked := s.ackAssignmentsForWorker(workerID, req.LeaseIDs)
	s.writeJSON(w, http.StatusOK, &AckAssignmentsResponse{Acknowledged: acked})
}

func (s *Server) getAssignmentsForWorker(workerID string) []types.WorkerAssignment {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pendingAssignments == nil {
		return []types.WorkerAssignment{}
	}

	assignments, ok := s.pendingAssignments[workerID]
	if !ok {
		return []types.WorkerAssignment{}
	}

	delete(s.pendingAssignments, workerID)
	if s.pendingAck == nil {
		s.pendingAck = make(map[string][]deliveredAssignment)
	}

	now := time.Now()
	delivered := make([]deliveredAssignment, len(assignments))
	for i, assignment := range assignments {
		delivered[i] = deliveredAssignment{assignment: assignment, deliveredAt: now}
	}
	s.pendingAck[workerID] = append(s.pendingAck[workerID], delivered...)
	return assignments
}

func (s *Server) ackAssignmentsForWorker(workerID string, leaseIDs []string) int {
	if len(leaseIDs) == 0 {
		return 0
	}

	leaseIDSet := make(map[string]struct{}, len(leaseIDs))
	for _, leaseID := range leaseIDs {
		if leaseID == "" {
			continue
		}
		leaseIDSet[leaseID] = struct{}{}
	}
	if len(leaseIDSet) == 0 {
		return 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	pending := s.pendingAck[workerID]
	if len(pending) == 0 {
		return 0
	}

	acked := 0
	remaining := pending[:0]
	for _, delivered := range pending {
		if _, ok := leaseIDSet[delivered.assignment.LeaseID]; ok {
			acked++
			continue
		}
		remaining = append(remaining, delivered)
	}
	if len(remaining) == 0 {
		delete(s.pendingAck, workerID)
		return acked
	}
	s.pendingAck[workerID] = remaining
	return acked
}

func (s *Server) AddAssignment(workerID string, assignment types.WorkerAssignment) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.addAssignmentLocked(workerID, assignment)
}

type ServerAssignmentAdapter struct {
	server *Server
}

func NewServerAssignmentAdapter(server *Server) *ServerAssignmentAdapter {
	return &ServerAssignmentAdapter{server: server}
}

func (a *ServerAssignmentAdapter) AddAssignment(workerID string, assignment types.WorkerAssignment) {
	a.server.AddAssignment(workerID, assignment)
}

func (s *Server) extractRunIDFromTelemetry(req TelemetryBatchRequest) string {
	return req.RunID
}

func (s *Server) issueWorkerToken(workerID string) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(tokenBytes)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.workerTokens == nil {
		s.workerTokens = make(map[string]string)
	}
	s.workerTokens[workerID] = token
	return token, nil
}

func (s *Server) verifyWorkerToken(w http.ResponseWriter, r *http.Request, workerID string) bool {
	if !s.isWorkerAuthEnabled() {
		return true
	}

	// Security: worker endpoints carry assignment secrets, so require a worker token.
	token := r.Header.Get("X-Worker-Token")
	if token == "" {
		s.writeError(w, http.StatusUnauthorized, &ErrorResponse{
			ErrorType:    ErrorTypeUnauthorized,
			ErrorCode:    "WORKER_AUTH_REQUIRED",
			ErrorMessage: "Worker token required",
			Retryable:    false,
		})
		return false
	}

	s.mu.Lock()
	expected, ok := s.workerTokens[workerID]
	s.mu.Unlock()
	if !ok || subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
		s.writeError(w, http.StatusUnauthorized, &ErrorResponse{
			ErrorType:    ErrorTypeUnauthorized,
			ErrorCode:    "INVALID_WORKER_TOKEN",
			ErrorMessage: "Invalid worker token",
			Retryable:    false,
		})
		return false
	}

	return true
}

func redactAssignments(assignments []types.WorkerAssignment) []types.WorkerAssignment {
	if len(assignments) == 0 {
		return assignments
	}
	redacted := make([]types.WorkerAssignment, len(assignments))
	for i, assignment := range assignments {
		redacted[i] = assignment
		if assignment.Target.Headers != nil {
			headers := make(map[string]string, len(assignment.Target.Headers))
			for key, value := range assignment.Target.Headers {
				if isSensitiveHeader(key) {
					headers[key] = "[redacted]"
					continue
				}
				headers[key] = value
			}
			redacted[i].Target.Headers = headers
		}
		if assignment.Target.Auth != nil && len(assignment.Target.Auth.Tokens) > 0 {
			redacted[i].Target.Auth = &types.AuthConfig{
				Type:   assignment.Target.Auth.Type,
				Tokens: []string{"[redacted]"},
			}
		}
	}
	return redacted
}

func isSensitiveHeader(header string) bool {
	switch strings.ToLower(header) {
	case "authorization", "proxy-authorization", "x-api-key", "x-auth-token":
		return true
	default:
		return false
	}
}
