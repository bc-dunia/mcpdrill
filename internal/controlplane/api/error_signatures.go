package api

import (
	"net/http"
	"strings"

	"github.com/bc-dunia/mcpdrill/internal/analysis"
)

const maxErrorSignatures = 10

type ErrorSignaturesResponse struct {
	RunID      string                     `json:"run_id"`
	Signatures []analysis.ErrorSignature `json:"signatures"`
}

func (s *Server) handleGetErrorSignatures(w http.ResponseWriter, r *http.Request, runID string) {
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

	errorLogs, err := s.telemetryStore.GetErrorLogs(runID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.writeError(w, http.StatusNotFound, NewNotFoundErrorResponse(runID))
			return
		}
		s.writeError(w, http.StatusInternalServerError, NewInternalErrorResponse(err.Error()))
		return
	}

	signatures := analysis.ExtractSignatures(errorLogs, maxErrorSignatures)

	s.writeJSON(w, http.StatusOK, &ErrorSignaturesResponse{
		RunID:      runID,
		Signatures: signatures,
	})
}
