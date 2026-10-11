package sam

import (
	"context"
	"sync"
	"time"
)

// turns tracks the one running turn each session may have. It is in memory, so
// it assumes a single server instance per deployment (see the README).
type turns struct {
	mu     sync.Mutex
	active map[string]*activeTurn
}

type activeTurn struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func newTurns() *turns { return &turns{active: map[string]*activeTurn{}} }

// begin claims the session for a turn. ok is false when one is already running.
// The returned context is canceled when the client leaves or cancelAndWait runs.
func (t *turns) begin(parent context.Context, sessionID string) (ctx context.Context, release func(), ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, running := t.active[sessionID]; running {
		return nil, nil, false
	}
	ctx, cancel := context.WithCancel(parent)
	turn := &activeTurn{cancel: cancel, done: make(chan struct{})}
	t.active[sessionID] = turn
	return ctx, func() {
		cancel()
		t.mu.Lock()
		delete(t.active, sessionID)
		t.mu.Unlock()
		close(turn.done)
	}, true
}

// cancelAndWait stops the session's running turn, if any, and waits for it to
// finish writing. It reports false when the turn outlived the timeout.
func (t *turns) cancelAndWait(sessionID string, timeout time.Duration) bool {
	t.mu.Lock()
	turn := t.active[sessionID]
	t.mu.Unlock()
	if turn == nil {
		return true
	}
	turn.cancel()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-turn.done:
		return true
	case <-timer.C:
		return false
	}
}
