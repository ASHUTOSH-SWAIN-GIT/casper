package awsx

import (
	"strings"
	"testing"
	"time"
)

func TestExpandWindow_AddsStartAndEndAndStripsWindow(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC) }
	in := map[string]any{
		"Namespace":  "AWS/RDS",
		"MetricName": "CPUUtilization",
		"Window":     "5m",
	}
	out, err := expandWindow(in, now)
	if err != nil {
		t.Fatalf("expandWindow: %v", err)
	}
	if _, present := out["Window"]; present {
		t.Error("Window should be stripped from output")
	}
	if got := out["EndTime"]; got != "2026-04-29T12:00:00Z" {
		t.Errorf("EndTime: got %v", got)
	}
	if got := out["StartTime"]; got != "2026-04-29T11:55:00Z" {
		t.Errorf("StartTime: got %v", got)
	}
	// Other params untouched.
	if got := out["Namespace"]; got != "AWS/RDS" {
		t.Errorf("Namespace mutated: %v", got)
	}
}

func TestExpandWindow_NoWindowIsPassThrough(t *testing.T) {
	in := map[string]any{"Namespace": "AWS/RDS"}
	out, err := expandWindow(in, time.Now)
	if err != nil {
		t.Fatalf("expandWindow: %v", err)
	}
	if &out == &in {
		// Same map identity is fine; just sanity-check contents.
	}
	if out["Namespace"] != "AWS/RDS" {
		t.Errorf("namespace: %v", out["Namespace"])
	}
}

func TestExpandWindow_RejectsBadDuration(t *testing.T) {
	_, err := expandWindow(map[string]any{"Window": "not-a-duration"}, time.Now)
	if err == nil || !strings.Contains(err.Error(), "invalid Window") {
		t.Fatalf("expected invalid-duration error, got: %v", err)
	}
}

func TestAverageDatapoints(t *testing.T) {
	dps := []any{
		map[string]any{"Average": 30.0},
		map[string]any{"Average": 50.0},
		map[string]any{"Average": 40.0},
	}
	if got := averageDatapoints(dps); got != 40.0 {
		t.Errorf("got %v want 40.0", got)
	}
	if got := averageDatapoints(nil); got != 0 {
		t.Errorf("empty: got %v want 0", got)
	}
	// Skips entries without Average.
	mixed := []any{
		map[string]any{"Average": 100.0},
		map[string]any{"Maximum": 200.0},
	}
	if got := averageDatapoints(mixed); got != 100.0 {
		t.Errorf("mixed: got %v want 100.0", got)
	}
}

func TestRemarshal(t *testing.T) {
	type S struct {
		A string `json:"a"`
		B int    `json:"b"`
	}
	in := map[string]any{"a": "hello", "b": 42}
	var s S
	if err := remarshal(in, &s); err != nil {
		t.Fatalf("remarshal: %v", err)
	}
	if s.A != "hello" || s.B != 42 {
		t.Errorf("got %+v", s)
	}
}
