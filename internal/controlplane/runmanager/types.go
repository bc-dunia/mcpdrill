package runmanager

// RunState represents the lifecycle state for a run.
type RunState string

const (
	RunStateCreated          RunState = "created"
	RunStatePreflightRunning RunState = "preflight_running"
	RunStatePreflightPassed  RunState = "preflight_passed"
	RunStatePreflightFailed  RunState = "preflight_failed"
	RunStateBaselineRunning  RunState = "baseline_running"
	RunStateRampRunning      RunState = "ramp_running"
	RunStateSoakRunning      RunState = "soak_running"
	RunStateStopping         RunState = "stopping"
	RunStateAnalyzing        RunState = "analyzing"
	RunStateCompleted        RunState = "completed"
	RunStateFailed           RunState = "failed"
	RunStateAborted          RunState = "aborted"
)
