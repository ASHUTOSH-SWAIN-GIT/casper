package interpreter

import (
	"testing"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/plan"
)

func TestEvalPredicate_Eq(t *testing.T) {
	body := map[string]any{"x": "available"}
	if err := evalPredicate(body, plan.Predicate{Path: "x", Operator: "eq", Value: "available"}); err != nil {
		t.Errorf("eq match: %v", err)
	}
	if err := evalPredicate(body, plan.Predicate{Path: "x", Operator: "eq", Value: "modifying"}); err == nil {
		t.Error("expected eq mismatch error")
	}
}

func TestEvalPredicate_Empty(t *testing.T) {
	body := map[string]any{
		"empty_map":   map[string]any{},
		"empty_list":  []any{},
		"empty_str":   "",
		"full_map":    map[string]any{"a": 1},
		"all_nil_map": map[string]any{"a": nil, "b": nil, "c": nil},
		// Mirrors the shape AWS returns for PendingModifiedValues with
		// no actual pending changes — populated keys, all nil values.
		"aws_pending_empty": map[string]any{
			"DBInstanceClass":  nil,
			"AllocatedStorage": nil,
			"MultiAZ":          nil,
			"EngineVersion":    nil,
		},
		"aws_pending_real": map[string]any{
			"DBInstanceClass":  "db.r6g.xlarge",
			"AllocatedStorage": nil,
		},
		"nested_empty": map[string]any{
			"outer": map[string]any{"inner": nil},
		},
	}
	cases := []struct {
		path string
		want bool
	}{
		{"empty_map", true},
		{"empty_list", true},
		{"empty_str", true},
		{"full_map", false},
		{"all_nil_map", true},        // map of all-nil values is empty
		{"aws_pending_empty", true},  // the actual bug we hit on a real AWS response
		{"aws_pending_real", false},  // one populated field → not empty
		{"nested_empty", true},       // recurses
	}
	for _, c := range cases {
		err := evalPredicate(body, plan.Predicate{Path: c.path, Operator: "empty"})
		if (err == nil) != c.want {
			t.Errorf("empty %q: err=%v want pass=%v", c.path, err, c.want)
		}
	}
}

func TestEvalPredicate_Lte(t *testing.T) {
	body := map[string]any{"avg": 35.0}
	if err := evalPredicate(body, plan.Predicate{Path: "avg", Operator: "lte", Value: 60.0}); err != nil {
		t.Errorf("35 <= 60: %v", err)
	}
	if err := evalPredicate(body, plan.Predicate{Path: "avg", Operator: "lte", Value: 30.0}); err == nil {
		t.Error("expected 35 <= 30 to fail")
	}
}

func TestLookupPath_NestedAndArray(t *testing.T) {
	body := map[string]any{
		"DBInstances": []any{
			map[string]any{
				"DBInstanceStatus": "available",
			},
		},
	}
	got, ok := lookupPath(body, "DBInstances[0].DBInstanceStatus")
	if !ok || got != "available" {
		t.Errorf("got %v ok=%v", got, ok)
	}

	if _, ok := lookupPath(body, "DBInstances[5].DBInstanceStatus"); ok {
		t.Error("expected out-of-range to return ok=false")
	}
	if _, ok := lookupPath(body, "Nope"); ok {
		t.Error("expected missing key to return ok=false")
	}
}
