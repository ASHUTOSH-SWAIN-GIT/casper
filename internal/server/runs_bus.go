package server

import (
	"sync"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/audit"
)

// runEventBus is a tiny per-run pub/sub used to fan out audit events
// to SSE subscribers. One channel per subscriber so a slow client
// can't block the executor — non-blocking sends drop events the
// subscriber missed (replay from the audit store covers the gap).
type runEventBus struct {
	mu          sync.Mutex
	subscribers map[string][]chan audit.Event // runID -> subscriber channels
	finished    map[string]bool               // runID -> true when no more events expected
}

func newRunEventBus() *runEventBus {
	return &runEventBus{
		subscribers: make(map[string][]chan audit.Event),
		finished:    make(map[string]bool),
	}
}

// subscribe registers a buffered channel for a runID. Returns the
// channel and an unsubscribe function. Callers should defer the
// unsubscribe to avoid leaking goroutines.
func (b *runEventBus) subscribe(runID string) (<-chan audit.Event, func()) {
	ch := make(chan audit.Event, 64)
	b.mu.Lock()
	b.subscribers[runID] = append(b.subscribers[runID], ch)
	b.mu.Unlock()

	unsub := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		subs := b.subscribers[runID]
		for i, c := range subs {
			if c == ch {
				b.subscribers[runID] = append(subs[:i], subs[i+1:]...)
				close(ch)
				return
			}
		}
	}
	return ch, unsub
}

// publish fans the event out to every subscriber on runID. Non-blocking:
// a full channel drops the event for that subscriber so a slow consumer
// doesn't stall the executor. SSE clients re-fetch the audit history on
// reconnect so dropped events aren't lost permanently.
func (b *runEventBus) publish(runID string, ev audit.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subscribers[runID] {
		select {
		case ch <- ev:
		default:
		}
	}
}

// finish signals that no more events are coming for runID; subscribers
// should close their SSE response after draining their channel. We
// publish a sentinel zero-Event with Kind="run_finished" via a separate
// channel close — see the SSE handler for the loop.
func (b *runEventBus) finish(runID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.finished[runID] = true
	for _, ch := range b.subscribers[runID] {
		// Push an explicit terminator so the SSE handler can break
		// without a separate signal.
		select {
		case ch <- audit.Event{Kind: "run_finished"}:
		default:
		}
	}
}

// isFinished reports whether finish has been called for runID. Used by
// the SSE handler when a client connects after the run has already
// completed — we replay history then close immediately.
func (b *runEventBus) isFinished(runID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.finished[runID]
}
