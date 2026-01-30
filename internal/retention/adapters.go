package retention

import "github.com/bc-dunia/mcpdrill/internal/controlplane/api"

type TelemetryStoreAdapter struct {
	store *api.TelemetryStore
}

func NewTelemetryStoreAdapter(store *api.TelemetryStore) *TelemetryStoreAdapter {
	return &TelemetryStoreAdapter{store: store}
}

func (a *TelemetryStoreAdapter) ListRunsForRetention() []RunRetentionInfo {
	runs := a.store.ListRunsForRetention()
	result := make([]RunRetentionInfo, len(runs))
	for i, r := range runs {
		result[i] = RunRetentionInfo{
			RunID:     r.RunID,
			EndTimeMs: r.EndTimeMs,
		}
	}
	return result
}

func (a *TelemetryStoreAdapter) DeleteRun(runID string) {
	a.store.DeleteRun(runID)
}
