package interpreter

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/plan"
)

// evalPredicate returns nil if the predicate holds against body, or
// a descriptive error explaining why it failed. The error is the
// audit-log payload for verification failure, so it is structured
// enough for a human to read but plain enough to embed in JSON.
func evalPredicate(body map[string]any, p plan.Predicate) error {
	got, found := lookupPath(body, p.Path)

	switch p.Operator {
	case "eq":
		if !found {
			return fmt.Errorf("path %q: not found", p.Path)
		}
		if !equal(got, p.Value) {
			return fmt.Errorf("path %q: got %v, want %v", p.Path, got, p.Value)
		}
	case "ne":
		if found && equal(got, p.Value) {
			return fmt.Errorf("path %q: got %v, want != %v", p.Path, got, p.Value)
		}
	case "empty":
		if found && !isEmpty(got) {
			return fmt.Errorf("path %q: got %v, want empty", p.Path, got)
		}
	case "not_empty":
		if !found || isEmpty(got) {
			return fmt.Errorf("path %q: got %v, want non-empty", p.Path, got)
		}
	case "lte":
		if !found {
			return fmt.Errorf("path %q: not found", p.Path)
		}
		gn, err := toFloat(got)
		if err != nil {
			return fmt.Errorf("path %q: not numeric: %v", p.Path, got)
		}
		wn, err := toFloat(p.Value)
		if err != nil {
			return fmt.Errorf("path %q: predicate value not numeric: %v", p.Path, p.Value)
		}
		if gn > wn {
			return fmt.Errorf("path %q: got %v, want <= %v", p.Path, gn, wn)
		}
	default:
		return fmt.Errorf("unknown operator %q", p.Operator)
	}
	return nil
}

// lookupPath navigates a dotted path with optional [N] array indices.
// Example: "DBInstances[0].DBInstanceStatus".
func lookupPath(root any, path string) (any, bool) {
	if path == "" {
		return root, true
	}
	cur := any(root)
	for _, seg := range strings.Split(path, ".") {
		// Split off any [N] suffixes from the segment.
		key, indices := parseSegment(seg)
		if key != "" {
			m, ok := cur.(map[string]any)
			if !ok {
				return nil, false
			}
			v, ok := m[key]
			if !ok {
				return nil, false
			}
			cur = v
		}
		for _, idx := range indices {
			arr, ok := cur.([]any)
			if !ok {
				return nil, false
			}
			if idx < 0 || idx >= len(arr) {
				return nil, false
			}
			cur = arr[idx]
		}
	}
	return cur, true
}

// parseSegment splits "Foo[0][1]" into ("Foo", [0, 1]).
func parseSegment(seg string) (string, []int) {
	open := strings.Index(seg, "[")
	if open == -1 {
		return seg, nil
	}
	key := seg[:open]
	rest := seg[open:]
	var indices []int
	for len(rest) > 0 && rest[0] == '[' {
		end := strings.Index(rest, "]")
		if end == -1 {
			break
		}
		n, err := strconv.Atoi(rest[1:end])
		if err != nil {
			break
		}
		indices = append(indices, n)
		rest = rest[end+1:]
	}
	return key, indices
}

func equal(a, b any) bool {
	// JSON numbers come back as float64; allow numeric-equal across types.
	if af, aerr := toFloat(a); aerr == nil {
		if bf, berr := toFloat(b); berr == nil {
			return af == bf
		}
	}
	return reflect.DeepEqual(a, b)
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case string:
		return x == ""
	case map[string]any:
		return len(x) == 0
	case []any:
		return len(x) == 0
	}
	return false
}

func toFloat(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int:
		return float64(x), nil
	case int32:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case string:
		return strconv.ParseFloat(x, 64)
	}
	return 0, fmt.Errorf("not numeric: %T", v)
}
