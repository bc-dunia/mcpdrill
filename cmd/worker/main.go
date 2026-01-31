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
	"syscall"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/types"
)

type registerRequest struct {
	HostInfo types.HostInfo       `json:"host_info"`
	Capacity types.WorkerCapacity `json:"capacity"`
}

type registerResponse struct {
	WorkerID string `json:"worker_id"`
}

type heartbeatRequest struct {
	Health *types.WorkerHealth `json:"health,omitempty"`
}

type assignmentsResponse struct {
	Assignments []types.WorkerAssignment `json:"assignments"`
}

func main() {
	controlPlane := flag.String("control-plane", "http://localhost:8080", "Control plane URL")
	maxVUs := flag.Int("max-vus", 100, "Maximum virtual users this worker can handle")
	heartbeatInterval := flag.Duration("heartbeat-interval", 10*time.Second, "Heartbeat interval")
	pollInterval := flag.Duration("poll-interval", 1*time.Second, "Assignment poll interval")
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

	workerID, err := register(ctx, *controlPlane, hostInfo, capacity)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to register with control plane: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Worker registered: %s\n", workerID)
	fmt.Printf("Control plane: %s\n", *controlPlane)
	fmt.Printf("Max VUs: %d\n", *maxVUs)

	go heartbeatLoop(ctx, *controlPlane, workerID, *heartbeatInterval)
	go pollAssignments(ctx, *controlPlane, workerID, *pollInterval)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down worker...")
	cancel()
	time.Sleep(1 * time.Second)
	fmt.Println("Worker stopped")
}

func register(ctx context.Context, baseURL string, hostInfo types.HostInfo, capacity types.WorkerCapacity) (string, error) {
	req := registerRequest{HostInfo: hostInfo, Capacity: capacity}
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/workers/register", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("registration failed: %s - %s", resp.Status, string(respBody))
	}

	var result registerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.WorkerID, nil
}

func heartbeatLoop(ctx context.Context, baseURL, workerID string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := sendHeartbeat(ctx, baseURL, workerID); err != nil {
				fmt.Fprintf(os.Stderr, "Heartbeat failed: %v\n", err)
			}
		}
	}
}

func sendHeartbeat(ctx context.Context, baseURL, workerID string) error {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	req := heartbeatRequest{
		Health: &types.WorkerHealth{
			MemBytes: int64(memStats.Alloc),
		},
	}
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/workers/"+workerID+"/heartbeat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("heartbeat failed: %s", resp.Status)
	}
	return nil
}

func pollAssignments(ctx context.Context, baseURL, workerID string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			assignments, err := getAssignments(ctx, baseURL, workerID)
			if err != nil {
				continue
			}
			for _, a := range assignments {
				fmt.Printf("Received assignment: run=%s stage=%s vus=%d-%d\n",
					a.RunID, a.Stage, a.VUIDStart, a.VUIDEnd)
			}
		}
	}
}

func getAssignments(ctx context.Context, baseURL, workerID string) ([]types.WorkerAssignment, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/workers/"+workerID+"/assignments", nil)
	if err != nil {
		return nil, err
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
