package server

import (
	"context"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/audit"
)

// auditTee wraps an audit.Store and, on every successful Append,
// publishes the appended event to the run event bus under a specific
// runID. This is how the executor's audit writes turn into SSE events
// for the dashboard without the executor needing to know SSE exists.
type auditTee struct {
	inner audit.Store
	bus   *runEventBus
	runID string
}

// teeForRun returns a Store that decorates inner with a side-channel
// publish to bus under runID. The inner store remains the source of
// truth — chain verification still works and replay is unaffected.
func teeForRun(inner audit.Store, bus *runEventBus, runID string) audit.Store {
	return &auditTee{inner: inner, bus: bus, runID: runID}
}

func (t *auditTee) Append(ctx context.Context, kind audit.Kind, h action.ProposalHash, payload map[string]any) (audit.Event, error) {
	ev, err := t.inner.Append(ctx, kind, h, payload)
	if err == nil && t.bus != nil {
		t.bus.publish(t.runID, ev)
	}
	return ev, err
}

func (t *auditTee) List(ctx context.Context, h action.ProposalHash) ([]audit.Event, error) {
	return t.inner.List(ctx, h)
}

func (t *auditTee) Verify(ctx context.Context) error {
	return t.inner.Verify(ctx)
}
