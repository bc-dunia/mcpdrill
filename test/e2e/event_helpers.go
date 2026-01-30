package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Event represents a parsed SSE event from the control plane.
type Event struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
	RunID   string                 `json:"run_id"`
	EventID string                 `json:"event_id"`
}

// EventStream represents a connection to the SSE event stream.
type EventStream struct {
	events chan Event
	resp   *http.Response
	cancel context.CancelFunc
	ctx    context.Context
}

// StreamEvents connects to the SSE endpoint and returns a channel of events.
// The caller should call Close() when done to clean up resources.
func StreamEvents(serverURL, runID string) (*EventStream, error) {
	ctx, cancel := context.WithCancel(context.Background())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/runs/"+runID+"/events", nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{
		Timeout: 0, // No timeout for SSE
	}

	resp, err := client.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to connect to event stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("event stream returned status %d", resp.StatusCode)
	}

	es := &EventStream{
		events: make(chan Event, 100),
		resp:   resp,
		cancel: cancel,
		ctx:    ctx,
	}

	go es.readLoop()

	return es, nil
}

// Events returns the channel of events.
func (es *EventStream) Events() <-chan Event {
	return es.events
}

// Close closes the event stream and releases resources.
func (es *EventStream) Close() {
	es.cancel()
	if es.resp != nil {
		es.resp.Body.Close()
	}
}

// readLoop reads events from the SSE stream and sends them to the channel.
func (es *EventStream) readLoop() {
	defer close(es.events)
	defer es.resp.Body.Close()

	scanner := bufio.NewScanner(es.resp.Body)
	for scanner.Scan() {
		select {
		case <-es.ctx.Done():
			return
		default:
		}

		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var event Event
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}
			select {
			case es.events <- event:
			case <-es.ctx.Done():
				return
			}
		}
	}
}

// WaitForEvent waits for an event of the specified type within the timeout.
// Returns the event if found, or an error if timeout or stream closed.
func WaitForEvent(events <-chan Event, eventType string, timeout time.Duration) (*Event, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return nil, fmt.Errorf("event stream closed while waiting for %s", eventType)
			}
			if event.Type == eventType {
				return &event, nil
			}
		case <-timer.C:
			return nil, fmt.Errorf("timeout waiting for event %s after %v", eventType, timeout)
		}
	}
}

// WaitForEventWithPayload waits for an event of the specified type that matches the payload predicate.
func WaitForEventWithPayload(events <-chan Event, eventType string, predicate func(map[string]interface{}) bool, timeout time.Duration) (*Event, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return nil, fmt.Errorf("event stream closed while waiting for %s", eventType)
			}
			if event.Type == eventType && predicate(event.Payload) {
				return &event, nil
			}
		case <-timer.C:
			return nil, fmt.Errorf("timeout waiting for event %s with matching payload after %v", eventType, timeout)
		}
	}
}

// CollectEvents collects all events from the channel until timeout.
// Useful for verifying all expected events were emitted.
func CollectEvents(events <-chan Event, timeout time.Duration) []Event {
	var collected []Event
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return collected
			}
			collected = append(collected, event)
		case <-timer.C:
			return collected
		}
	}
}

// HasEventType checks if the event list contains an event of the specified type.
func HasEventType(events []Event, eventType string) bool {
	for _, e := range events {
		if e.Type == eventType {
			return true
		}
	}
	return false
}

// GetEventsByType returns all events of the specified type from the list.
func GetEventsByType(events []Event, eventType string) []Event {
	var result []Event
	for _, e := range events {
		if e.Type == eventType {
			result = append(result, e)
		}
	}
	return result
}

// EventContainsWorkerID checks if an event's payload contains the specified worker_id.
func EventContainsWorkerID(event *Event, workerID string) bool {
	if event == nil || event.Payload == nil {
		return false
	}
	if wid, ok := event.Payload["worker_id"].(string); ok {
		return wid == workerID
	}
	return false
}

// EventContainsLostWorker checks if an event's payload contains the specified lost_worker.
func EventContainsLostWorker(event *Event, workerID string) bool {
	if event == nil || event.Payload == nil {
		return false
	}
	if wid, ok := event.Payload["lost_worker"].(string); ok {
		return wid == workerID
	}
	return false
}
