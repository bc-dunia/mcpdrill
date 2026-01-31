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

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
)

type registerRequest struct {
	PairKey  string `json:"pair_key"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Version  string `json:"version"`
}

type registerResponse struct {
	AgentID    string `json:"agent_id"`
	ServerTime int64  `json:"server_time"`
}

type metricsRequest struct {
	AgentID string          `json:"agent_id"`
	PairKey string          `json:"pair_key"`
	Samples []metricsSample `json:"samples"`
}

type metricsSample struct {
	Timestamp int64        `json:"timestamp"`
	Host      *hostMetrics `json:"host,omitempty"`
	Process   *procMetrics `json:"process,omitempty"`
}

type hostMetrics struct {
	CPUPercent   float64 `json:"cpu_percent"`
	MemTotal     uint64  `json:"mem_total"`
	MemUsed      uint64  `json:"mem_used"`
	MemAvailable uint64  `json:"mem_available"`
}

type procMetrics struct {
	PID        int     `json:"pid"`
	CPUPercent float64 `json:"cpu_percent"`
	MemRSS     uint64  `json:"mem_rss"`
	NumThreads int     `json:"num_threads"`
}

func main() {
	controlPlaneURL := flag.String("control-plane-url", "http://localhost:8080", "Control plane URL")
	agentToken := flag.String("agent-token", "", "Agent authentication token")
	pairKey := flag.String("pair-key", "", "Pair key to link with test runs")
	listenPort := flag.Int("listen-port", 0, "Port of the MCP server process to monitor (0 = host metrics only)")
	collectInterval := flag.Duration("collect-interval", 5*time.Second, "Metrics collection interval")
	flag.Parse()

	if *pairKey == "" {
		fmt.Fprintln(os.Stderr, "Error: --pair-key is required")
		os.Exit(1)
	}

	hostname, _ := os.Hostname()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agentID, err := register(ctx, *controlPlaneURL, *agentToken, *pairKey, hostname)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to register with control plane: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Agent registered: %s\n", agentID)
	fmt.Printf("Control plane: %s\n", *controlPlaneURL)
	fmt.Printf("Pair key: %s\n", *pairKey)
	if *listenPort > 0 {
		fmt.Printf("Monitoring port: %d\n", *listenPort)
	}

	var targetPID int
	if *listenPort > 0 {
		targetPID = findProcessByPort(*listenPort)
		if targetPID > 0 {
			fmt.Printf("Found process PID: %d\n", targetPID)
		}
	}

	go collectAndSend(ctx, *controlPlaneURL, *agentToken, agentID, *pairKey, targetPID, *collectInterval)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down agent...")
	cancel()
	time.Sleep(1 * time.Second)
	fmt.Println("Agent stopped")
}

func register(ctx context.Context, baseURL, token, pairKey, hostname string) (string, error) {
	req := registerRequest{
		PairKey:  pairKey,
		Hostname: hostname,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Version:  "1.0.0",
	}
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/agents/v1/register", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

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
	return result.AgentID, nil
}

func collectAndSend(ctx context.Context, baseURL, token, agentID, pairKey string, targetPID int, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sample := collectMetrics(targetPID)
			if err := sendMetrics(ctx, baseURL, token, agentID, pairKey, sample); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to send metrics: %v\n", err)
			}
		}
	}
}

func collectMetrics(targetPID int) metricsSample {
	sample := metricsSample{
		Timestamp: time.Now().UnixMilli(),
	}

	cpuPercent, err := cpu.Percent(0, false)
	if err == nil && len(cpuPercent) > 0 {
		memInfo, _ := mem.VirtualMemory()
		sample.Host = &hostMetrics{
			CPUPercent: cpuPercent[0],
		}
		if memInfo != nil {
			sample.Host.MemTotal = memInfo.Total
			sample.Host.MemUsed = memInfo.Used
			sample.Host.MemAvailable = memInfo.Available
		}
	}

	if targetPID > 0 {
		proc, err := process.NewProcess(int32(targetPID))
		if err == nil {
			cpuPct, _ := proc.CPUPercent()
			memInfo, _ := proc.MemoryInfo()
			numThreads, _ := proc.NumThreads()

			sample.Process = &procMetrics{
				PID:        targetPID,
				CPUPercent: cpuPct,
				NumThreads: int(numThreads),
			}
			if memInfo != nil {
				sample.Process.MemRSS = memInfo.RSS
			}
		}
	}

	return sample
}

func sendMetrics(ctx context.Context, baseURL, token, agentID, pairKey string, sample metricsSample) error {
	req := metricsRequest{
		AgentID: agentID,
		PairKey: pairKey,
		Samples: []metricsSample{sample},
	}
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/agents/v1/metrics", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("send metrics failed: %s", resp.Status)
	}
	return nil
}

func findProcessByPort(port int) int {
	procs, err := process.Processes()
	if err != nil {
		return 0
	}

	for _, p := range procs {
		conns, err := p.Connections()
		if err != nil {
			continue
		}
		for _, conn := range conns {
			if conn.Status == "LISTEN" && conn.Laddr.Port == uint32(port) {
				return int(p.Pid)
			}
		}
	}
	return 0
}
