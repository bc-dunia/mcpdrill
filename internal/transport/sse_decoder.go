package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrStreamClosed   = errors.New("stream closed")
	ErrStreamStall    = errors.New("stream stall timeout")
	ErrMalformedEvent = errors.New("malformed SSE event")
	ErrInvalidJSON    = errors.New("invalid JSON in SSE data")
)

// eventIDPattern validates event IDs: evt_<hex> format per spec
var eventIDPattern = regexp.MustCompile(`^evt_[0-9a-f]+$`)

// isValidEventID checks if an event ID matches the spec format
func isValidEventID(id string) bool {
	return eventIDPattern.MatchString(id)
}

// lineResult holds a line read from the reader
type lineResult struct {
	line string
	err  error
}

type SSEDecoder struct {
	reader       *bufio.Reader
	closer       io.Closer
	stallTimeout time.Duration
	lastEventID  string
	lastEventMu  sync.RWMutex
	mu           sync.Mutex
	closed       bool

	// Single reader goroutine pattern to prevent goroutine leaks
	lineCh   chan lineResult
	cancelFn context.CancelFunc
	wg       sync.WaitGroup
	started  bool
}

func NewSSEDecoder(r io.ReadCloser, stallTimeout time.Duration) *SSEDecoder {
	ctx, cancel := context.WithCancel(context.Background())
	d := &SSEDecoder{
		reader:       bufio.NewReader(r),
		closer:       r,
		stallTimeout: stallTimeout,
		lineCh:       make(chan lineResult, 1),
		cancelFn:     cancel,
	}
	// Start single reader goroutine
	d.wg.Add(1)
	d.started = true
	go d.readerLoop(ctx)
	return d
}

// readerLoop is a single goroutine that reads lines and sends them to lineCh.
// It exits when context is cancelled or EOF/error is encountered.
func (d *SSEDecoder) readerLoop(ctx context.Context) {
	defer d.wg.Done()
	for {
		line, err := d.reader.ReadString('\n')
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		select {
		case <-ctx.Done():
			return
		case d.lineCh <- lineResult{line: line, err: err}:
			if err != nil {
				// EOF or error - exit the loop
				return
			}
		}
	}
}

func (d *SSEDecoder) ReadEvent() (*SSEEvent, error) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil, ErrStreamClosed
	}
	d.mu.Unlock()

	event := &SSEEvent{}
	var dataLines []string

	for {
		line, err := d.readLineWithTimeout()
		if err != nil {
			if err == io.EOF {
				if len(dataLines) > 0 {
					event.Data = strings.Join(dataLines, "\n")
					// Only update lastEventID if it's a valid evt_<hex> format
					if event.ID != "" && isValidEventID(event.ID) {
						d.lastEventMu.Lock()
						d.lastEventID = event.ID
						d.lastEventMu.Unlock()
					}
					return event, nil
				}
				return nil, io.EOF
			}
			return nil, err
		}

		if line == "" {
			if len(dataLines) > 0 || event.Event != "" || event.ID != "" {
				event.Data = strings.Join(dataLines, "\n")
				// Only update lastEventID if it's a valid evt_<hex> format
				if event.ID != "" && isValidEventID(event.ID) {
					d.lastEventMu.Lock()
					d.lastEventID = event.ID
					d.lastEventMu.Unlock()
				}
				return event, nil
			}
			continue
		}

		if strings.HasPrefix(line, ":") {
			continue
		}

		colonIdx := strings.Index(line, ":")
		var field, value string
		if colonIdx == -1 {
			field = line
			value = ""
		} else {
			field = line[:colonIdx]
			value = line[colonIdx+1:]
			if strings.HasPrefix(value, " ") {
				value = value[1:]
			}
		}

		switch field {
		case "event":
			event.Event = value
		case "data":
			dataLines = append(dataLines, value)
		case "id":
			if !strings.Contains(value, "\x00") {
				event.ID = value
			}
		case "retry":
			if retry, err := strconv.Atoi(value); err == nil {
				event.Retry = retry
			}
		}
	}
}

func (d *SSEDecoder) readLineWithTimeout() (string, error) {
	timer := time.NewTimer(d.stallTimeout)
	defer timer.Stop()

	select {
	case r, ok := <-d.lineCh:
		if !ok {
			return "", ErrStreamClosed
		}
		return r.line, r.err
	case <-timer.C:
		return "", ErrStreamStall
	}
}

func (d *SSEDecoder) LastEventID() string {
	d.lastEventMu.RLock()
	defer d.lastEventMu.RUnlock()
	return d.lastEventID
}

func (d *SSEDecoder) Close() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	d.mu.Unlock()

	// Cancel the reader goroutine and wait for it to exit
	d.cancelFn()

	// Close the underlying reader to unblock any pending read
	err := d.closer.Close()

	// Wait for reader goroutine to exit (with timeout to prevent deadlock)
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		// Reader goroutine didn't exit in time, but we've done our best
	}

	return err
}

type SSEResponseHandler struct {
	stallTimeout time.Duration
}

func NewSSEResponseHandler(stallTimeout time.Duration) *SSEResponseHandler {
	return &SSEResponseHandler{
		stallTimeout: stallTimeout,
	}
}

func (h *SSEResponseHandler) HandleSSEStream(
	ctx context.Context,
	body io.ReadCloser,
	requestID string,
) (*JSONRPCResponse, *StreamSignals, error) {
	decoder := NewSSEDecoder(body, h.stallTimeout)
	defer decoder.Close()

	signals := &StreamSignals{
		IsStreaming: true,
	}

	var notifications []json.RawMessage
	var finalResponse *JSONRPCResponse
	startTime := time.Now()
	var firstEventTime *time.Time
	var lastEventTime *time.Time

	gapTracker := newEventGapTracker()

	for {
		select {
		case <-ctx.Done():
			signals.EndedNormally = false
			h.finalizeStreamSignals(signals, gapTracker, firstEventTime, startTime)
			return nil, signals, ctx.Err()
		default:
		}

		event, err := decoder.ReadEvent()
		if err != nil {
			if err == io.EOF {
				signals.EndedNormally = finalResponse != nil
				break
			}
			if err == ErrStreamStall {
				signals.StallCount++
				stallDurationSec := h.stallTimeout.Seconds()
				signals.TotalStallSeconds += stallDurationSec
				signals.Stalled = true
				signals.StallDurationMs = int(h.stallTimeout.Milliseconds())
				signals.EndedNormally = false
				h.finalizeStreamSignals(signals, gapTracker, firstEventTime, startTime)
				return nil, signals, NewStreamStallError(signals.StallDurationMs)
			}
			signals.EndedNormally = false
			h.finalizeStreamSignals(signals, gapTracker, firstEventTime, startTime)
			return nil, signals, err
		}

		now := time.Now()

		if firstEventTime == nil {
			firstEventTime = &now
			signals.StreamConnectMs = now.Sub(startTime).Milliseconds()
			signals.TimeToFirstEventMs = signals.StreamConnectMs
		}

		if lastEventTime != nil {
			gapMs := now.Sub(*lastEventTime).Milliseconds()
			gapTracker.recordGap(gapMs)
		}
		lastEventTime = &now

		signals.EventsCount++

		if event.Data == "" {
			continue
		}

		var msg JSONRPCResponse
		if err := json.Unmarshal([]byte(event.Data), &msg); err != nil {
			var notification struct {
				JSONRPC string `json:"jsonrpc"`
				Method  string `json:"method"`
			}
			if json.Unmarshal([]byte(event.Data), &notification) == nil && notification.Method != "" {
				notifications = append(notifications, json.RawMessage(event.Data))
				continue
			}
			h.finalizeStreamSignals(signals, gapTracker, firstEventTime, startTime)
			return nil, signals, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
		}

		if msg.ID != nil {
			idStr := fmt.Sprintf("%v", msg.ID)
			if idStr == requestID {
				finalResponse = &msg
				signals.EndedNormally = true
				break
			}
		}

		if msg.Result == nil && msg.Error == nil {
			notifications = append(notifications, json.RawMessage(event.Data))
		}
	}

	if finalResponse == nil {
		signals.EndedNormally = false
		h.finalizeStreamSignals(signals, gapTracker, firstEventTime, startTime)
		return nil, signals, fmt.Errorf("stream ended without final response for request %s", requestID)
	}

	_ = notifications
	h.finalizeStreamSignals(signals, gapTracker, firstEventTime, startTime)

	return finalResponse, signals, nil
}

func (h *SSEResponseHandler) finalizeStreamSignals(
	signals *StreamSignals,
	gapTracker *eventGapTracker,
	firstEventTime *time.Time,
	startTime time.Time,
) {
	if gapTracker.count > 0 {
		signals.EventGapHistogram = gapTracker.buildHistogram()
	}
}

type eventGapTracker struct {
	gaps  []int64
	count int
	sum   int64
	min   int64
	max   int64
}

const eventGapTrackerMinUnset = -1

func newEventGapTracker() *eventGapTracker {
	return &eventGapTracker{
		gaps: make([]int64, 0, 100),
		min:  eventGapTrackerMinUnset,
	}
}

func (t *eventGapTracker) recordGap(gapMs int64) {
	t.gaps = append(t.gaps, gapMs)
	t.count++
	t.sum += gapMs

	if t.min == eventGapTrackerMinUnset || gapMs < t.min {
		t.min = gapMs
	}
	if gapMs > t.max {
		t.max = gapMs
	}
}

func (t *eventGapTracker) buildHistogram() *EventGapHistogram {
	if t.count == 0 {
		return nil
	}

	hist := &EventGapHistogram{
		MinGapMs: t.min,
		MaxGapMs: t.max,
		AvgGapMs: float64(t.sum) / float64(t.count),
	}

	for _, gap := range t.gaps {
		switch {
		case gap < 10:
			hist.Under10ms++
		case gap < 50:
			hist.From10to50++
		case gap < 100:
			hist.From50to100++
		case gap < 500:
			hist.From100to500++
		case gap < 1000:
			hist.From500to1000++
		default:
			hist.Over1000ms++
		}
	}

	if len(t.gaps) > 0 {
		sorted := make([]int64, len(t.gaps))
		copy(sorted, t.gaps)
		sortInt64Slice(sorted)

		hist.P50GapMs = percentile(sorted, 50)
		hist.P95GapMs = percentile(sorted, 95)
		hist.P99GapMs = percentile(sorted, 99)
	}

	return hist
}

func sortInt64Slice(s []int64) {
	n := len(s)
	if n <= 20 {
		insertionSortInt64(s)
		return
	}
	quicksortInt64(s, 0, n-1)
}

func insertionSortInt64(s []int64) {
	for i := 1; i < len(s); i++ {
		key := s[i]
		j := i - 1
		for j >= 0 && s[j] > key {
			s[j+1] = s[j]
			j--
		}
		s[j+1] = key
	}
}

func quicksortInt64(s []int64, lo, hi int) {
	if lo < hi {
		p := partitionInt64(s, lo, hi)
		quicksortInt64(s, lo, p-1)
		quicksortInt64(s, p+1, hi)
	}
}

func partitionInt64(s []int64, lo, hi int) int {
	pivot := s[hi]
	i := lo - 1
	for j := lo; j < hi; j++ {
		if s[j] <= pivot {
			i++
			s[i], s[j] = s[j], s[i]
		}
	}
	s[i+1], s[hi] = s[hi], s[i+1]
	return i + 1
}

func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}

	rank := float64(p) / 100.0 * float64(len(sorted)-1)
	lower := int(rank)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}

	weight := rank - float64(lower)
	return sorted[lower] + int64(weight*float64(sorted[upper]-sorted[lower]))
}

func ParseSSEData(data string) (*JSONRPCResponse, error) {
	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	return &resp, nil
}

func IsNotification(data []byte) bool {
	var msg struct {
		ID     interface{} `json:"id"`
		Method string      `json:"method"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return false
	}
	return msg.ID == nil && msg.Method != ""
}

type BufferedSSEReader struct {
	events []*SSEEvent
	idx    int
	mu     sync.Mutex
}

func NewBufferedSSEReader(events []*SSEEvent) *BufferedSSEReader {
	return &BufferedSSEReader{
		events: events,
	}
}

func (r *BufferedSSEReader) ReadEvent() (*SSEEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.idx >= len(r.events) {
		return nil, io.EOF
	}

	event := r.events[r.idx]
	r.idx++
	return event, nil
}

func (r *BufferedSSEReader) Close() error {
	return nil
}

func ParseSSEFromBytes(data []byte) ([]*SSEEvent, error) {
	var events []*SSEEvent
	reader := bufio.NewReader(bytes.NewReader(data))

	var currentEvent *SSEEvent
	var dataLines []string

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}

		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		if line == "" {
			if currentEvent != nil || len(dataLines) > 0 {
				if currentEvent == nil {
					currentEvent = &SSEEvent{}
				}
				currentEvent.Data = strings.Join(dataLines, "\n")
				events = append(events, currentEvent)
				currentEvent = nil
				dataLines = nil
			}
			if err == io.EOF {
				break
			}
			continue
		}

		if strings.HasPrefix(line, ":") {
			if err == io.EOF {
				break
			}
			continue
		}

		if currentEvent == nil {
			currentEvent = &SSEEvent{}
		}

		colonIdx := strings.Index(line, ":")
		var field, value string
		if colonIdx == -1 {
			field = line
			value = ""
		} else {
			field = line[:colonIdx]
			value = line[colonIdx+1:]
			if strings.HasPrefix(value, " ") {
				value = value[1:]
			}
		}

		switch field {
		case "event":
			currentEvent.Event = value
		case "data":
			dataLines = append(dataLines, value)
		case "id":
			if !strings.Contains(value, "\x00") {
				currentEvent.ID = value
			}
		case "retry":
			if retry, err := strconv.Atoi(value); err == nil {
				currentEvent.Retry = retry
			}
		}

		if err == io.EOF {
			if currentEvent != nil || len(dataLines) > 0 {
				if currentEvent == nil {
					currentEvent = &SSEEvent{}
				}
				currentEvent.Data = strings.Join(dataLines, "\n")
				events = append(events, currentEvent)
			}
			break
		}
	}

	return events, nil
}
