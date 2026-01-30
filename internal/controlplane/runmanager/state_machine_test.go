package runmanager

import "testing"

type transition struct {
	from RunState
	to   RunState
}

func TestCanTransitionValid(t *testing.T) {
	valid := []transition{
		{RunStateCreated, RunStatePreflightRunning},
		{RunStateCreated, RunStateFailed},
		{RunStateCreated, RunStateAborted},
		{RunStatePreflightRunning, RunStatePreflightPassed},
		{RunStatePreflightRunning, RunStatePreflightFailed},
		{RunStatePreflightRunning, RunStateStopping},
		{RunStatePreflightFailed, RunStateStopping},
		{RunStatePreflightPassed, RunStateBaselineRunning},
		{RunStatePreflightPassed, RunStateStopping},
		{RunStateBaselineRunning, RunStateRampRunning},
		{RunStateBaselineRunning, RunStateStopping},
		{RunStateRampRunning, RunStateSoakRunning},
		{RunStateRampRunning, RunStateStopping},
		{RunStateSoakRunning, RunStateStopping},
		{RunStateStopping, RunStateAnalyzing},
		{RunStateStopping, RunStateStopping},
		{RunStateAnalyzing, RunStateCompleted},
		{RunStateAnalyzing, RunStateFailed},
		{RunStateAnalyzing, RunStateAborted},
	}

	for _, tc := range valid {
		if !CanTransition(tc.from, tc.to) {
			t.Fatalf("expected transition allowed: %s -> %s", tc.from, tc.to)
		}
	}
}

func TestCanTransitionInvalid(t *testing.T) {
	valid := map[transition]struct{}{
		{RunStateCreated, RunStatePreflightRunning}:         {},
		{RunStateCreated, RunStateFailed}:                   {},
		{RunStateCreated, RunStateAborted}:                  {},
		{RunStatePreflightRunning, RunStatePreflightPassed}: {},
		{RunStatePreflightRunning, RunStatePreflightFailed}: {},
		{RunStatePreflightRunning, RunStateStopping}:        {},
		{RunStatePreflightFailed, RunStateStopping}:         {},
		{RunStatePreflightPassed, RunStateBaselineRunning}:  {},
		{RunStatePreflightPassed, RunStateStopping}:         {},
		{RunStateBaselineRunning, RunStateRampRunning}:      {},
		{RunStateBaselineRunning, RunStateStopping}:         {},
		{RunStateRampRunning, RunStateSoakRunning}:          {},
		{RunStateRampRunning, RunStateStopping}:             {},
		{RunStateSoakRunning, RunStateStopping}:             {},
		{RunStateStopping, RunStateAnalyzing}:               {},
		{RunStateStopping, RunStateStopping}:                {},
		{RunStateAnalyzing, RunStateCompleted}:              {},
		{RunStateAnalyzing, RunStateFailed}:                 {},
		{RunStateAnalyzing, RunStateAborted}:                {},
	}

	allStates := []RunState{
		RunStateCreated,
		RunStatePreflightRunning,
		RunStatePreflightPassed,
		RunStatePreflightFailed,
		RunStateBaselineRunning,
		RunStateRampRunning,
		RunStateSoakRunning,
		RunStateStopping,
		RunStateAnalyzing,
		RunStateCompleted,
		RunStateFailed,
		RunStateAborted,
	}

	for _, from := range allStates {
		for _, to := range allStates {
			pair := transition{from, to}
			_, isValid := valid[pair]
			if isValid {
				continue
			}
			if CanTransition(from, to) {
				t.Fatalf("expected transition denied: %s -> %s", from, to)
			}
		}
	}

	unknown := RunState("unknown")
	for _, to := range allStates {
		if CanTransition(unknown, to) {
			t.Fatalf("expected transition denied: %s -> %s", unknown, to)
		}
	}
	if CanTransition(unknown, unknown) {
		t.Fatalf("expected transition denied: %s -> %s", unknown, unknown)
	}
}
