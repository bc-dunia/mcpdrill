package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/types"
	"github.com/bc-dunia/mcpdrill/internal/worker"
)

type registerRequest struct {
	HostInfo types.HostInfo       `json:"host_info"`
	Capacity types.WorkerCapacity `json:"capacity"`
}

type registerResponse struct {
	WorkerID    string `json:"worker_id"`
	WorkerToken string `json:"worker_token,omitempty"`
}

type heartbeatRequest struct {
	Health *types.WorkerHealth `json:"health,omitempty"`
}

type heartbeatResponse struct {
	OK                  bool     `json:"ok"`
	StopRunIDs          []string `json:"stop_run_ids,omitempty"`
	ImmediateStopRunIDs []string `json:"immediate_stop_run_ids,omitempty"`
}

type assignmentsResponse struct {
	Assignments []types.WorkerAssignment `json:"assignments"`
}

type ackAssignmentsRequest struct {
	LeaseIDs []string `json:"lease_ids"`
}

func main() {
	controlPlane := flag.String("control-plane", "http://localhost:8080", "Control plane URL")
	maxVUs := flag.Int("max-vus", 100, "Maximum virtual users this worker can handle")
	heartbeatInterval := flag.Duration("heartbeat-interval", 10*time.Second, "Heartbeat interval")
	pollInterval := flag.Duration("poll-interval", 1*time.Second, "Assignment poll interval")
	allowPrivateNetworks := flag.String("allow-private-networks", "", "Comma-separated CIDR ranges to allow (e.g., '127.0.0.0/8,10.0.0.0/8')")
	flag.Parse()

	hostname, _ := os.Hostname()
	hostInfo := types.HostInfo{
		Hostname: hostname,
		Platform: runtime.GOOS,
	}
	capacity := types.WorkerCapacity{
		MaxVUs:           *maxVUs,
		MaxConcurrentOps: *maxVUs * 10,
		MaxRPS:           float64(*maxVUs) * 10,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	workerID, workerToken, err := register(ctx, *controlPlane, hostInfo, capacity)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to register with control plane: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Worker registered: %s\n", workerID)
	fmt.Printf("Control plane: %s\n", *controlPlane)
	fmt.Printf("Max VUs: %d\n", *maxVUs)

	privateNets := parsePrivateNetworks(*allowPrivateNetworks)
	if len(privateNets) > 0 {
		fmt.Printf("Allowed private networks: %v\n", privateNets)
	}

	retryClient := worker.NewRetryHTTPClient(ctx, *controlPlane, http.DefaultClient, worker.RetryConfig{
		MaxRetries: 3,
		Backoff:    100 * time.Millisecond,
		MaxBackoff: 5 * time.Second,
	})
	retryClient.SetWorkerToken(workerToken)

	telemetryShipper := worker.NewTelemetryShipper(ctx, workerID, retryClient)
	defer telemetryShipper.Close()

	executor := worker.NewAssignmentExecutor(workerID, privateNets, telemetryShipper)

	go heartbeatLoop(ctx, *controlPlane, workerID, workerToken, *heartbeatInterval, executor)
	go pollAssignments(ctx, *controlPlane, workerID, workerToken, *pollInterval, executor)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down worker...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	for executor.ActiveAssignments() > 0 {
		select {
		case <-shutdownCtx.Done():
			fmt.Println("Shutdown timeout, forcing exit")
			goto done
		case <-time.After(500 * time.Millisecond):
			fmt.Printf("Waiting for %d active assignment(s) to complete...\n", executor.ActiveAssignments())
		}
	}

done:
	shipped, dropped := telemetryShipper.Stats()
	fmt.Printf("Telemetry stats: shipped=%d dropped=%d\n", shipped, dropped)
	fmt.Println("Worker stopped")
}

func parsePrivateNetworks(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func register(ctx context.Context, baseURL string, hostInfo types.HostInfo, capacity types.WorkerCapacity) (string, string, error) {
	req := registerRequest{HostInfo: hostInfo, Capacity: capacity}
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/workers/register", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("registration failed: %s - %s", resp.Status, string(respBody))
	}

	var result registerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}
	return result.WorkerID, result.WorkerToken, nil
}

func heartbeatLoop(ctx context.Context, baseURL, workerID, workerToken string, interval time.Duration, executor *worker.AssignmentExecutor) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			resp, err := sendHeartbeat(ctx, baseURL, workerID, workerToken, executor)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Heartbeat failed: %v\n", err)
				continue
			}

			for _, runID := range resp.StopRunIDs {
				executor.StopRun(runID, false)
			}
			for _, runID := range resp.ImmediateStopRunIDs {
				executor.StopRun(runID, true)
			}
		}
	}
}

func sendHeartbeat(ctx context.Context, baseURL, workerID, workerToken string, executor *worker.AssignmentExecutor) (*heartbeatResponse, error) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	req := heartbeatRequest{
		Health: &types.WorkerHealth{
			MemBytes:  int64(memStats.Alloc),
			ActiveVUs: executor.ActiveVUs(),
		},
	}
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/workers/"+workerID+"/heartbeat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if workerToken != "" {
		httpReq.Header.Set("X-Worker-Token", workerToken)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("heartbeat failed: %s", resp.Status)
	}

	var result heartbeatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func pollAssignments(ctx context.Context, baseURL, workerID, workerToken string, interval time.Duration, executor *worker.AssignmentExecutor) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			assignments, err := getAssignments(ctx, baseURL, workerID, workerToken)
			if err != nil {
				continue
			}
			var started []types.WorkerAssignment
			for _, a := range assignments {
				if err := executor.Execute(ctx, a); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to execute assignment %s: %v\n", a.LeaseID, err)
					continue
				}
				started = append(started, a)
			}
			if len(started) > 0 {
				if err := ackAssignments(ctx, baseURL, workerID, workerToken, started); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to ack assignments: %v\n", err)
				}
			}
		}
	}
}

func getAssignments(ctx context.Context, baseURL, workerID, workerToken string) ([]types.WorkerAssignment, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/workers/"+workerID+"/assignments", nil)
	if err != nil {
		return nil, err
	}
	if workerToken != "" {
		httpReq.Header.Set("X-Worker-Token", workerToken)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get assignments failed: %s", resp.Status)
	}

	var result assignmentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Assignments, nil
}

func ackAssignments(ctx context.Context, baseURL, workerID, workerToken string, assignments []types.WorkerAssignment) error {
	if len(assignments) == 0 {
		return nil
	}

	leaseIDs := make([]string, 0, len(assignments))
	for _, assignment := range assignments {
		if assignment.LeaseID == "" {
			continue
		}
		leaseIDs = append(leaseIDs, assignment.LeaseID)
	}
	if len(leaseIDs) == 0 {
		return nil
	}

	req := ackAssignmentsRequest{LeaseIDs: leaseIDs}
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/workers/"+workerID+"/assignments/ack", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if workerToken != "" {
		httpReq.Header.Set("X-Worker-Token", workerToken)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ack assignments failed: %s - %s", resp.Status, string(respBody))
	}

	return nil
}
