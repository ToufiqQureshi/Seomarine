package sam

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Chunk is one event of the AI SDK UI message stream, the wire format the React
// client's useChat hook reads. See sseSink for framing.
type Chunk map[string]any

// Sink receives the events of one turn.
type Sink interface {
	Send(Chunk) error
}

const (
	// keepAliveInterval keeps proxies from closing a stream that is quiet while
	// a slow tool runs.
	keepAliveInterval = 15 * time.Second
	streamHeader      = "x-vercel-ai-ui-message-stream"
)

// sseSink writes chunks as server-sent events.
type sseSink struct {
	w      http.ResponseWriter
	rc     *http.ResponseController
	mu     sync.Mutex
	closed bool
	stop   chan struct{}
}

// newSSESink starts an event stream on w. ok is false when the writer cannot
// stream, in which case nothing has been written.
//
// The server's write timeout would cut a turn that runs for minutes, so the
// deadline is lifted for this response only. A ResponseController is used
// because the logging middleware wraps the writer without implementing Flusher.
func newSSESink(w http.ResponseWriter) (sink *sseSink, ok bool) {
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		return nil, false
	}
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache, no-transform")
	header.Set("X-Accel-Buffering", "no")
	header.Set(streamHeader, "v1")
	w.WriteHeader(http.StatusOK)
	if err := rc.Flush(); err != nil {
		return nil, false
	}

	sink = &sseSink{w: w, rc: rc, stop: make(chan struct{})}
	go sink.keepAlive()
	return sink, true
}

func (s *sseSink) keepAlive() {
	ticker := time.NewTicker(keepAliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.mu.Lock()
			if !s.closed {
				_, _ = fmt.Fprint(s.w, ": keep-alive\n\n")
				_ = s.rc.Flush()
			}
			s.mu.Unlock()
		}
	}
}

// Send writes one event. It fails once the stream is closed or the client left.
func (s *sseSink) Send(chunk Chunk) error {
	encoded, err := json.Marshal(chunk)
	if err != nil {
		return fmt.Errorf("encode stream chunk: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errStreamClosed
	}
	if _, err := fmt.Fprintf(s.w, "data: %s\n\n", encoded); err != nil {
		return fmt.Errorf("write stream chunk: %w", err)
	}
	_ = s.rc.Flush()
	return nil
}

// Close ends the stream with the terminator the client waits for.
func (s *sseSink) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.stop)
	_, _ = fmt.Fprint(s.w, "data: [DONE]\n\n")
	_ = s.rc.Flush()
}
