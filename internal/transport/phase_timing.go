package transport

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"
)

type phaseTimingTracker struct {
	mu sync.Mutex

	startTime        time.Time
	dnsStart         time.Time
	dnsEnd           time.Time
	connectStart     time.Time
	connectEnd       time.Time
	tlsStart         time.Time
	tlsEnd           time.Time
	gotFirstByte     time.Time
	gotConn          time.Time
	connectionReused bool
	wroteRequest     time.Time
}

func newPhaseTimingTracker() *phaseTimingTracker {
	return &phaseTimingTracker{
		startTime: time.Now(),
	}
}

func (t *phaseTimingTracker) createClientTrace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		DNSStart: func(info httptrace.DNSStartInfo) {
			t.mu.Lock()
			t.dnsStart = time.Now()
			t.mu.Unlock()
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			t.mu.Lock()
			t.dnsEnd = time.Now()
			t.mu.Unlock()
		},
		ConnectStart: func(network, addr string) {
			t.mu.Lock()
			t.connectStart = time.Now()
			t.mu.Unlock()
		},
		ConnectDone: func(network, addr string, err error) {
			t.mu.Lock()
			t.connectEnd = time.Now()
			t.mu.Unlock()
		},
		TLSHandshakeStart: func() {
			t.mu.Lock()
			t.tlsStart = time.Now()
			t.mu.Unlock()
		},
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			t.mu.Lock()
			t.tlsEnd = time.Now()
			t.mu.Unlock()
		},
		GotConn: func(info httptrace.GotConnInfo) {
			t.mu.Lock()
			t.gotConn = time.Now()
			t.connectionReused = info.Reused
			t.mu.Unlock()
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			t.mu.Lock()
			t.wroteRequest = time.Now()
			t.mu.Unlock()
		},
		GotFirstResponseByte: func() {
			t.mu.Lock()
			t.gotFirstByte = time.Now()
			t.mu.Unlock()
		},
	}
}

func (t *phaseTimingTracker) computePhaseTiming(endTime time.Time) *PhaseTiming {
	t.mu.Lock()
	defer t.mu.Unlock()

	pt := &PhaseTiming{
		ConnectionReused: t.connectionReused,
		E2EMs:            endTime.Sub(t.startTime).Milliseconds(),
	}

	if !t.connectionReused {
		if !t.dnsStart.IsZero() && !t.dnsEnd.IsZero() {
			pt.DNSMs = t.dnsEnd.Sub(t.dnsStart).Milliseconds()
		}
		if !t.connectStart.IsZero() && !t.connectEnd.IsZero() {
			pt.TCPConnectMs = t.connectEnd.Sub(t.connectStart).Milliseconds()
		}
		if !t.tlsStart.IsZero() && !t.tlsEnd.IsZero() {
			pt.TLSHandshakeMs = t.tlsEnd.Sub(t.tlsStart).Milliseconds()
		}
	}

	if !t.gotFirstByte.IsZero() {
		baseline := t.startTime
		if !t.wroteRequest.IsZero() {
			baseline = t.wroteRequest
		} else if !t.gotConn.IsZero() {
			baseline = t.gotConn
		}
		pt.TTFBMs = t.gotFirstByte.Sub(baseline).Milliseconds()
		pt.DownloadMs = endTime.Sub(t.gotFirstByte).Milliseconds()
	}

	return pt
}

func addPhaseTimingToRequest(req *http.Request, tracker *phaseTimingTracker) *http.Request {
	trace := tracker.createClientTrace()
	ctx := httptrace.WithClientTrace(req.Context(), trace)
	return req.WithContext(ctx)
}

func createTracedContext(ctx context.Context) (context.Context, *phaseTimingTracker) {
	tracker := newPhaseTimingTracker()
	trace := tracker.createClientTrace()
	return httptrace.WithClientTrace(ctx, trace), tracker
}
