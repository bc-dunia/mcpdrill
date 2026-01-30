package runmanager

import (
	"encoding/json"
	"testing"
)

func TestParseRunConfig_StreamingStopConditions(t *testing.T) {
	configJSON := `{
		"scenario_id": "test",
		"target": {"kind": "server", "url": "http://localhost:3000", "transport": "streamable_http"},
		"stages": [{
			"stage_id": "stg_0000000000000002",
			"stage": "baseline",
			"enabled": true,
			"duration_ms": 10000,
			"load": {"target_vus": 1},
			"streaming_stop_conditions": {
				"stream_stall_seconds": 5,
				"min_events_per_second": 10.5
			}
		}]
	}`

	var cfg parsedRunConfig
	err := json.Unmarshal([]byte(configJSON), &cfg)
	if err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	if len(cfg.Stages) != 1 {
		t.Fatalf("Expected 1 stage, got %d", len(cfg.Stages))
	}

	stage := cfg.Stages[0]
	if stage.StreamingStopConfig == nil {
		t.Fatal("StreamingStopConfig is nil")
	}

	if stage.StreamingStopConfig.StreamStallSeconds != 5 {
		t.Errorf("Expected StreamStallSeconds=5, got %d", stage.StreamingStopConfig.StreamStallSeconds)
	}

	if stage.StreamingStopConfig.MinEventsPerSecond != 10.5 {
		t.Errorf("Expected MinEventsPerSecond=10.5, got %f", stage.StreamingStopConfig.MinEventsPerSecond)
	}
}

func TestParseRunConfig_StreamingStopConditions_OnlyStall(t *testing.T) {
	configJSON := `{
		"scenario_id": "test",
		"target": {"kind": "server", "url": "http://localhost:3000", "transport": "streamable_http"},
		"stages": [{
			"stage_id": "stg_0000000000000002",
			"stage": "baseline",
			"enabled": true,
			"duration_ms": 10000,
			"load": {"target_vus": 1},
			"streaming_stop_conditions": {
				"stream_stall_seconds": 30
			}
		}]
	}`

	var cfg parsedRunConfig
	err := json.Unmarshal([]byte(configJSON), &cfg)
	if err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	stage := cfg.Stages[0]
	if stage.StreamingStopConfig == nil {
		t.Fatal("StreamingStopConfig is nil")
	}

	if stage.StreamingStopConfig.StreamStallSeconds != 30 {
		t.Errorf("Expected StreamStallSeconds=30, got %d", stage.StreamingStopConfig.StreamStallSeconds)
	}

	if stage.StreamingStopConfig.MinEventsPerSecond != 0 {
		t.Errorf("Expected MinEventsPerSecond=0 (default), got %f", stage.StreamingStopConfig.MinEventsPerSecond)
	}
}

func TestParseRunConfig_StreamingStopConditions_OnlyMinEvents(t *testing.T) {
	configJSON := `{
		"scenario_id": "test",
		"target": {"kind": "server", "url": "http://localhost:3000", "transport": "streamable_http"},
		"stages": [{
			"stage_id": "stg_0000000000000002",
			"stage": "baseline",
			"enabled": true,
			"duration_ms": 10000,
			"load": {"target_vus": 1},
			"streaming_stop_conditions": {
				"min_events_per_second": 5.0
			}
		}]
	}`

	var cfg parsedRunConfig
	err := json.Unmarshal([]byte(configJSON), &cfg)
	if err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	stage := cfg.Stages[0]
	if stage.StreamingStopConfig == nil {
		t.Fatal("StreamingStopConfig is nil")
	}

	if stage.StreamingStopConfig.StreamStallSeconds != 0 {
		t.Errorf("Expected StreamStallSeconds=0 (default), got %d", stage.StreamingStopConfig.StreamStallSeconds)
	}

	if stage.StreamingStopConfig.MinEventsPerSecond != 5.0 {
		t.Errorf("Expected MinEventsPerSecond=5.0, got %f", stage.StreamingStopConfig.MinEventsPerSecond)
	}
}

func TestParseRunConfig_NoStreamingStopConditions(t *testing.T) {
	configJSON := `{
		"scenario_id": "test",
		"target": {"kind": "server", "url": "http://localhost:3000", "transport": "streamable_http"},
		"stages": [{
			"stage_id": "stg_0000000000000002",
			"stage": "baseline",
			"enabled": true,
			"duration_ms": 10000,
			"load": {"target_vus": 1}
		}]
	}`

	var cfg parsedRunConfig
	err := json.Unmarshal([]byte(configJSON), &cfg)
	if err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	stage := cfg.Stages[0]
	if stage.StreamingStopConfig != nil {
		t.Errorf("Expected StreamingStopConfig to be nil, got %+v", stage.StreamingStopConfig)
	}
}

func TestParseRunConfig_StreamingStopConditions_MultipleStages(t *testing.T) {
	configJSON := `{
		"scenario_id": "test",
		"target": {"kind": "server", "url": "http://localhost:3000", "transport": "streamable_http"},
		"stages": [
			{
				"stage_id": "stg_0000000000000001",
				"stage": "preflight",
				"enabled": true,
				"duration_ms": 5000,
				"load": {"target_vus": 1}
			},
			{
				"stage_id": "stg_0000000000000002",
				"stage": "baseline",
				"enabled": true,
				"duration_ms": 10000,
				"load": {"target_vus": 5},
				"streaming_stop_conditions": {
					"stream_stall_seconds": 10,
					"min_events_per_second": 5.0
				}
			},
			{
				"stage_id": "stg_0000000000000003",
				"stage": "ramp",
				"enabled": true,
				"duration_ms": 30000,
				"load": {"target_vus": 20},
				"streaming_stop_conditions": {
					"stream_stall_seconds": 15,
					"min_events_per_second": 10.0
				}
			}
		]
	}`

	var cfg parsedRunConfig
	err := json.Unmarshal([]byte(configJSON), &cfg)
	if err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	if len(cfg.Stages) != 3 {
		t.Fatalf("Expected 3 stages, got %d", len(cfg.Stages))
	}

	// Preflight has no streaming config
	if cfg.Stages[0].StreamingStopConfig != nil {
		t.Errorf("Expected preflight StreamingStopConfig to be nil")
	}

	// Baseline has streaming config
	if cfg.Stages[1].StreamingStopConfig == nil {
		t.Fatal("Expected baseline StreamingStopConfig to be set")
	}
	if cfg.Stages[1].StreamingStopConfig.StreamStallSeconds != 10 {
		t.Errorf("Expected baseline StreamStallSeconds=10, got %d", cfg.Stages[1].StreamingStopConfig.StreamStallSeconds)
	}
	if cfg.Stages[1].StreamingStopConfig.MinEventsPerSecond != 5.0 {
		t.Errorf("Expected baseline MinEventsPerSecond=5.0, got %f", cfg.Stages[1].StreamingStopConfig.MinEventsPerSecond)
	}

	// Ramp has streaming config
	if cfg.Stages[2].StreamingStopConfig == nil {
		t.Fatal("Expected ramp StreamingStopConfig to be set")
	}
	if cfg.Stages[2].StreamingStopConfig.StreamStallSeconds != 15 {
		t.Errorf("Expected ramp StreamStallSeconds=15, got %d", cfg.Stages[2].StreamingStopConfig.StreamStallSeconds)
	}
	if cfg.Stages[2].StreamingStopConfig.MinEventsPerSecond != 10.0 {
		t.Errorf("Expected ramp MinEventsPerSecond=10.0, got %f", cfg.Stages[2].StreamingStopConfig.MinEventsPerSecond)
	}
}

func TestParseRunConfig_StreamingStopConditions_ZeroValues(t *testing.T) {
	configJSON := `{
		"scenario_id": "test",
		"target": {"kind": "server", "url": "http://localhost:3000", "transport": "streamable_http"},
		"stages": [{
			"stage_id": "stg_0000000000000002",
			"stage": "baseline",
			"enabled": true,
			"duration_ms": 10000,
			"load": {"target_vus": 1},
			"streaming_stop_conditions": {
				"stream_stall_seconds": 0,
				"min_events_per_second": 0
			}
		}]
	}`

	var cfg parsedRunConfig
	err := json.Unmarshal([]byte(configJSON), &cfg)
	if err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	stage := cfg.Stages[0]
	if stage.StreamingStopConfig == nil {
		t.Fatal("StreamingStopConfig is nil")
	}

	// Zero values should be parsed correctly (disabled conditions)
	if stage.StreamingStopConfig.StreamStallSeconds != 0 {
		t.Errorf("Expected StreamStallSeconds=0, got %d", stage.StreamingStopConfig.StreamStallSeconds)
	}

	if stage.StreamingStopConfig.MinEventsPerSecond != 0 {
		t.Errorf("Expected MinEventsPerSecond=0, got %f", stage.StreamingStopConfig.MinEventsPerSecond)
	}
}

func TestParseRunConfig_StreamingStopConditions_FloatPrecision(t *testing.T) {
	configJSON := `{
		"scenario_id": "test",
		"target": {"kind": "server", "url": "http://localhost:3000", "transport": "streamable_http"},
		"stages": [{
			"stage_id": "stg_0000000000000002",
			"stage": "baseline",
			"enabled": true,
			"duration_ms": 10000,
			"load": {"target_vus": 1},
			"streaming_stop_conditions": {
				"min_events_per_second": 0.001
			}
		}]
	}`

	var cfg parsedRunConfig
	err := json.Unmarshal([]byte(configJSON), &cfg)
	if err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	stage := cfg.Stages[0]
	if stage.StreamingStopConfig == nil {
		t.Fatal("StreamingStopConfig is nil")
	}

	// Check float precision
	if stage.StreamingStopConfig.MinEventsPerSecond < 0.0009 || stage.StreamingStopConfig.MinEventsPerSecond > 0.0011 {
		t.Errorf("Expected MinEventsPerSecond~=0.001, got %f", stage.StreamingStopConfig.MinEventsPerSecond)
	}
}

func TestFindStageByName_WithStreamingConfig(t *testing.T) {
	configJSON := `{
		"scenario_id": "test",
		"target": {"kind": "server", "url": "http://localhost:3000", "transport": "streamable_http"},
		"stages": [
			{
				"stage_id": "stg_0000000000000001",
				"stage": "preflight",
				"enabled": true,
				"duration_ms": 5000,
				"load": {"target_vus": 1}
			},
			{
				"stage_id": "stg_0000000000000002",
				"stage": "baseline",
				"enabled": true,
				"duration_ms": 10000,
				"load": {"target_vus": 5},
				"streaming_stop_conditions": {
					"stream_stall_seconds": 10
				}
			}
		]
	}`

	var cfg parsedRunConfig
	err := json.Unmarshal([]byte(configJSON), &cfg)
	if err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	// Find baseline stage
	baselineStage := findStageByName(&cfg, StageNameBaseline)
	if baselineStage == nil {
		t.Fatal("Expected to find baseline stage")
	}

	if baselineStage.StreamingStopConfig == nil {
		t.Fatal("Expected baseline stage to have StreamingStopConfig")
	}

	if baselineStage.StreamingStopConfig.StreamStallSeconds != 10 {
		t.Errorf("Expected StreamStallSeconds=10, got %d", baselineStage.StreamingStopConfig.StreamStallSeconds)
	}
}

func TestFindStageByName_DisabledStageWithStreamingConfig(t *testing.T) {
	configJSON := `{
		"scenario_id": "test",
		"target": {"kind": "server", "url": "http://localhost:3000", "transport": "streamable_http"},
		"stages": [
			{
				"stage_id": "stg_0000000000000002",
				"stage": "baseline",
				"enabled": false,
				"duration_ms": 10000,
				"load": {"target_vus": 5},
				"streaming_stop_conditions": {
					"stream_stall_seconds": 10
				}
			}
		]
	}`

	var cfg parsedRunConfig
	err := json.Unmarshal([]byte(configJSON), &cfg)
	if err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	// Should not find disabled stage
	baselineStage := findStageByName(&cfg, StageNameBaseline)
	if baselineStage != nil {
		t.Error("Expected not to find disabled baseline stage")
	}
}
