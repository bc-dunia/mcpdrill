package vu

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/events"
	"github.com/bc-dunia/mcpdrill/internal/otel"
	"github.com/bc-dunia/mcpdrill/internal/plugin"
	"github.com/bc-dunia/mcpdrill/internal/session"
	"github.com/bc-dunia/mcpdrill/internal/transport"
	"go.opentelemetry.io/otel/attribute"
)

type VUExecutor struct {
	vu               *VUInstance
	config           *VUConfig
	sampler          *OperationSampler
	thinkTimeSampler *ThinkTimeSampler
	rateLimiter      *RateLimiter
	inFlightLimiter  *InFlightLimiter
	metrics          *VUMetrics
	resultChan       chan<- *OperationResult
	tracer           *otel.Tracer
	userJourney      *UserJourneyExecutor
	sessionMode      session.SessionMode
	wg               sync.WaitGroup
}

func NewVUExecutor(
	vu *VUInstance,
	config *VUConfig,
	sampler *OperationSampler,
	rateLimiter *RateLimiter,
	metrics *VUMetrics,
	resultChan chan<- *OperationResult,
) *VUExecutor {
	mode := session.ModeReuse
	if config != nil && config.SessionManager != nil {
		mode = config.SessionManager.Mode()
	}
	return &VUExecutor{
		vu:               vu,
		config:           config,
		sampler:          sampler,
		thinkTimeSampler: NewThinkTimeSampler(config.ThinkTime, vu.RNGSeed+1),
		rateLimiter:      rateLimiter,
		inFlightLimiter:  NewInFlightLimiter(config.InFlightPerVU),
		metrics:          metrics,
		resultChan:       resultChan,
		tracer:           otel.GetGlobalTracer(),
		userJourney:      NewUserJourneyExecutor(config.UserJourney, vu.RNGSeed+2),
		sessionMode:      mode,
	}
}

func (e *VUExecutor) Run(ctx context.Context) {
	e.vu.SetState(StateInitializing)
	e.vu.StartedAt = time.Now()

	var reuseSess *session.SessionInfo
	if e.sessionMode == session.ModeReuse {
		var err error
		reuseSess, err = e.acquireSessionWithRetry(ctx)
		if err != nil {
			log.Printf("VU %s: session acquire failed: %v", e.vu.ID, err)
			e.vu.SetState(StateStopped)
			e.vu.StoppedAt = time.Now()
			return
		}
		e.vu.SetSession(reuseSess)
	}

	startupSess := reuseSess
	if e.sessionMode != session.ModeReuse {
		var err error
		startupSess, err = e.acquireSessionWithRetry(ctx)
		if err != nil {
			log.Printf("VU %s: session acquire failed: %v", e.vu.ID, err)
			e.vu.SetState(StateStopped)
			e.vu.StoppedAt = time.Now()
			return
		}
	}

	if outcome, err := e.userJourney.RunStartupSequence(ctx, startupSess); err != nil || (outcome != nil && !outcome.OK) {
		log.Printf("VU %s: startup sequence failed: %v", e.vu.ID, err)
	}
	if e.sessionMode != session.ModeReuse {
		e.releaseSession(ctx, startupSess)
	}

	e.vu.SetState(StateRunning)
	e.metrics.ActiveVUs.Add(1)
	defer func() {
		e.metrics.ActiveVUs.Add(-1)
		e.vu.SetState(StateStopped)
		e.vu.StoppedAt = time.Now()
	}()

	for {
		select {
		case <-ctx.Done():
			e.vu.SetState(StateDraining)
			e.wg.Wait()
			if e.sessionMode == session.ModeReuse {
				e.releaseSession(ctx, reuseSess)
			}
			return
		default:
		}

		if e.vu.State() == StateDraining {
			e.wg.Wait()
			if e.sessionMode == session.ModeReuse {
				e.releaseSession(ctx, reuseSess)
			}
			return
		}

		if e.sessionMode == session.ModeReuse && reuseSess != nil {
			if reuseSess.IsExpired() || reuseSess.GetState() == session.StateClosed || reuseSess.GetState() == session.StateExpired {
				e.wg.Wait()
				e.invalidateReuseSession(ctx, reuseSess)
				newSess, err := e.acquireSessionWithRetry(ctx)
				if err != nil {
					log.Printf("VU %s: session reacquire failed: %v", e.vu.ID, err)
					return
				}
				reuseSess = newSess
				e.vu.SetSession(reuseSess)
			}
		}

		if e.userJourney.ShouldRunPeriodicToolsList() || e.userJourney.ShouldRunToolsListAfterErrors() {
			if e.sessionMode == session.ModeReuse {
				if outcome, err := e.userJourney.RunPeriodicToolsList(ctx, reuseSess); err == nil && outcome != nil && outcome.OK {
					e.emitPeriodicToolsListResult(reuseSess, outcome)
				}
			} else {
				periodicAcquireStart := time.Now()
				periodicSess, err := e.acquireSessionWithRetry(ctx)
				periodicAcquireEnd := time.Now()
				if err != nil {
					if e.shouldEmitSessionAcquireError(err) {
						e.emitSessionAcquireError(OpToolsList, "", err, periodicAcquireStart, periodicAcquireEnd)
					}
				} else {
					if outcome, err := e.userJourney.RunPeriodicToolsList(ctx, periodicSess); err == nil && outcome != nil && outcome.OK {
						e.emitPeriodicToolsListResult(periodicSess, outcome)
					}
					e.releaseSession(ctx, periodicSess)
				}
			}
		}

		if e.rateLimiter != nil && e.rateLimiter.Enabled() {
			if err := e.rateLimiter.Acquire(ctx); err != nil {
				continue
			}
		}

		if err := e.inFlightLimiter.Acquire(ctx); err != nil {
			continue
		}

		op := e.sampler.Sample()

		currentSess := reuseSess
		e.wg.Add(1)
		go func(op *OperationWeight, sess *session.SessionInfo) {
			defer e.wg.Done()
			defer e.inFlightLimiter.Release()

			e.updateMaxInFlight()
			opSess := sess
			shouldRelease := false
			if e.sessionMode != session.ModeReuse {
				var err error
				acquireStart := time.Now()
				opSess, err = e.acquireSessionWithRetry(ctx)
				acquireEnd := time.Now()
				if err != nil {
					if e.shouldEmitSessionAcquireError(err) {
						e.emitSessionAcquireError(op.Operation, op.ToolName, err, acquireStart, acquireEnd)
					}
					return
				}
				shouldRelease = true
			}
			if shouldRelease {
				defer e.releaseSession(ctx, opSess)
			}
			e.executeOperation(ctx, opSess, op)
		}(op, currentSess)

		thinkTime := e.thinkTimeSampler.Sample()
		if thinkTime > 0 {
			e.metrics.ThinkTimeTotal.Add(thinkTime)
			select {
			case <-ctx.Done():
			case <-time.After(time.Duration(thinkTime) * time.Millisecond):
			}
		}
	}
}

func (e *VUExecutor) Stop() {
	e.vu.SetState(StateDraining)
}

func (e *VUExecutor) Wait() {
	e.wg.Wait()
}

func (e *VUExecutor) SetTracer(t *otel.Tracer) {
	e.tracer = t
}

func (e *VUExecutor) acquireSession(ctx context.Context) (*session.SessionInfo, error) {
	e.metrics.SessionAcquires.Add(1)

	sess, err := e.config.SessionManager.Acquire(ctx, e.vu.ID)
	if err != nil {
		e.metrics.SessionErrors.Add(1)
		e.metrics.ReconnectAttempts.Add(1)
		return nil, err
	}

	e.metrics.SessionsCreated.Add(1)
	e.metrics.ActiveSessions.Add(1)

	// Record session creation in OTel metrics
	if m := otel.GetGlobalMetrics(); m != nil {
		m.IncrementSessions(ctx)
	}

	return sess, nil
}

func (e *VUExecutor) acquireSessionWithRetry(ctx context.Context) (*session.SessionInfo, error) {
	sess, err := e.acquireSession(ctx)
	if err == nil {
		e.userJourney.ResetRetryState()
		return sess, nil
	}

	for e.userJourney.ShouldRetryReconnect() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		delay := e.userJourney.GetReconnectDelay()
		e.metrics.ReconnectAttempts.Add(1)
		attempt := e.userJourney.RetryAttempts()

		// Record reconnect attempt in OTel metrics
		if m := otel.GetGlobalMetrics(); m != nil {
			m.RecordReconnect(ctx)
		}

		// Log reconnect event
		if l := events.GetGlobalEventLogger(); l != nil {
			l.LogReconnect(e.vu.ID, attempt, "session_acquire_failed", delay.Milliseconds())
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}

		sess, err = e.acquireSession(ctx)
		if err == nil {
			e.userJourney.ResetRetryState()
			return sess, nil
		}
	}

	return nil, err
}

func (e *VUExecutor) shouldEmitSessionAcquireError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

func (e *VUExecutor) emitSessionAcquireError(op OperationType, toolName string, err error, startTime, endTime time.Time) {

	e.metrics.TotalOperations.Add(1)
	e.metrics.InFlightOperations.Add(1)
	defer e.metrics.InFlightOperations.Add(-1)

	e.metrics.FailedOperations.Add(1)
	e.vu.OperationsFailed.Add(1)
	e.userJourney.RecordOperationResult(false)

	errorType := transport.ErrorTypeConnect
	errorCode := transport.ErrorCode("SESSION_ACQUIRE_FAILED")
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		errorType = transport.ErrorTypeCancelled
		errorCode = transport.CodeCancelled
	}

	acquireOutcome := &transport.OperationOutcome{
		Operation: transport.OperationType(op),
		ToolName:  toolName,
		StartTime: startTime,
		LatencyMs: endTime.Sub(startTime).Milliseconds(),
		OK:        false,
		Error: &transport.OperationError{
			Type:    errorType,
			Code:    errorCode,
			Message: "session acquire failed: " + err.Error(),
		},
	}

	if e.resultChan != nil {
		result := &OperationResult{
			Operation: op,
			ToolName:  toolName,
			Outcome:   acquireOutcome,
			VUID:      e.vu.ID,
			StartTime: startTime,
			EndTime:   endTime,
		}
		select {
		case e.resultChan <- result:
		default:
			e.metrics.DroppedResults.Add(1)
		}
	}
}

func (e *VUExecutor) emitPeriodicToolsListResult(sess *session.SessionInfo, outcome *transport.OperationOutcome) {
	if e.resultChan == nil {
		return
	}

	result := &OperationResult{
		Operation: OpToolsList,
		Outcome:   outcome,
		VUID:      e.vu.ID,
		SessionID: sess.ID,
		StartTime: outcome.StartTime,
		EndTime:   time.Now(),
	}

	select {
	case e.resultChan <- result:
	default:
		e.metrics.DroppedResults.Add(1)
	}
}

func (e *VUExecutor) invalidateReuseSession(ctx context.Context, sess *session.SessionInfo) {
	if sess == nil {
		return
	}

	e.metrics.SessionReleases.Add(1)
	e.metrics.SessionsDestroyed.Add(1)
	e.metrics.ActiveSessions.Add(-1)

	if m := otel.GetGlobalMetrics(); m != nil {
		m.DecrementSessions(ctx)
	}

	e.config.SessionManager.Invalidate(ctx, sess)
}

func (e *VUExecutor) releaseSession(ctx context.Context, sess *session.SessionInfo) {
	if sess == nil {
		return
	}

	e.metrics.SessionReleases.Add(1)
	e.metrics.SessionsDestroyed.Add(1)
	e.metrics.ActiveSessions.Add(-1)

	// Record session destruction in OTel metrics
	if m := otel.GetGlobalMetrics(); m != nil {
		m.DecrementSessions(ctx)
	}

	if err := e.config.SessionManager.Release(ctx, sess); err != nil {
		e.metrics.SessionErrors.Add(1)
	}
}

func (e *VUExecutor) executeOperation(ctx context.Context, sess *session.SessionInfo, op *OperationWeight) {
	e.metrics.TotalOperations.Add(1)
	e.metrics.InFlightOperations.Add(1)
	defer e.metrics.InFlightOperations.Add(-1)

	spanOpts := otel.OperationSpanOptions{
		RunID:     e.config.RunID,
		StageID:   e.config.StageID,
		WorkerID:  e.config.WorkerID,
		VUID:      e.vu.ID,
		SessionID: sess.ID,
		Operation: string(op.Operation),
		ToolName:  op.ToolName,
	}
	ctx, span := e.tracer.StartOperationSpan(ctx, spanOpts)
	defer span.End()

	startTime := time.Now()

	var outcome *transport.OperationOutcome
	var err error

	conn := sess.Connection
	if conn == nil {
		e.metrics.FailedOperations.Add(1)
		e.vu.OperationsFailed.Add(1)
		span.SetAttributes(attribute.Bool("error", true))
		span.SetAttributes(attribute.String("error.type", "no_connection"))

		endTime := time.Now()
		traceID, spanID := otel.GetTraceInfo(ctx)
		noConnOutcome := &transport.OperationOutcome{
			Operation: transport.OperationType(op.Operation),
			ToolName:  op.ToolName,
			StartTime: startTime,
			LatencyMs: endTime.Sub(startTime).Milliseconds(),
			OK:        false,
			Error: &transport.OperationError{
				Type:    transport.ErrorTypeConnect,
				Code:    "NO_CONNECTION",
				Message: "no connection available for session",
			},
		}
		if e.resultChan != nil {
			result := &OperationResult{
				Operation: op.Operation,
				ToolName:  op.ToolName,
				Outcome:   noConnOutcome,
				VUID:      e.vu.ID,
				SessionID: sess.ID,
				StartTime: startTime,
				EndTime:   endTime,
				TraceID:   traceID,
				SpanID:    spanID,
			}
			select {
			case e.resultChan <- result:
			default:
				e.metrics.DroppedResults.Add(1)
			}
		}
		return
	}

	registeredOp, found := plugin.Get(string(op.Operation))
	if !found {
		e.metrics.FailedOperations.Add(1)
		e.vu.OperationsFailed.Add(1)
		span.SetAttributes(attribute.Bool("error", true))
		span.SetAttributes(attribute.String("error.type", "unknown_operation"))

		endTime := time.Now()
		traceID, spanID := otel.GetTraceInfo(ctx)
		unknownOpOutcome := &transport.OperationOutcome{
			Operation: transport.OperationType(op.Operation),
			ToolName:  op.ToolName,
			StartTime: startTime,
			LatencyMs: endTime.Sub(startTime).Milliseconds(),
			OK:        false,
			Error: &transport.OperationError{
				Type:    transport.ErrorTypeProtocol,
				Code:    "UNKNOWN_OPERATION",
				Message: "operation not registered: " + string(op.Operation),
			},
		}
		if e.resultChan != nil {
			result := &OperationResult{
				Operation: op.Operation,
				ToolName:  op.ToolName,
				Outcome:   unknownOpOutcome,
				VUID:      e.vu.ID,
				SessionID: sess.ID,
				StartTime: startTime,
				EndTime:   endTime,
				TraceID:   traceID,
				SpanID:    spanID,
			}
			select {
			case e.resultChan <- result:
			default:
				e.metrics.DroppedResults.Add(1)
			}
		}
		return
	}

	params := buildOperationParams(op)

	var toolMetrics *ToolCallMetrics
	if op.Operation == OpToolsCall {
		toolMetrics = &ToolCallMetrics{
			ToolName:      op.ToolName,
			ArgumentSize:  calculateArgumentSize(op.Arguments),
			ArgumentDepth: calculateArgumentDepth(op.Arguments),
		}
	}

	if validationErr := registeredOp.Validate(params); validationErr != nil {
		e.metrics.FailedOperations.Add(1)
		e.vu.OperationsFailed.Add(1)
		e.userJourney.RecordOperationResult(false)
		span.SetAttributes(attribute.Bool("error", true))
		span.SetAttributes(attribute.String("error.type", "validation_error"))
		if toolMetrics != nil {
			toolMetrics.ParseError = true
		}

		endTime := time.Now()
		traceID, spanID := otel.GetTraceInfo(ctx)

		validationOutcome := &transport.OperationOutcome{
			Operation: transport.OperationType(op.Operation),
			ToolName:  op.ToolName,
			StartTime: startTime,
			LatencyMs: endTime.Sub(startTime).Milliseconds(),
			OK:        false,
			Error: &transport.OperationError{
				Type:    transport.ErrorTypeProtocol,
				Code:    "VALIDATION_ERROR",
				Message: validationErr.Error(),
			},
		}

		if e.resultChan != nil {
			result := &OperationResult{
				Operation:   op.Operation,
				ToolName:    op.ToolName,
				Outcome:     validationOutcome,
				VUID:        e.vu.ID,
				SessionID:   sess.ID,
				StartTime:   startTime,
				EndTime:     endTime,
				TraceID:     traceID,
				SpanID:      spanID,
				ToolMetrics: toolMetrics,
			}

			select {
			case e.resultChan <- result:
			default:
				e.metrics.DroppedResults.Add(1)
			}
		}
		return
	}

	outcome, err = registeredOp.Execute(ctx, conn, params)

	endTime := time.Now()

	if outcome == nil && err == nil {
		err = errors.New("plugin returned nil outcome without error")
	}

	if err != nil || (outcome != nil && !outcome.OK) {
		e.metrics.FailedOperations.Add(1)
		e.vu.OperationsFailed.Add(1)
		e.userJourney.RecordOperationResult(false)

		if toolMetrics != nil {
			toolMetrics.ExecutionError = true
		}

		if outcome != nil && outcome.Error != nil {
			otel.RecordError(span, err, string(outcome.Error.Type), false)
			if outcome.Error.Type == transport.ErrorTypeRateLimited {
				e.metrics.RateLimitedOperations.Add(1)
			}
		} else if err != nil {
			otel.RecordError(span, err, "internal", false)
		}
	} else {
		e.metrics.SuccessfulOperations.Add(1)
		e.vu.OperationsCompleted.Add(1)
		e.userJourney.RecordOperationResult(true)
		span.SetAttributes(attribute.Bool("ok", true))
	}

	if toolMetrics != nil && outcome != nil {
		toolMetrics.ResultSize = int(outcome.BytesIn)
	}

	if outcome != nil {
		span.SetAttributes(
			attribute.Int64("latency_ms", outcome.LatencyMs),
			attribute.Int64("bytes_in", outcome.BytesIn),
			attribute.Int64("bytes_out", outcome.BytesOut),
		)
		if outcome.HTTPStatus != nil {
			span.SetAttributes(attribute.Int("http.status_code", *outcome.HTTPStatus))
		}
	}

	if mgr, ok := e.config.SessionManager.(*session.Manager); ok {
		sess.Touch(mgr.Config().MaxIdleMs)
	}

	traceID, spanID := otel.GetTraceInfo(ctx)

	// Record operation latency and errors in OTel metrics
	if m := otel.GetGlobalMetrics(); m != nil {
		if outcome != nil {
			m.RecordOperationLatency(ctx, string(op.Operation), op.ToolName, float64(outcome.LatencyMs), outcome.OK)
			if !outcome.OK && outcome.Error != nil {
				m.RecordError(ctx, string(outcome.Error.Type))
			}
		}
	}

	if e.resultChan != nil {
		result := &OperationResult{
			Operation:   op.Operation,
			ToolName:    op.ToolName,
			Outcome:     outcome,
			VUID:        e.vu.ID,
			SessionID:   sess.ID,
			StartTime:   startTime,
			EndTime:     endTime,
			TraceID:     traceID,
			SpanID:      spanID,
			ToolMetrics: toolMetrics,
		}

		select {
		case e.resultChan <- result:
		default:
			e.metrics.DroppedResults.Add(1)
		}
	}
}

func (e *VUExecutor) updateMaxInFlight() {
	current := int64(e.inFlightLimiter.Current())
	for {
		max := e.metrics.MaxInFlightReached.Load()
		if current <= max {
			break
		}
		if e.metrics.MaxInFlightReached.CompareAndSwap(max, current) {
			break
		}
	}
}

func buildOperationParams(op *OperationWeight) map[string]interface{} {
	params := make(map[string]interface{})

	switch op.Operation {
	case OpToolsCall:
		params["name"] = op.ToolName
		if len(op.Arguments) > 0 {
			params["arguments"] = op.Arguments
		}

	case OpResourcesRead:
		params["uri"] = op.URI

	case OpPromptsGet:
		params["name"] = op.PromptName
		if len(op.Arguments) > 0 {
			params["arguments"] = op.Arguments
		}
	}

	if len(params) == 0 {
		return nil
	}

	return params
}

func calculateArgumentSize(args map[string]interface{}) int {
	if len(args) == 0 {
		return 0
	}
	data, err := json.Marshal(args)
	if err != nil {
		return 0
	}
	return len(data)
}

func calculateArgumentDepth(v interface{}) int {
	switch val := v.(type) {
	case map[string]interface{}:
		if len(val) == 0 {
			return 1
		}
		maxChild := 0
		for _, child := range val {
			depth := calculateArgumentDepth(child)
			if depth > maxChild {
				maxChild = depth
			}
		}
		return 1 + maxChild
	case []interface{}:
		if len(val) == 0 {
			return 1
		}
		maxChild := 0
		for _, child := range val {
			depth := calculateArgumentDepth(child)
			if depth > maxChild {
				maxChild = depth
			}
		}
		return 1 + maxChild
	default:
		return 0
	}
}
