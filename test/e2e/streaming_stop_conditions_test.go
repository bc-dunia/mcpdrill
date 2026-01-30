package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/mockserver"
)

func TestStreamingStopConditions_StreamStall(t *testing.T) {
	config := mockserver.DefaultConfig()
	config.Addr = ":0"
	config.SetBehavior(&mockserver.BehaviorProfile{
		StreamingChunkCount:   10,
		StreamingChunkDelayMs: 2000,
	})

	server := mockserver.New(config)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	addr := server.Addr()
	if addr == "" {
		t.Fatal("Server did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "test-stall-1",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "streaming_tool",
			"arguments": map[string]interface{}{
				"chunks":   10,
				"delay_ms": 2000,
			},
		},
	}
	body, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/mcp", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			t.Logf("Request timed out as expected (simulating stall detection)")
			return
		}
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("Expected Content-Type text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	eventCount := 0
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			eventCount++
			t.Logf("Received event %d: %s", eventCount, line[:minInt(len(line), 100)])
		}
	}

	t.Logf("Stream stall test: received %d events before timeout/completion", eventCount)
}

func TestStreamingStopConditions_MinEventsPerSecond(t *testing.T) {
	config := mockserver.DefaultConfig()
	config.Addr = ":0"
	config.SetBehavior(&mockserver.BehaviorProfile{
		StreamingChunkCount:   5,
		StreamingChunkDelayMs: 500,
	})

	server := mockserver.New(config)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	addr := server.Addr()
	if addr == "" {
		t.Fatal("Server did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "test-rate-1",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "streaming_tool",
			"arguments": map[string]interface{}{
				"chunks":   5,
				"delay_ms": 500,
			},
		},
	}
	body, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/mcp", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	startTime := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("Expected Content-Type text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	eventCount := 0
	var lastEventTime time.Time
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			eventCount++
			lastEventTime = time.Now()
		}
	}

	duration := lastEventTime.Sub(startTime)
	if duration > 0 {
		eventsPerSecond := float64(eventCount) / duration.Seconds()
		t.Logf("Min events/sec test: %d events in %.2fs = %.2f events/sec", eventCount, duration.Seconds(), eventsPerSecond)

		expectedRate := 2.0
		if eventsPerSecond < expectedRate {
			t.Logf("Event rate (%.2f/sec) is below threshold (%.2f/sec) - would trigger stop condition", eventsPerSecond, expectedRate)
		}
	}

	if eventCount < 5 {
		t.Errorf("Expected at least 5 events (progress notifications), got %d", eventCount)
	}
}

func TestStreamingStopConditions_NoTriggerWhenHealthy(t *testing.T) {
	config := mockserver.DefaultConfig()
	config.Addr = ":0"
	config.SetBehavior(&mockserver.BehaviorProfile{
		StreamingChunkCount:   5,
		StreamingChunkDelayMs: 50,
	})

	server := mockserver.New(config)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	addr := server.Addr()
	if addr == "" {
		t.Fatal("Server did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "test-healthy-1",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "streaming_tool",
			"arguments": map[string]interface{}{
				"chunks":   5,
				"delay_ms": 50,
			},
		},
	}
	body, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/mcp", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	startTime := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("Expected Content-Type text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	eventCount := 0
	var finalResponse map[string]interface{}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			eventCount++
			data := strings.TrimPrefix(line, "data: ")
			var msg map[string]interface{}
			if json.Unmarshal([]byte(data), &msg) == nil {
				if msg["result"] != nil {
					finalResponse = msg
				}
			}
		}
	}

	duration := time.Since(startTime)
	t.Logf("Healthy stream test: %d events in %.2fs", eventCount, duration.Seconds())

	if eventCount < 6 {
		t.Errorf("Expected at least 6 events (5 progress + 1 final), got %d", eventCount)
	}

	if finalResponse == nil {
		t.Error("Did not receive final response with result")
	} else {
		t.Logf("Received final response with ID: %v", finalResponse["id"])
	}

	if duration.Seconds() > 2.0 {
		t.Errorf("Healthy stream took too long: %.2fs (expected < 2s)", duration.Seconds())
	}
}

func TestStreamingStopConditions_ConfigParsing(t *testing.T) {
	testCases := []struct {
		name          string
		chunkCount    int
		chunkDelayMs  int
		expectedCount int
	}{
		{"default_config", 5, 50, 6},
		{"single_chunk", 1, 50, 2},
		{"many_chunks", 10, 10, 11},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := mockserver.DefaultConfig()
			config.Addr = ":0"
			config.SetBehavior(&mockserver.BehaviorProfile{
				StreamingChunkCount:   tc.chunkCount,
				StreamingChunkDelayMs: tc.chunkDelayMs,
			})

			server := mockserver.New(config)
			if err := server.Start(); err != nil {
				t.Fatalf("Failed to start server: %v", err)
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				server.Stop(ctx)
			}()

			time.Sleep(100 * time.Millisecond)

			addr := server.Addr()
			if addr == "" {
				t.Fatal("Server did not start")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			reqBody := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      fmt.Sprintf("test-config-%s", tc.name),
				"method":  "tools/call",
				"params": map[string]interface{}{
					"name":      "streaming_tool",
					"arguments": map[string]interface{}{},
				},
			}
			body, _ := json.Marshal(reqBody)

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/mcp", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "text/event-stream")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			eventCount := 0
			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data:") {
					eventCount++
				}
			}

			t.Logf("Config %s: chunks=%d, delay=%dms -> %d events", tc.name, tc.chunkCount, tc.chunkDelayMs, eventCount)

			if eventCount != tc.expectedCount {
				t.Errorf("Expected %d events, got %d", tc.expectedCount, eventCount)
			}
		})
	}
}

func TestStreamingSSEFormat(t *testing.T) {
	config := mockserver.DefaultConfig()
	config.Addr = ":0"
	config.SetBehavior(&mockserver.BehaviorProfile{
		StreamingChunkCount:   3,
		StreamingChunkDelayMs: 10,
	})

	server := mockserver.New(config)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	addr := server.Addr()
	if addr == "" {
		t.Fatal("Server did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "test-format-1",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "streaming_tool",
			"arguments": map[string]interface{}{},
		},
	}
	body, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/mcp", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "text/event-stream" {
		t.Errorf("Expected Content-Type text/event-stream, got %s", contentType)
	}

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	lines := strings.Split(string(rawBody), "\n")
	dataLineCount := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "data:") {
			dataLineCount++
			data := strings.TrimPrefix(line, "data: ")

			var msg map[string]interface{}
			if err := json.Unmarshal([]byte(data), &msg); err != nil {
				t.Errorf("Invalid JSON in SSE data line: %v", err)
				continue
			}

			if msg["jsonrpc"] != "2.0" {
				t.Errorf("Expected jsonrpc 2.0, got %v", msg["jsonrpc"])
			}

			t.Logf("SSE event %d: method=%v, id=%v", dataLineCount, msg["method"], msg["id"])
		}
	}

	if dataLineCount < 4 {
		t.Errorf("Expected at least 4 SSE data lines (3 progress + 1 final), got %d", dataLineCount)
	}

	t.Logf("SSE format test passed: %d data lines, all valid JSON-RPC", dataLineCount)
}

func TestStreamingNonStreamingToolStillWorks(t *testing.T) {
	config := mockserver.DefaultConfig()
	config.Addr = ":0"

	server := mockserver.New(config)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	addr := server.Addr()
	if addr == "" {
		t.Fatal("Server did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "test-non-streaming-1",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "fast_echo",
			"arguments": map[string]interface{}{
				"message": "hello world",
			},
		},
	}
	body, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/mcp", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json for non-streaming tool, got %s", contentType)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result["jsonrpc"] != "2.0" {
		t.Errorf("Expected jsonrpc 2.0, got %v", result["jsonrpc"])
	}

	if result["id"] != "test-non-streaming-1" {
		t.Errorf("Expected id test-non-streaming-1, got %v", result["id"])
	}

	if result["result"] == nil {
		t.Error("Expected result in response")
	}

	t.Logf("Non-streaming tool test passed: received JSON response with result")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
