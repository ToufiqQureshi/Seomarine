// Package spendtest is a spend.Guard for tests of code that sends billed
// provider requests. It allows every request and remembers what was held and
// settled, so a test can assert that a request was paid for.
package spendtest

import (
	"context"
	"sync"

	"github.com/toufiqqureshi/uplumo/backend/internal/platform/spend"
)

// Guard allows every request unless Refuse is set.
type Guard struct {
	// Refuse, when set, is returned by every Hold.
	Refuse error

	mu      sync.Mutex
	held    []spend.Request
	settled []float64
}

// Hold records req and allows it, or returns Refuse.
func (g *Guard) Hold(_ context.Context, req spend.Request) (spend.Hold, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Refuse != nil {
		return nil, g.Refuse
	}
	g.held = append(g.held, req)
	return hold{guard: g}, nil
}

// Held returns the requests held so far.
func (g *Guard) Held() []spend.Request {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]spend.Request(nil), g.held...)
}

// Settled returns the costs settled so far, in order.
func (g *Guard) Settled() []float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]float64(nil), g.settled...)
}

type hold struct{ guard *Guard }

func (h hold) Settle(_ context.Context, costUSD float64) error {
	h.guard.mu.Lock()
	defer h.guard.mu.Unlock()
	h.guard.settled = append(h.guard.settled, costUSD)
	return nil
}
