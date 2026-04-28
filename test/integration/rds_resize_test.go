//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/awsx"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/interpreter"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/plan"
)

// TestRDSResize_RealAWS runs the forward plan against a real RDS instance
// then runs the rollback plan to restore it. Requires:
//   - AWS credentials in env (AWS_PROFILE / AWS_ACCESS_KEY_ID / etc.)
//   - CASPER_TEST_DB_INSTANCE: identifier of a throwaway instance
//   - CASPER_TEST_REGION:      region the instance lives in
//   - CASPER_TEST_CURRENT_CLASS: instance class right now (e.g. db.t4g.medium)
//   - CASPER_TEST_TARGET_CLASS:  instance class to resize to (e.g. db.t4g.large)
//
// **This will modify and roll back a real RDS instance.** Do not point
// CASPER_TEST_DB_INSTANCE at anything that holds data you care about.
func TestRDSResize_RealAWS(t *testing.T) {
	id := os.Getenv("CASPER_TEST_DB_INSTANCE")
	region := os.Getenv("CASPER_TEST_REGION")
	current := os.Getenv("CASPER_TEST_CURRENT_CLASS")
	target := os.Getenv("CASPER_TEST_TARGET_CLASS")
	if id == "" || region == "" || current == "" || target == "" {
		t.Skip("set CASPER_TEST_DB_INSTANCE, CASPER_TEST_REGION, CASPER_TEST_CURRENT_CLASS, CASPER_TEST_TARGET_CLASS")
	}

	prop := action.RDSResizeProposal{
		DBInstanceIdentifier: id,
		Region:               region,
		CurrentInstanceClass: current,
		TargetInstanceClass:  target,
		ApplyImmediately:     true,
		SuccessCriteria: action.SuccessCriteria{
			Metric:             "CPUUtilization",
			ThresholdPercent:   100, // permissive — we're testing mechanics, not load
			VerificationWindow: "1m",
		},
		Reasoning: "integration test",
	}
	raw, err := json.Marshal(prop)
	if err != nil {
		t.Fatalf("marshal proposal: %v", err)
	}
	if err := action.Validate(raw); err != nil {
		t.Fatalf("proposal invalid: %v", err)
	}
	h, err := action.Hash(raw)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	fwd, rb := plan.CompileRDSResize(prop, h)

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		t.Fatalf("load aws config: %v", err)
	}
	interp := &interpreter.Interpreter{Client: awsx.New(cfg)}

	t.Logf("running forward plan (%d steps)", len(fwd.Steps))
	fwdResults, fwdErr := interp.Run(ctx, fwd)
	for _, r := range fwdResults {
		t.Logf("  forward: %-30s %-7s (%d calls)", r.StepID, r.Status, len(r.Calls))
	}
	if fwdErr != nil {
		t.Errorf("forward plan failed: %v", fwdErr)
	}

	// Always run rollback so we leave the instance as we found it.
	t.Logf("running rollback plan (%d steps)", len(rb.Steps))
	rbResults, rbErr := interp.Run(ctx, rb)
	for _, r := range rbResults {
		t.Logf("  rollback: %-30s %-7s (%d calls)", r.StepID, r.Status, len(r.Calls))
	}
	if rbErr != nil {
		t.Fatalf("rollback failed (instance may be in unexpected state): %v", rbErr)
	}
	if fwdErr != nil {
		t.Fatalf("forward failed; rollback restored: %v", fwdErr)
	}

	fmt.Fprintln(os.Stderr, "rds resize round-trip completed")
}
