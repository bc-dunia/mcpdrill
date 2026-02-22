package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/controlplane/runmanager"
)

func TestSSE_ConnectAndReceiveEvents(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, err := rm.CreateRun(config, "test")
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL()+"/runs/"+runID+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream; charset=utf-8" {
		t.Errorf("Expected Content-Type text/event-stream; charset=utf-8, got %s", ct)
	}

	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Expected Cache-Control no-cache, got %s", cc)
	}

	scanner := bufio.NewScanner(resp.Body)
	var receivedEvents []string
	eventCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			receivedEvents = append(receivedEvents, strings.TrimPrefix(line, "data: "))
			eventCount++
			if eventCount >= 1 {
				cancel()
				break
			}
		}
	}

	if len(receivedEvents) == 0 {
		t.Error("Expected to receive at least one event")
	}

	var event runmanager.RunEvent
	if err := json.Unmarshal([]byte(receivedEvents[0]), &event); err != nil {
		t.Errorf("Failed to parse event JSON: %v", err)
	}

	if event.Type != runmanager.EventTypeRunCreated {
		t.Errorf("Expected RUN_CREATED event, got %s", event.Type)
	}
}

func TestSSE_RunNotFound(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	resp, err := http.Get(server.URL() + "/runs/nonexistent/events")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", resp.StatusCode)
	}
}

func TestSSE_InvalidSinceParam(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")

	resp, err := http.Get(server.URL() + "/runs/" + runID + "/events?since=invalid")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", resp.StatusCode)
	}
}

func TestSSE_InvalidLastEventID(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")

	// Non-evt_ prefix should be rejected per spec
	req, _ := http.NewRequest("GET", server.URL()+"/runs/"+runID+"/events", nil)
	req.Header.Set("Last-Event-ID", "not-evt-format")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", resp.StatusCode)
	}
}

func TestSSE_ReconnectWithLastEventID(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")
	_ = rm.StartRun(runID, "test")

	// First, get the first event to capture its event_id
	ctx1, cancel1 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel1()

	req1, _ := http.NewRequestWithContext(ctx1, "GET", server.URL()+"/runs/"+runID+"/events", nil)
	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}
	defer resp1.Body.Close()

	scanner1 := bufio.NewScanner(resp1.Body)
	var firstEventID string

	for scanner1.Scan() {
		line := scanner1.Text()
		if strings.HasPrefix(line, "id: ") {
			firstEventID = strings.TrimPrefix(line, "id: ")
			break
		}
	}

	if firstEventID == "" || !strings.HasPrefix(firstEventID, "evt_") {
		t.Fatalf("Expected evt_<hex> format event ID, got %q", firstEventID)
	}

	// Now reconnect with that event ID and expect to get the next event
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()

	req2, _ := http.NewRequestWithContext(ctx2, "GET", server.URL()+"/runs/"+runID+"/events", nil)
	req2.Header.Set("Last-Event-ID", firstEventID)

	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d", resp2.StatusCode)
	}

	scanner2 := bufio.NewScanner(resp2.Body)
	var secondEventID string

	for scanner2.Scan() {
		line := scanner2.Text()
		if strings.HasPrefix(line, "id: ") {
			secondEventID = strings.TrimPrefix(line, "id: ")
			break
		}
	}

	// The second event ID should be different from the first (it's the next event)
	if secondEventID == firstEventID {
		t.Errorf("Expected different event ID after Last-Event-ID reconnect, got same: %s", secondEventID)
	}

	// Verify it's in evt_<hex> format
	if !strings.HasPrefix(secondEventID, "evt_") {
		t.Errorf("Expected evt_<hex> format, got %s", secondEventID)
	}
}

func TestSSE_ReconnectWithSinceParam(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")
	_ = rm.StartRun(runID, "test")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// since=1 means start from index 1 (skip the first event)
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL()+"/runs/"+runID+"/events?since=1", nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	var firstEventID string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "id: ") {
			firstEventID = strings.TrimPrefix(line, "id: ")
			break
		}
	}

	// Event IDs should be in evt_<hex> format per spec
	if !strings.HasPrefix(firstEventID, "evt_") {
		t.Errorf("Expected evt_<hex> format event ID, got %s", firstEventID)
	}
}

func TestSSE_NegativeSinceParam(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")

	resp, err := http.Get(server.URL() + "/runs/" + runID + "/events?since=-1")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", resp.StatusCode)
	}
}

func TestSSE_ClientDisconnect(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")

	ctx, cancel := context.WithCancel(context.Background())

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL()+"/runs/"+runID+"/events", nil)

	done := make(chan struct{})
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Handler did not exit after client disconnect")
	}
}

func TestSSE_ConcurrentClients(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")

	numClients := 5
	var wg sync.WaitGroup
	errors := make(chan error, numClients)

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			req, _ := http.NewRequestWithContext(ctx, "GET", server.URL()+"/runs/"+runID+"/events", nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errors <- fmt.Errorf("client %d: request failed: %v", clientID, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				errors <- fmt.Errorf("client %d: expected 200, got %d", clientID, resp.StatusCode)
				return
			}

			scanner := bufio.NewScanner(resp.Body)
			receivedEvent := false
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data: ") {
					receivedEvent = true
					break
				}
			}

			if !receivedEvent {
				errors <- fmt.Errorf("client %d: no events received", clientID)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

func TestSSE_MethodNotAllowed(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")

	resp, err := http.Post(server.URL()+"/runs/"+runID+"/events", "application/json", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", resp.StatusCode)
	}
}

func TestSSE_EventFormat(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL()+"/runs/"+runID+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	var eventID string

	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if strings.HasPrefix(line, "id: ") {
			eventID = strings.TrimPrefix(line, "id: ")
		}
		if strings.HasPrefix(line, "data: ") {
			break
		}
	}

	hasID := false
	hasData := false
	for _, line := range lines {
		if strings.HasPrefix(line, "id: ") {
			hasID = true
		}
		if strings.HasPrefix(line, "data: ") {
			hasData = true
		}
	}

	if !hasID {
		t.Error("SSE event missing 'id:' field")
	}
	if !hasData {
		t.Error("SSE event missing 'data:' field")
	}

	// Verify event ID is in evt_<hex> format per spec
	if !strings.HasPrefix(eventID, "evt_") {
		t.Errorf("Expected evt_<hex> format event ID, got %s", eventID)
	}
}

func TestSSE_NewEventsWhileConnected(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL()+"/runs/"+runID+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	eventsCh := make(chan string, 10)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				eventsCh <- strings.TrimPrefix(line, "data: ")
			}
		}
		close(eventsCh)
	}()

	<-eventsCh

	time.Sleep(200 * time.Millisecond)
	_ = rm.StartRun(runID, "test")

	select {
	case eventData := <-eventsCh:
		var event runmanager.RunEvent
		if err := json.Unmarshal([]byte(eventData), &event); err != nil {
			t.Errorf("Failed to parse event: %v", err)
		}
		if event.Type != runmanager.EventTypeStateTransition {
			t.Errorf("Expected STATE_TRANSITION event, got %s", event.Type)
		}
	case <-time.After(2 * time.Second):
		t.Error("Did not receive new event after StartRun")
	}
}

func TestSSE_EventIDNotFound(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")

	// Use a valid format but non-existent event ID
	req, _ := http.NewRequest("GET", server.URL()+"/runs/"+runID+"/events", nil)
	req.Header.Set("Last-Event-ID", "evt_deadbeef01234567")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Should return 400 because the event ID is not found
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 for non-existent event ID, got %d", resp.StatusCode)
	}
}

func TestSSE_LastEventIDPrecedesInvalidCursorParam(t *testing.T) {
	rm := newTestRunManager(t)
	server, cleanup, err := StartTestServer(rm)
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer cleanup()

	config := loadValidConfig(t)
	runID, _ := rm.CreateRun(config, "test")
	_ = rm.StartRun(runID, "test")

	events, err := rm.TailEvents(runID, 0, 1)
	if err != nil || len(events) == 0 {
		t.Fatalf("failed to get seed event: %v", err)
	}
	lastEventID := events[0].EventID

	req, _ := http.NewRequest("GET", server.URL()+"/runs/"+runID+"/events?cursor=not-evt-format", nil)
	req.Header.Set("Last-Event-ID", lastEventID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 with valid Last-Event-ID precedence, got %d", resp.StatusCode)
	}
}
