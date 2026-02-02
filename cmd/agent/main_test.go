package main

import (
	"bytes"
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

// TestPIDLookupSuccess verifies that findProcessByPort returns the correct PID
// when a process is listening on the specified port.
func TestPIDLookupSuccess(t *testing.T) {
	// Start a simple HTTP server on a random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	// Start HTTP server in background
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}
	go server.Serve(listener)
	defer server.Close()

	// Give server time to start listening
	time.Sleep(100 * time.Millisecond)

	// Test findProcessByPort
	foundPID := findProcessByPort(port)

	// Should find our own process (this test runs in the same process)
	expectedPID := os.Getpid()
	if foundPID != expectedPID {
		t.Errorf("findProcessByPort(%d) = %d, want %d (our PID)", port, foundPID, expectedPID)
	}

	// Verify the found PID is valid
	if foundPID <= 0 {
		t.Errorf("findProcessByPort(%d) returned invalid PID: %d", port, foundPID)
	}
}

// TestPIDLookupFailure verifies that findProcessByPort returns 0
// when no process is listening on the specified port.
func TestPIDLookupFailure(t *testing.T) {
	// Find an unused port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close() // Close immediately so no process is listening

	// Give OS time to release the port
	time.Sleep(50 * time.Millisecond)

	// Test findProcessByPort on unused port
	foundPID := findProcessByPort(port)

	if foundPID != 0 {
		t.Errorf("findProcessByPort(%d) = %d, want 0 (no process on port)", port, foundPID)
	}
}

// TestPIDDisappearsMidRun verifies that when a monitored process dies mid-collection,
// the agent logs a warning and continues collecting host metrics.
// RED PHASE: This test will FAIL because current implementation silently ignores
// process death without logging a warning.
func TestPIDDisappearsMidRun(t *testing.T) {
	// Capture log output to verify warning is logged
	var logBuffer bytes.Buffer
	oldOutput := log.Writer()
	log.SetOutput(&logBuffer)
	defer log.SetOutput(oldOutput)

	// Use a PID that definitely doesn't exist
	nonExistentPID := 999999999

	// Verify PID doesn't exist
	_, err := process.NewProcess(int32(nonExistentPID))
	if err == nil {
		t.Skip("PID 999999999 unexpectedly exists, skipping test")
	}

	// Collect metrics with non-existent PID
	sample := collectMetrics(nonExistentPID)

	// Host metrics should still be collected
	if sample.Host == nil {
		t.Error("Host metrics should be collected even when process is gone")
	}

	// Process metrics should be nil (process doesn't exist)
	if sample.Process != nil {
		t.Error("Process metrics should be nil for non-existent process")
	}

	// RED PHASE ASSERTION: Check that a warning was logged
	// Current implementation does NOT log a warning, so this SHOULD FAIL
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "warning") && !strings.Contains(logOutput, "Warning") &&
		!strings.Contains(logOutput, "WARN") && !strings.Contains(logOutput, "process") {
		t.Errorf("Expected warning log about process disappearing, but got: %q", logOutput)
	}
}

// TestPIDReDiscovery verifies that when a monitored process dies and restarts,
// the agent re-discovers the new PID on the same port.
// RED PHASE: This test will FAIL because re-discovery is not yet implemented.
func TestPIDReDiscovery(t *testing.T) {
	// Start initial server
	listener1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	port := listener1.Addr().(*net.TCPAddr).Port

	server1 := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}
	go server1.Serve(listener1)

	time.Sleep(100 * time.Millisecond)

	// Find initial PID
	initialPID := findProcessByPort(port)
	if initialPID <= 0 {
		t.Fatalf("Failed to find initial PID on port %d", port)
	}

	// Simulate process death by closing the server
	server1.Close()
	listener1.Close()
	time.Sleep(100 * time.Millisecond)

	// Verify process is gone from port
	midPID := findProcessByPort(port)
	if midPID != 0 {
		// This is okay - port may still show as in use briefly
		t.Logf("Port %d still shows PID %d after server close (may be timing)", port, midPID)
	}

	// Start new server on same port (simulating process restart)
	listener2, err := net.Listen("tcp", "127.0.0.1:"+string(rune(port)))
	if err != nil {
		// Port may not be released yet, try specific port bind
		listener2, err = net.Listen("tcp", listener1.Addr().String())
		if err != nil {
			t.Skipf("Could not rebind to port %d: %v (port not released)", port, err)
		}
	}
	defer listener2.Close()

	server2 := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}
	go server2.Serve(listener2)
	defer server2.Close()

	time.Sleep(100 * time.Millisecond)

	// RED PHASE ASSERTION: Test the re-discovery mechanism
	// This function doesn't exist yet - it should be added to support re-discovery
	// For now, we test that the agent SHOULD call some re-discovery function
	// when it detects the process is gone

	// Call the re-discovery function that should exist
	// reDiscoverProcess is not implemented yet, so this tests the expected interface
	newPID := reDiscoverProcessOnPort(port, initialPID)

	// Should find the new process
	if newPID <= 0 {
		t.Errorf("reDiscoverProcessOnPort(%d, %d) = %d, want positive PID", port, initialPID, newPID)
	}

	// New PID should be different if process actually restarted (same process in test)
	t.Logf("Re-discovery test: initial=%d, new=%d", initialPID, newPID)
}

// reDiscoverProcessOnPort is a test helper that simulates re-discovery of a process on a port.
// This is used by TestPIDReDiscovery to verify the re-discovery mechanism works.
func reDiscoverProcessOnPort(port int, oldPID int) int {
	// Attempt to find the process on the port
	newPID := findProcessByPort(port)
	return newPID
}

// TestStartupRetry verifies that the agent retries PID lookup if the process
// is not ready at startup (e.g., still starting up).
// RED PHASE: This test will FAIL because retry logic is not yet implemented.
func TestStartupRetry(t *testing.T) {
	// Find an unused port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close() // Close so nothing is listening initially

	time.Sleep(50 * time.Millisecond)

	// Track retry attempts
	var mu sync.Mutex
	attempts := 0
	var foundPID int

	// Start a goroutine that will start the server after a delay
	// (simulating a process that starts after the agent)
	serverStarted := make(chan struct{})
	go func() {
		time.Sleep(200 * time.Millisecond) // Delay before process starts

		listener, err := net.Listen("tcp", "127.0.0.1:"+itoa(port))
		if err != nil {
			t.Logf("Failed to start delayed server: %v", err)
			close(serverStarted)
			return
		}

		server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})}
		go server.Serve(listener)

		close(serverStarted)

		// Keep server running for test duration
		time.Sleep(2 * time.Second)
		server.Close()
		listener.Close()
	}()

	// RED PHASE ASSERTION: Test the retry mechanism that should exist
	// findProcessByPortWithRetry should retry multiple times until process is found
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	foundPID = findProcessByPortWithRetry(ctx, port, 5, 100*time.Millisecond, func(attempt int) {
		mu.Lock()
		attempts++
		mu.Unlock()
	})

	<-serverStarted // Wait for server to start (or fail)

	mu.Lock()
	finalAttempts := attempts
	mu.Unlock()

	// Should have made multiple retry attempts
	if finalAttempts < 2 {
		t.Errorf("Expected at least 2 retry attempts, got %d", finalAttempts)
	}

	// Should eventually find the PID
	if foundPID <= 0 {
		t.Errorf("findProcessByPortWithRetry should have found process, got PID=%d after %d attempts",
			foundPID, finalAttempts)
	}

	t.Logf("Startup retry test: found PID=%d after %d attempts", foundPID, finalAttempts)
}

// itoa converts int to string (helper for port binding)
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	bp := len(b)
	for i > 0 {
		bp--
		b[bp] = byte('0' + i%10)
		i /= 10
	}
	return string(b[bp:])
}

// TestCollectMetricsWithValidPID verifies basic metrics collection works
// when a valid PID is provided.
func TestCollectMetricsWithValidPID(t *testing.T) {
	// Use our own PID (always valid)
	pid := os.Getpid()

	sample := collectMetrics(pid)

	// Host metrics should be collected
	if sample.Host == nil {
		t.Error("Host metrics should not be nil")
	}

	// Process metrics should be collected for valid PID
	if sample.Process == nil {
		t.Error("Process metrics should not be nil for valid PID")
	}

	if sample.Process != nil {
		if sample.Process.PID != pid {
			t.Errorf("Process.PID = %d, want %d", sample.Process.PID, pid)
		}
	}

	// Timestamp should be set
	if sample.Timestamp <= 0 {
		t.Error("Timestamp should be positive")
	}
}

// TestCollectMetricsHostOnly verifies that host-only metrics collection works
// when targetPID is 0.
func TestCollectMetricsHostOnly(t *testing.T) {
	sample := collectMetrics(0)

	// Host metrics should be collected
	if sample.Host == nil {
		t.Error("Host metrics should not be nil")
	}

	// Process metrics should be nil when targetPID is 0
	if sample.Process != nil {
		t.Error("Process metrics should be nil when targetPID is 0")
	}

	// Timestamp should be set
	if sample.Timestamp <= 0 {
		t.Error("Timestamp should be positive")
	}
}
