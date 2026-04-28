package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildCLI compiles casperctl into a temp binary so the tests can shell
// out to it. Done once per test run via testing.M would be cleaner but
// for one binary on one platform, this is plenty.
func buildCLI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "casperctl")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build cli: %v\n%s", err, out)
	}
	return bin
}

func fixture(name string) string {
	return filepath.Join("..", "..", "testdata", "proposals", name)
}

func runCLI(t *testing.T, bin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var so, se bytes.Buffer
	cmd := exec.Command(bin, args...)
	cmd.Stdout = &so
	cmd.Stderr = &se
	err := cmd.Run()
	code = 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("run cli: %v", err)
	}
	return so.String(), se.String(), code
}

func TestCLI_ValidateOK(t *testing.T) {
	bin := buildCLI(t)
	out, _, code := runCLI(t, bin, "validate", fixture("valid.json"))
	if code != 0 {
		t.Fatalf("exit code: got %d want 0", code)
	}
	if !strings.Contains(out, "ok") {
		t.Errorf("stdout: %q", out)
	}
}

func TestCLI_ValidateRejects(t *testing.T) {
	bin := buildCLI(t)
	_, errOut, code := runCLI(t, bin, "validate", fixture("threshold_too_high.json"))
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	if !strings.Contains(errOut, "threshold_percent") {
		t.Errorf("stderr did not mention threshold_percent: %q", errOut)
	}
}

func TestCLI_HashIsHex(t *testing.T) {
	bin := buildCLI(t)
	out, _, code := runCLI(t, bin, "hash", fixture("valid.json"))
	if code != 0 {
		t.Fatalf("exit code: got %d want 0", code)
	}
	h := strings.TrimSpace(out)
	if len(h) != 64 {
		t.Errorf("hash length: got %d want 64 (%q)", len(h), h)
	}
}

func TestCLI_CompileEmitsBothPlans(t *testing.T) {
	bin := buildCLI(t)
	out, _, code := runCLI(t, bin, "compile", fixture("valid.json"))
	if code != 0 {
		t.Fatalf("exit code: got %d want 0", code)
	}
	for _, want := range []string{`"forward"`, `"rollback"`, `"proposal_hash"`, "ModifyDBInstance"} {
		if !strings.Contains(out, want) {
			t.Errorf("compile output missing %q", want)
		}
	}
}

func TestCLI_UnknownCommand(t *testing.T) {
	bin := buildCLI(t)
	_, _, code := runCLI(t, bin, "nope")
	if code == 0 {
		t.Fatal("expected non-zero exit for unknown command")
	}
}
