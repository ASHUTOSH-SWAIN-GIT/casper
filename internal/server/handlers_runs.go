package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/audit"
)

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	if s.deps == nil {
		writeError(w, http.StatusServiceUnavailable, "deps_missing", "")
		return
	}
	status := r.URL.Query().Get("status")
	out := s.deps.Runs().list(status)
	writeJSON(w, http.StatusOK, map[string]any{
		"runs":  out,
		"total": len(out),
	})
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	if s.deps == nil {
		writeError(w, http.StatusServiceUnavailable, "deps_missing", "")
		return
	}
	id := r.PathValue("id")
	run, ok := s.deps.Runs().get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "run_not_found", "no run with id "+id)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// handleRunEvents streams audit events for a run as Server-Sent Events.
//
// Behavior:
//   - On connect, replays every event already recorded for the run's
//     proposal_hash, so a late client doesn't miss history.
//   - Then subscribes to live events from the run event bus.
//   - Closes the response when the bus signals the run is finished
//     OR when the client disconnects.
//
// SSE was chosen over WebSockets because it's one-way (server→client),
// auto-reconnects in browsers, and survives proxies/CDNs that don't
// know about the WS protocol.
func (s *Server) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	if s.deps == nil {
		writeError(w, http.StatusServiceUnavailable, "deps_missing", "")
		return
	}
	id := r.PathValue("id")
	run, ok := s.deps.Runs().get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "run_not_found", "no run with id "+id)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "flush_unsupported",
			"streaming not supported by this response writer")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable buffering on proxies
	w.WriteHeader(http.StatusOK)

	send := func(event string, payload any) bool {
		b, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		if event != "" {
			if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
				return false
			}
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	// 1. Subscribe BEFORE replaying history so live events that arrive
	// during the replay are queued, not lost.
	ch, unsub := s.deps.Bus().subscribe(id)
	defer unsub()

	// 2. Replay existing history.
	history, err := s.deps.Audit().List(context.Background(), action.ProposalHash(run.ProposalHash))
	if err == nil {
		for _, ev := range history {
			if !send("audit", ev) {
				return
			}
		}
	}

	// 3. If the run already finished before this client connected, send
	// a terminator and exit. Otherwise stream live until the bus closes.
	if s.deps.Bus().isFinished(id) || run.FinishedAt != nil {
		send("done", map[string]any{"run_id": id, "status": run.Status})
		return
	}

	ctx := r.Context()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				// channel closed by unsubscribe — client gone or bus done
				return
			}
			if ev.Kind == "run_finished" {
				updated, _ := s.deps.Runs().get(id)
				status := ""
				if updated != nil {
					status = updated.Status
				}
				send("done", map[string]any{"run_id": id, "status": status})
				return
			}
			if !send("audit", ev) {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// guard ensures audit.Event is referenced even if SSE codepath drifts.
var _ audit.Event
