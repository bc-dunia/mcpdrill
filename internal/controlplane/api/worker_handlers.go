package api

import (
	"encoding/json"
	"net/http"
	"regexp"

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

	s.writeJSON(w, http.StatusCreated, &RegisterWorkerResponse{WorkerID: string(workerID)})
}

func (s *Server) handleWorkerHeartbeat(w http.ResponseWriter, r *http.Request, workerID string) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w, r.Method, "POST")
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

	var req TelemetryBatchRequest
	if err := json.NewDecoder(limitedBody(w, r)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			"Invalid JSON request body",
			map[string]interface{}{"parse_error": err.Error()},
		))
		return
	}

	// Validate required correlation keys per spec
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
	s.writeJSON(w, http.StatusOK, &GetAssignmentsResponse{Assignments: assignments})
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
	return assignments
}

func (s *Server) AddAssignment(workerID string, assignment types.WorkerAssignment) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pendingAssignments == nil {
		s.pendingAssignments = make(map[string][]types.WorkerAssignment)
	}

	s.pendingAssignments[workerID] = append(s.pendingAssignments[workerID], assignment)
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
