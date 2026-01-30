package runmanager

var allowedTransitions = map[RunState]map[RunState]struct{}{
	RunStateCreated: {
		RunStatePreflightRunning: {},
		RunStateFailed:           {},
		RunStateAborted:          {},
	},
	RunStatePreflightRunning: {
		RunStatePreflightPassed: {},
		RunStatePreflightFailed: {},
		RunStateStopping:        {},
	},
	RunStatePreflightFailed: {
		RunStateStopping: {},
	},
	RunStatePreflightPassed: {
		RunStateBaselineRunning: {},
		RunStateStopping:        {},
	},
	RunStateBaselineRunning: {
		RunStateRampRunning: {},
		RunStateStopping:    {},
	},
	RunStateRampRunning: {
		RunStateSoakRunning: {},
		RunStateStopping:    {},
	},
	RunStateSoakRunning: {
		RunStateStopping: {},
	},
	RunStateStopping: {
		RunStateAnalyzing: {},
		RunStateStopping:  {},
	},
	RunStateAnalyzing: {
		RunStateCompleted: {},
		RunStateFailed:    {},
		RunStateAborted:   {},
	},
}

// CanTransition reports whether a state transition is valid.
func CanTransition(from, to RunState) bool {
	allowed, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	_, ok = allowed[to]
	return ok
}
