package api

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r.Method, "GET")
		return
	}

	if s.telemetryStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, &ErrorResponse{
			ErrorType:    ErrorTypeInternal,
			ErrorCode:    "TELEMETRY_NOT_CONFIGURED",
			ErrorMessage: "Telemetry store not configured",
			Retryable:    false,
		})
		return
	}

	if !s.telemetryStore.HasRun(runID) {
		s.writeError(w, http.StatusNotFound, NewNotFoundErrorResponse(runID))
		return
	}

	filters, err := parseLogFilters(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse(
			err.Error(),
			nil,
		))
		return
	}

	logs, total, err := s.telemetryStore.QueryLogs(runID, filters)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.writeError(w, http.StatusNotFound, NewNotFoundErrorResponse(runID))
			return
		}
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(err.Error()))
		return
	}

	s.writeJSON(w, http.StatusOK, &LogQueryResponse{
		RunID:  runID,
		Total:  total,
		Offset: filters.Offset,
		Limit:  filters.Limit,
		Logs:   logs,
	})
}

func parseLogFilters(r *http.Request) (LogFilters, error) {
	q := r.URL.Query()

	filters := LogFilters{
		Stage:     q.Get("stage"),
		StageID:   q.Get("stage_id"),
		WorkerID:  q.Get("worker_id"),
		SessionID: q.Get("session_id"),
		Operation: q.Get("operation"),
		ToolName:  q.Get("tool_name"),
		ErrorType: q.Get("error_type"),
		ErrorCode: q.Get("error_code"),
		Limit:     100,
		Offset:    0,
		Order:     "desc",
	}

	if vuIDStr := q.Get("vu_id"); vuIDStr != "" {
		filters.VUID = vuIDStr
	}

	if limitStr := q.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return filters, &InvalidParamError{Param: "limit", Value: limitStr, Reason: "must be an integer"}
		}
		if limit < 1 {
			return filters, &InvalidParamError{Param: "limit", Value: limitStr, Reason: "must be at least 1"}
		}
		if limit > 1000 {
			limit = 1000
		}
		filters.Limit = limit
	}

	if offsetStr := q.Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil {
			return filters, &InvalidParamError{Param: "offset", Value: offsetStr, Reason: "must be an integer"}
		}
		if offset < 0 {
			return filters, &InvalidParamError{Param: "offset", Value: offsetStr, Reason: "must be non-negative"}
		}
		filters.Offset = offset
	}

	if order := q.Get("order"); order != "" {
		order = strings.ToLower(order)
		if order != "asc" && order != "desc" {
			return filters, &InvalidParamError{Param: "order", Value: order, Reason: "must be 'asc' or 'desc'"}
		}
		filters.Order = order
	}

	return filters, nil
}

type InvalidParamError struct {
	Param  string
	Value  string
	Reason string
}

func (e *InvalidParamError) Error() string {
	return "Invalid parameter '" + e.Param + "': " + e.Reason
}
