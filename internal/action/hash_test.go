package action

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestHash_StableForSameContent(t *testing.T) {
	a := loadFixture(t, "valid.json")
	h1, err := Hash(a)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	h2, err := Hash(a)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("hash not deterministic: %q vs %q", h1, h2)
	}
}

func TestHash_IgnoresWhitespace(t *testing.T) {
	pretty := loadFixture(t, "valid.json")
	var compact bytes.Buffer
	if err := json.Compact(&compact, pretty); err != nil {
		t.Fatalf("compact: %v", err)
	}

	hPretty, err := Hash(pretty)
	if err != nil {
		t.Fatalf("hash pretty: %v", err)
	}
	hCompact, err := Hash(compact.Bytes())
	if err != nil {
		t.Fatalf("hash compact: %v", err)
	}
	if hPretty != hCompact {
		t.Fatalf("whitespace changed hash: pretty=%q compact=%q", hPretty, hCompact)
	}
}

func TestHash_IgnoresKeyOrder(t *testing.T) {
	original := []byte(`{
  "db_instance_identifier": "orders-prod",
  "region": "us-east-1",
  "current_instance_class": "db.r6g.large",
  "target_instance_class": "db.r6g.xlarge",
  "apply_immediately": true,
  "success_criteria": {
    "metric": "CPUUtilization",
    "threshold_percent": 60,
    "verification_window": "5m"
  },
  "reasoning": "CPU sustained at 90% over the last 30 minutes"
}`)
	reordered := []byte(`{
  "reasoning": "CPU sustained at 90% over the last 30 minutes",
  "success_criteria": {
    "verification_window": "5m",
    "threshold_percent": 60,
    "metric": "CPUUtilization"
  },
  "apply_immediately": true,
  "target_instance_class": "db.r6g.xlarge",
  "current_instance_class": "db.r6g.large",
  "region": "us-east-1",
  "db_instance_identifier": "orders-prod"
}`)

	h1, err := Hash(original)
	if err != nil {
		t.Fatalf("hash original: %v", err)
	}
	h2, err := Hash(reordered)
	if err != nil {
		t.Fatalf("hash reordered: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("key order changed hash: original=%q reordered=%q", h1, h2)
	}
}

func TestHash_DifferentContentDifferentHash(t *testing.T) {
	a := loadFixture(t, "valid.json")
	b := loadFixture(t, "threshold_too_high.json")

	ha, err := Hash(a)
	if err != nil {
		t.Fatalf("hash a: %v", err)
	}
	hb, err := Hash(b)
	if err != nil {
		t.Fatalf("hash b: %v", err)
	}
	if ha == hb {
		t.Fatalf("different proposals hashed to same value: %q", ha)
	}
}

func TestHash_NormalizesNumbers(t *testing.T) {
	// 60 vs 60.0 should canonicalize identically (both are float64 60).
	intForm := []byte(`{"x": 60}`)
	floatForm := []byte(`{"x": 60.0}`)
	h1, err := Hash(intForm)
	if err != nil {
		t.Fatalf("hash int form: %v", err)
	}
	h2, err := Hash(floatForm)
	if err != nil {
		t.Fatalf("hash float form: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("60 vs 60.0 hashed differently: %q vs %q", h1, h2)
	}
}

func TestHash_RejectsMalformedJSON(t *testing.T) {
	if _, err := Hash([]byte("{not json")); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestCanonicalize_NoTrailingNewline(t *testing.T) {
	out, err := Canonicalize([]byte(`{"a":1}`))
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if len(out) > 0 && out[len(out)-1] == '\n' {
		t.Fatalf("canonical form has trailing newline: %q", out)
	}
}
