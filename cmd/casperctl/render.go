package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/audit"
)

var renderCmd = &cobra.Command{
	Use:   "render-audit [audit.jsonl]",
	Short: "Render an audit chain (JSONL) as a markdown timeline",
	Long: `Reads an audit chain in JSON-Lines format (the same shape that
'casperctl run' emits to stdout) and produces a human-readable markdown
report — one section per lifecycle event and per step execution, with
AWS request/response summaries and hash-chain integrity status.

Reads from a file path (positional arg) or stdin if no arg given.
Writes markdown to stdout.

Examples:
  casperctl render-audit /tmp/audit.jsonl > /tmp/audit.md
  cat /tmp/audit.jsonl | casperctl render-audit > /tmp/audit.md`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var src io.Reader = os.Stdin
		if len(args) == 1 {
			f, err := os.Open(args[0])
			if err != nil {
				return fmt.Errorf("open audit file: %w", err)
			}
			defer f.Close()
			src = f
		}
		return renderAudit(src, os.Stdout)
	},
}

func init() { rootCmd.AddCommand(renderCmd) }

func renderAudit(in io.Reader, out io.Writer) error {
	events, err := readAuditEvents(in)
	if err != nil {
		return fmt.Errorf("read events: %w", err)
	}
	if len(events) == 0 {
		return fmt.Errorf("no events found in input")
	}

	// Sort by ID — ensures correct timeline order even if input is shuffled.
	sort.SliceStable(events, func(i, j int) bool { return events[i].ID < events[j].ID })

	w := &mdWriter{w: out}
	writeHeader(w, events)
	writeLifecycle(w, events)
	writeSteps(w, events)
	writeChainSummary(w, events)
	return w.err
}

// readAuditEvents parses one audit.Event per line.
func readAuditEvents(in io.Reader) ([]audit.Event, error) {
	var events []audit.Event
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024) // raise the line cap for large step events
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e audit.Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("parse line %d: %w", len(events)+1, err)
		}
		events = append(events, e)
	}
	return events, sc.Err()
}

// mdWriter is a tiny convenience wrapper that captures the first error.
type mdWriter struct {
	w   io.Writer
	err error
}

func (m *mdWriter) printf(format string, a ...any) {
	if m.err != nil {
		return
	}
	_, m.err = fmt.Fprintf(m.w, format, a...)
}

func (m *mdWriter) println(s string) { m.printf("%s\n", s) }

// writeHeader emits the document header with run-level metadata.
func writeHeader(w *mdWriter, events []audit.Event) {
	first := events[0]
	last := events[len(events)-1]

	// Pull metadata from the "proposed" event (always first).
	var actionType, instance, region, currentClass, targetClass string
	if first.Kind == audit.KindProposed {
		actionType = stringFrom(first.Payload, "action_type")
		instance = stringFrom(first.Payload, "db_instance_identifier")
		region = stringFrom(first.Payload, "region")
		currentClass = stringFrom(first.Payload, "current_instance_class")
		targetClass = stringFrom(first.Payload, "target_instance_class")
	}

	terminalKind := string(last.Kind)
	totalDuration := last.At.Sub(first.At)

	w.println("# Casper Run — Audit Timeline")
	w.println("")
	w.printf("- **Proposal hash:** `%s`\n", first.ProposalHash)
	if actionType != "" {
		w.printf("- **Action type:** `%s`\n", actionType)
	}
	if instance != "" {
		w.printf("- **Instance:** `%s` (region `%s`)\n", instance, region)
	}
	if currentClass != "" && targetClass != "" {
		w.printf("- **Resize:** `%s` → `%s`\n", currentClass, targetClass)
	}
	w.printf("- **Started:** %s\n", first.At.UTC().Format(time.RFC3339))
	w.printf("- **Ended:** %s\n", last.At.UTC().Format(time.RFC3339))
	w.printf("- **Duration:** %s\n", totalDuration.Round(time.Millisecond))
	w.printf("- **Total events:** %d\n", len(events))
	w.printf("- **Terminal event:** `%s`\n", terminalKind)
	w.println("")
	w.println("---")
	w.println("")
}

// writeLifecycle prints the run-level lifecycle events (proposed,
// policy_evaluated, credentials_minted, plan_compiled, plan_completed,
// plan_failed, rollback_*) — anything that's not a step_*.
func writeLifecycle(w *mdWriter, events []audit.Event) {
	w.println("## Lifecycle events")
	w.println("")
	for _, e := range events {
		switch e.Kind {
		case audit.KindStepStarted, audit.KindStepCompleted, audit.KindStepFailed:
			continue // step events go in their own section
		}
		writeLifecycleEvent(w, e)
	}
	w.println("---")
	w.println("")
}

func writeLifecycleEvent(w *mdWriter, e audit.Event) {
	w.printf("### %d. `%s` — %s\n", e.ID, e.Kind, e.At.UTC().Format("15:04:05"))
	w.println("")

	switch e.Kind {
	case audit.KindProposed:
		w.printf("- Action type: `%s`\n", stringFrom(e.Payload, "action_type"))
		w.printf("- DB instance: `%s`\n", stringFrom(e.Payload, "db_instance_identifier"))
		w.printf("- Region: `%s`\n", stringFrom(e.Payload, "region"))
		w.printf("- Current class: `%s`\n", stringFrom(e.Payload, "current_instance_class"))
		w.printf("- Target class: `%s`\n", stringFrom(e.Payload, "target_instance_class"))
	case audit.KindPolicyEvaluated:
		w.printf("- Decision: **`%s`**\n", stringFrom(e.Payload, "decision"))
		w.printf("- Reason: %s\n", stringFrom(e.Payload, "reason"))
	case audit.KindCredentialsMinted:
		w.printf("- Role ARN: `%s`\n", stringFrom(e.Payload, "role_arn"))
		w.printf("- Session name: `%s`\n", stringFrom(e.Payload, "session_name"))
		w.printf("- Policy hash: `%s`\n", truncateHash(stringFrom(e.Payload, "policy_hash")))
		w.printf("- Expires at: %s\n", stringFrom(e.Payload, "expires_at"))
	case audit.KindPlanCompiled:
		w.printf("- Forward steps: %v\n", e.Payload["forward_steps"])
		w.printf("- Rollback steps: %v\n", e.Payload["rollback_steps"])
	case audit.KindRollbackBegun:
		w.printf("- Reason: %s\n", stringFrom(e.Payload, "reason"))
	case audit.KindRollbackEnded:
		ok := e.Payload["ok"]
		w.printf("- Rollback successful: %v\n", ok)
		if errStr := stringFrom(e.Payload, "error"); errStr != "" {
			w.printf("- Error: %s\n", errStr)
		}
	case audit.KindPlanCompleted:
		w.printf("- Plan kind: `%s`\n", stringFrom(e.Payload, "plan_kind"))
		w.printf("- Action type: `%s`\n", stringFrom(e.Payload, "action_type"))
	case audit.KindPlanFailed:
		w.printf("- Plan kind: `%s`\n", stringFrom(e.Payload, "plan_kind"))
		if errStr := stringFrom(e.Payload, "error"); errStr != "" {
			w.printf("- Error: %s\n", errStr)
		}
	}
	w.printf("- Hash: `%s` (prev: `%s`)\n", truncateHash(e.Hash), truncateHash(e.PrevHash))
	w.println("")
}

// writeSteps groups step_started + step_completed/failed pairs by step
// ID and writes one section per step.
func writeSteps(w *mdWriter, events []audit.Event) {
	type stepBundle struct {
		started   *audit.Event
		completed *audit.Event
	}
	bundles := []*stepBundle{}
	byID := map[string]*stepBundle{}

	for i := range events {
		e := &events[i]
		switch e.Kind {
		case audit.KindStepStarted:
			id := stringFrom(e.Payload, "step_id")
			b := &stepBundle{started: e}
			byID[id] = b
			bundles = append(bundles, b)
		case audit.KindStepCompleted, audit.KindStepFailed:
			id := stringFrom(e.Payload, "step_id")
			if b := byID[id]; b != nil {
				b.completed = e
			}
		}
	}

	if len(bundles) == 0 {
		return
	}

	w.println("## Step execution")
	w.println("")
	for _, b := range bundles {
		writeStep(w, b.started, b.completed)
	}
	w.println("---")
	w.println("")
}

func writeStep(w *mdWriter, started, completed *audit.Event) {
	if started == nil {
		return
	}
	stepID := stringFrom(started.Payload, "step_id")
	stepKind := stringFrom(started.Payload, "step_kind")
	desc := stringFrom(started.Payload, "description")

	statusStr := "running (no completion event)"
	if completed != nil {
		statusStr = stringFrom(completed.Payload, "status")
	}

	w.printf("### `%s` (`%s`) — **%s**\n", stepID, stepKind, statusStr)
	w.println("")
	if desc != "" {
		w.printf("> %s\n\n", desc)
	}
	w.printf("- Started: %s\n", started.At.UTC().Format("15:04:05.000"))
	if completed != nil {
		w.printf("- Ended: %s\n", completed.At.UTC().Format("15:04:05.000"))
		// Compute duration from event timestamps (audit-side) rather
		// than the duration_ms payload field (interpreter-side), which
		// has a known zero-FinishedAt issue in older runs.
		dur := completed.At.Sub(started.At).Round(time.Millisecond)
		w.printf("- Duration: %s\n", dur)
		if errStr := stringFrom(completed.Payload, "error"); errStr != "" {
			w.printf("- **Error:** %s\n", errStr)
		}
		writeStepCalls(w, completed)
	}
	w.println("")
}

// writeStepCalls renders the AWS calls captured during a step (one per
// API call for aws_api_call, many for poll, optional for verify).
func writeStepCalls(w *mdWriter, completed *audit.Event) {
	calls, ok := completed.Payload["calls"].([]any)
	if !ok || len(calls) == 0 {
		return
	}
	w.printf("- AWS calls: %d\n", len(calls))
	for i, c := range calls {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		req, _ := m["request"].(map[string]any)
		reqID := stringFrom(m, "aws_request_id")
		errStr := stringFrom(m, "error")

		op := ""
		svc := ""
		if req != nil {
			op = stringFrom(req, "operation")
			svc = stringFrom(req, "service")
		}

		label := fmt.Sprintf("`%s.%s`", svc, op)
		if reqID != "" {
			label += fmt.Sprintf(" (request id `%s`)", reqID)
		}
		if errStr != "" {
			label += fmt.Sprintf(" — error: %s", errStr)
		}
		w.printf("  %d. %s\n", i+1, label)
	}
}

// writeChainSummary prints the hash-chain endpoint info.
func writeChainSummary(w *mdWriter, events []audit.Event) {
	first := events[0]
	last := events[len(events)-1]
	w.println("## Hash chain")
	w.println("")
	w.printf("- Genesis prev_hash: `%s` (should be empty)\n", first.PrevHash)
	w.printf("- Genesis hash: `%s`\n", truncateHash(first.Hash))
	w.printf("- Final hash: `%s`\n", truncateHash(last.Hash))
	w.printf("- Chain length: %d events\n", len(events))
	w.println("")
	w.println("If `casperctl run` printed `audit log: N events, chain verified` to")
	w.println("stderr, every event's `prev_hash` matched the previous event's `hash`")
	w.println("and every event's `hash` matched `sha256(prev_hash || canonical(payload))`.")
	w.println("Tampering with any event's payload would have broken the chain forward")
	w.println("of that event.")
	w.println("")
}

// helpers

func stringFrom(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func truncateHash(s string) string {
	if len(s) <= 16 {
		return s
	}
	return s[:16] + "…"
}
