package api

import (
	"net/http"
	"strings"

	"github.com/bc-dunia/mcpdrill/internal/analysis"
	"github.com/bc-dunia/mcpdrill/internal/metrics"
)

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r.Method, "GET")
		return
	}

	runs := s.runManager.ListRuns()
	s.writeJSON(w, http.StatusOK, &ListRunsResponse{Runs: runs})
}

func (s *Server) handleGetRunMetrics(w http.ResponseWriter, r *http.Request, runID string) {
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

	if s.telemetryStore == nil {
		s.writeJSON(w, http.StatusOK, &RunMetricsResponse{RunID: runID})
		return
	}

	data, err := s.telemetryStore.GetTelemetryData(runID)
	if err != nil {
		s.writeJSON(w, http.StatusOK, &RunMetricsResponse{RunID: runID})
		return
	}

	aggregator := analysis.NewAggregator()
	aggregator.SetTimeRange(data.StartTimeMs, data.EndTimeMs)
	for _, op := range data.Operations {
		aggregator.AddOperation(op)
	}
	metricsData := aggregator.Compute()

	duration := data.EndTimeMs - data.StartTimeMs
	throughput := 0.0
	if duration > 0 {
		throughput = float64(metricsData.TotalOps) / (float64(duration) / 1000.0)
	}

	includeTimeSeries := r.URL.Query().Get("include_time_series") == "true"
	var timeSeries []metrics.MetricsTimePoint
	if includeTimeSeries {
		timeSeries = s.telemetryStore.GetMetricsTimeSeries(runID)
	}

	s.writeJSON(w, http.StatusOK, &RunMetricsResponse{
		RunID:          runID,
		Throughput:     throughput,
		LatencyP50:     float64(metricsData.LatencyP50),
		LatencyP95:     float64(metricsData.LatencyP95),
		LatencyP99:     float64(metricsData.LatencyP99),
		ErrorRate:      metricsData.ErrorRate,
		TotalOps:       int64(metricsData.TotalOps),
		FailedOps:      int64(metricsData.FailureOps),
		DurationMs:     duration,
		ByTool:         metricsData.ByTool,
		TimeSeriesData: timeSeries,
	})
}

func (s *Server) handleGetRunStability(w http.ResponseWriter, r *http.Request, runID string) {
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

	includeEvents := r.URL.Query().Get("include_events") == "true"
	includeTimeSeries := r.URL.Query().Get("include_time_series") == "true"

	if s.telemetryStore == nil {
		s.writeJSON(w, http.StatusOK, &StabilityResponse{
			RunID:          runID,
			StabilityScore: 100.0,
		})
		return
	}

	stabilityMetrics := s.telemetryStore.GetStabilityMetrics(runID, includeEvents, includeTimeSeries)
	if stabilityMetrics == nil {
		s.writeJSON(w, http.StatusOK, &StabilityResponse{
			RunID:          runID,
			StabilityScore: 100.0,
		})
		return
	}

	s.writeJSON(w, http.StatusOK, &StabilityResponse{
		RunID:                runID,
		TotalSessions:        stabilityMetrics.TotalSessions,
		ActiveSessions:       stabilityMetrics.ActiveSessions,
		DroppedSessions:      stabilityMetrics.DroppedSessions,
		TerminatedSessions:   stabilityMetrics.TerminatedSessions,
		AvgSessionLifetimeMs: stabilityMetrics.AvgSessionLifetimeMs,
		ReconnectRate:        stabilityMetrics.ReconnectRate,
		ProtocolErrorRate:    stabilityMetrics.ProtocolErrorRate,
		ConnectionChurnRate:  stabilityMetrics.ConnectionChurnRate,
		StabilityScore:       stabilityMetrics.StabilityScore,
		DropRate:             stabilityMetrics.DropRate,
		Events:               stabilityMetrics.Events,
		SessionMetrics:       stabilityMetrics.SessionMetrics,
		TimeSeriesData:       stabilityMetrics.TimeSeriesData,
	})
}

func (s *Server) handleCompareRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w, r.Method, "GET")
		return
	}

	// Parse path: /runs/{runA}/compare/{runB}
	path := strings.TrimPrefix(r.URL.Path, "/runs/")
	parts := strings.Split(path, "/compare/")
	if len(parts) != 2 {
		s.writeError(w, http.StatusBadRequest, NewInvalidRequestErrorResponse("invalid compare path", nil))
		return
	}
	runIdA := parts[0]
	runIdB := parts[1]

	if s.telemetryStore == nil {
		s.writeJSON(w, http.StatusOK, &CompareRunsResponse{
			RunA: RunMetricsResponse{RunID: runIdA},
			RunB: RunMetricsResponse{RunID: runIdB},
		})
		return
	}

	// Get metrics for both runs
	dataA, errA := s.telemetryStore.GetTelemetryData(runIdA)
	dataB, errB := s.telemetryStore.GetTelemetryData(runIdB)

	if errA != nil {
		s.writeError(w, http.StatusNotFound, NewNotFoundErrorResponse(runIdA))
		return
	}
	if errB != nil {
		s.writeError(w, http.StatusNotFound, NewNotFoundErrorResponse(runIdB))
		return
	}

	// Compute metrics for run A
	aggregatorA := analysis.NewAggregator()
	aggregatorA.SetTimeRange(dataA.StartTimeMs, dataA.EndTimeMs)
	for _, op := range dataA.Operations {
		aggregatorA.AddOperation(op)
	}
	metricsA := aggregatorA.Compute()
	durationA := dataA.EndTimeMs - dataA.StartTimeMs
	throughputA := 0.0
	if durationA > 0 {
		throughputA = float64(metricsA.TotalOps) / (float64(durationA) / 1000.0)
	}

	// Compute metrics for run B
	aggregatorB := analysis.NewAggregator()
	aggregatorB.SetTimeRange(dataB.StartTimeMs, dataB.EndTimeMs)
	for _, op := range dataB.Operations {
		aggregatorB.AddOperation(op)
	}
	metricsB := aggregatorB.Compute()
	durationB := dataB.EndTimeMs - dataB.StartTimeMs
	throughputB := 0.0
	if durationB > 0 {
		throughputB = float64(metricsB.TotalOps) / (float64(durationB) / 1000.0)
	}

	// Build comparison response
	comparison := CompareRunsResponse{
		RunA: RunMetricsResponse{
			RunID:      runIdA,
			TotalOps:   int64(metricsA.TotalOps),
			FailedOps:  int64(metricsA.FailureOps),
			Throughput: throughputA,
			LatencyP50: float64(metricsA.LatencyP50),
			LatencyP95: float64(metricsA.LatencyP95),
			LatencyP99: float64(metricsA.LatencyP99),
			ErrorRate:  metricsA.ErrorRate,
			DurationMs: durationA,
		},
		RunB: RunMetricsResponse{
			RunID:      runIdB,
			TotalOps:   int64(metricsB.TotalOps),
			FailedOps:  int64(metricsB.FailureOps),
			Throughput: throughputB,
			LatencyP50: float64(metricsB.LatencyP50),
			LatencyP95: float64(metricsB.LatencyP95),
			LatencyP99: float64(metricsB.LatencyP99),
			ErrorRate:  metricsB.ErrorRate,
			DurationMs: durationB,
		},
	}

	s.writeJSON(w, http.StatusOK, comparison)
}
