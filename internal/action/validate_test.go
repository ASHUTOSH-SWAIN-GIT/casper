package action

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join("..", "..", "testdata", "proposals", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func TestValidate_AcceptsValidProposal(t *testing.T) {
	if err := Validate(loadFixture(t, "valid.json")); err != nil {
		t.Fatalf("expected valid proposal to pass, got: %v", err)
	}
}

func TestValidate_RejectsMissingTargetInstanceClass(t *testing.T) {
	err := Validate(loadFixture(t, "missing_target.json"))
	if err == nil {
		t.Fatal("expected error for missing target_instance_class, got nil")
	}
	if !strings.Contains(err.Error(), "target_instance_class") {
		t.Fatalf("expected error to mention target_instance_class, got: %v", err)
	}
}

func TestValidate_RejectsApplyImmediatelyFalse(t *testing.T) {
	err := Validate(loadFixture(t, "apply_immediately_false.json"))
	if err == nil {
		t.Fatal("expected error when apply_immediately=false, got nil")
	}
}

func TestValidate_RejectsThresholdAbove100(t *testing.T) {
	err := Validate(loadFixture(t, "threshold_too_high.json"))
	if err == nil {
		t.Fatal("expected error for threshold_percent > 100, got nil")
	}
}

func TestValidate_RejectsMalformedJSON(t *testing.T) {
	if err := Validate([]byte("{not json")); err == nil {
		t.Fatal("expected parse error for malformed JSON, got nil")
	}
}
