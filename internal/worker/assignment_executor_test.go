package worker

import (
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/session"
	"github.com/bc-dunia/mcpdrill/internal/types"
)

func TestAssignmentExecutorAppliesTargetTimeoutsTLSAndChurn(t *testing.T) {
	t.Setenv("MCPDRILL_TEST_CA_BUNDLE", "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----")
	executor := NewAssignmentExecutor("wkr_test", []string{"127.0.0.0/8"}, nil)
	assignment := types.WorkerAssignment{
		Load: types.LoadConfig{TargetRPS: 12.5},
		Target: types.TargetConfig{
			URL: "http://127.0.0.1:3000/mcp",
			Timeouts: &types.TimeoutConfig{
				ConnectTimeoutMs:     1234,
				RequestTimeoutMs:     5678,
				StreamStallTimeoutMs: 9012,
			},
			TLS: &types.TLSConfig{Verify: false, CABundleRef: "env://MCPDRILL_TEST_CA_BUNDLE"},
		},
		SessionPolicy: types.SessionPolicyConfig{
			Mode:             "churn",
			ChurnIntervalOps: 7,
		},
		Workload: types.WorkloadConfig{
			InFlightPerVU: 3,
			ThinkTime:     types.ThinkTimeConfig{BaseMs: 25, JitterMs: 5},
			UserJourney: &types.UserJourneyConfig{
				StartupSequence: &types.StartupSequenceConfig{RunToolsListOnStart: false},
				PeriodicOps:     &types.PeriodicOpsConfig{ToolsListIntervalMs: 1000, ToolsListAfterErrors: 2},
			},
		},
	}

	transportCfg := executor.buildTransportConfig(assignment)
	if transportCfg.Timeouts.ConnectTimeout != 1234*time.Millisecond {
		t.Fatalf("ConnectTimeout = %s, want 1234ms", transportCfg.Timeouts.ConnectTimeout)
	}
	if transportCfg.Timeouts.RequestTimeout != 5678*time.Millisecond {
		t.Fatalf("RequestTimeout = %s, want 5678ms", transportCfg.Timeouts.RequestTimeout)
	}
	if transportCfg.Timeouts.StreamStallTimeout != 9012*time.Millisecond {
		t.Fatalf("StreamStallTimeout = %s, want 9012ms", transportCfg.Timeouts.StreamStallTimeout)
	}
	if !transportCfg.TLSSkipVerify {
		t.Fatal("TLSSkipVerify = false, want true for verify=false")
	}
	if string(transportCfg.CABundle) != "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----" {
		t.Fatalf("CABundle = %q", string(transportCfg.CABundle))
	}

	sessionCfg := executor.buildSessionConfig(assignment, transportCfg, nil)
	if sessionCfg.Mode != session.ModeChurn {
		t.Fatalf("Mode = %s, want churn", sessionCfg.Mode)
	}
	if sessionCfg.ChurnIntervalOps != 7 {
		t.Fatalf("ChurnIntervalOps = %d, want 7", sessionCfg.ChurnIntervalOps)
	}
	if sessionCfg.TransportConfig != transportCfg {
		t.Fatal("session config did not retain transport config")
	}

	vuCfg := executor.buildVUConfig(assignment, nil, nil, transportCfg)
	if vuCfg.Load.TargetRPS != 12.5 {
		t.Fatalf("TargetRPS = %v, want 12.5", vuCfg.Load.TargetRPS)
	}
	if vuCfg.InFlightPerVU != 3 {
		t.Fatalf("InFlightPerVU = %d, want 3", vuCfg.InFlightPerVU)
	}
	if vuCfg.ThinkTime.BaseMs != 25 || vuCfg.ThinkTime.JitterMs != 5 {
		t.Fatalf("ThinkTime = %+v, want base=25 jitter=5", vuCfg.ThinkTime)
	}
	if vuCfg.UserJourney.StartupSequence.RunToolsListOnStart {
		t.Fatal("RunToolsListOnStart = true, want false")
	}
	if vuCfg.UserJourney.PeriodicOps.ToolsListIntervalMs != 1000 || vuCfg.UserJourney.PeriodicOps.ToolsListAfterErrors != 2 {
		t.Fatalf("PeriodicOps = %+v", vuCfg.UserJourney.PeriodicOps)
	}
	if vuCfg.UserJourney.ReconnectPolicy == nil || !vuCfg.UserJourney.ReconnectPolicy.Enabled {
		t.Fatal("ReconnectPolicy default was not preserved")
	}
}
