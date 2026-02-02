package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/agent"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	psnet "github.com/shirou/gopsutil/v3/net"
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
	Timestamp int64                 `json:"timestamp"`
	Host      *agent.HostMetrics    `json:"host,omitempty"`
	Process   *agent.ProcessMetrics `json:"process,omitempty"`
}

func main() {
	controlPlaneURL := flag.String("control-plane-url", "http://localhost:8080", "Control plane URL")
	agentToken := flag.String("agent-token", "", "Agent authentication token")
	pairKey := flag.String("pair-key", "", "Pair key to link with test runs")
	listenPort := flag.Int("listen-port", 0, "Port of the MCP server process to monitor (0 = host metrics only)")
	pid := flag.Int("pid", 0, "PID of the process to monitor (mutually exclusive with --listen-port)")
	collectInterval := flag.Duration("collect-interval", 5*time.Second, "Metrics collection interval")
	flag.Parse()

	if *pairKey == "" {
		fmt.Fprintln(os.Stderr, "Error: --pair-key is required")
		os.Exit(1)
	}

	// Validate mutual exclusion: --pid and --listen-port cannot both be provided
	if *pid > 0 && *listenPort > 0 {
		fmt.Fprintln(os.Stderr, "Error: --pid and --listen-port are mutually exclusive (cannot use both)")
		os.Exit(1)
	}

	// Validate PID exists if provided
	var targetPID int
	if *pid > 0 {
		_, err := process.NewProcess(int32(*pid))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: PID %d does not exist or is not accessible: %v\n", *pid, err)
			os.Exit(1)
		}
		targetPID = *pid
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
	if *pid > 0 {
		fmt.Printf("Monitoring PID: %d\n", *pid)
	}

	// Resolve targetPID from --listen-port if not already set from --pid
	if *listenPort > 0 {
		// Try to find process with retry (process may still be starting up)
		foundPID := findProcessByPortWithRetry(ctx, *listenPort, 5, 1*time.Second, func(attempt int) {
			log.Printf("PID lookup attempt %d failed for port %d, retrying...", attempt, *listenPort)
		})
		if foundPID > 0 {
			targetPID = foundPID
			fmt.Printf("Found process PID: %d\n", targetPID)
		} else {
			log.Printf("Warning: No process found on port %d after retries, continuing with host metrics only", *listenPort)
		}
	}

	go collectAndSend(ctx, *controlPlaneURL, *agentToken, agentID, *pairKey, targetPID, *listenPort, *collectInterval)

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

func collectAndSend(ctx context.Context, baseURL, token, agentID, pairKey string, targetPID int, listenPort int, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	pidValid := targetPID > 0
	currentPID := targetPID

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !pidValid && listenPort > 0 {
				newPID := findProcessByPort(listenPort)
				if newPID > 0 {
					currentPID = newPID
					pidValid = true
					log.Printf("Re-discovered process on port %d: PID %d", listenPort, newPID)
				}
			}

			sample := collectMetrics(currentPID)

			if currentPID > 0 && sample.Process == nil && pidValid {
				pidValid = false
			}

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

	// Collect host metrics
	cpuPercent, err := cpu.Percent(0, false)
	if err == nil && len(cpuPercent) > 0 {
		sample.Host = &agent.HostMetrics{
			CPUPercent: cpuPercent[0],
		}

		// Memory info
		if memInfo, err := mem.VirtualMemory(); err == nil && memInfo != nil {
			sample.Host.MemTotal = memInfo.Total
			sample.Host.MemUsed = memInfo.Used
			sample.Host.MemAvailable = memInfo.Available
		}

		// Swap info
		if swapInfo, err := mem.SwapMemory(); err == nil && swapInfo != nil {
			sample.Host.SwapUsed = swapInfo.Used
		}

		// Load average (Unix systems)
		if loadAvg, err := load.Avg(); err == nil && loadAvg != nil {
			sample.Host.LoadAvg1 = loadAvg.Load1
			sample.Host.LoadAvg5 = loadAvg.Load5
			sample.Host.LoadAvg15 = loadAvg.Load15
		}
	}

	// Collect process metrics if monitoring a specific process
	if targetPID > 0 {
		proc, err := process.NewProcess(int32(targetPID))
		if err != nil {
			log.Printf("Warning: process %d no longer exists, continuing with host metrics only", targetPID)
		} else {
			cpuPct, _ := proc.CPUPercent()
			numThreads, _ := proc.NumThreads()

			sample.Process = &agent.ProcessMetrics{
				PID:        targetPID,
				CPUPercent: cpuPct,
				NumThreads: int(numThreads),
			}

			// Memory info
			if memInfo, err := proc.MemoryInfo(); err == nil && memInfo != nil {
				sample.Process.MemRSS = memInfo.RSS
				sample.Process.MemVMS = memInfo.VMS
			}

			// File descriptors (Unix only, ignore error on Windows)
			if numFDs, err := proc.NumFDs(); err == nil {
				sample.Process.NumFDs = int(numFDs)
			}

			// Open connections
			if conns, err := proc.Connections(); err == nil {
				sample.Process.OpenConnections = len(conns)
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

func findProcessByPortWithRetry(ctx context.Context, port int, maxRetries int, retryDelay time.Duration, onRetry func(attempt int)) int {
	results := make(chan int, maxRetries+1)

	for i := 0; i <= maxRetries; i++ {
		select {
		case pid := <-results:
			if pid > 0 {
				return pid
			}
		case <-ctx.Done():
			return drainResults(results)
		default:
		}

		go func() {
			results <- findProcessByPort(port)
		}()

		if onRetry != nil && i > 0 {
			onRetry(i)
		}

		if i < maxRetries {
			select {
			case pid := <-results:
				if pid > 0 {
					return pid
				}
			case <-ctx.Done():
				return drainResults(results)
			case <-time.After(retryDelay):
			}
		}
	}

	for {
		select {
		case pid := <-results:
			if pid > 0 {
				return pid
			}
		case <-ctx.Done():
			return drainResults(results)
		}
	}
}

func drainResults(ch chan int) int {
	for {
		select {
		case pid := <-ch:
			if pid > 0 {
				return pid
			}
		default:
			return 0
		}
	}
}

func findProcessByPort(port int) int {
	conns, err := psnet.Connections("tcp")
	if err == nil {
		for _, conn := range conns {
			if conn.Status == "LISTEN" && conn.Laddr.Port == uint32(port) && conn.Pid > 0 {
				return int(conn.Pid)
			}
		}
	}

	procs, err := process.Processes()
	if err != nil {
		return 0
	}
	for _, p := range procs {
		pconns, err := p.Connections()
		if err != nil {
			continue
		}
		for _, conn := range pconns {
			if conn.Status == "LISTEN" && conn.Laddr.Port == uint32(port) {
				return int(p.Pid)
			}
		}
	}
	return 0
}
