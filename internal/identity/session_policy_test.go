package identity

import (
	"strings"
	"testing"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

func sampleProposal() action.RDSResizeProposal {
	return action.RDSResizeProposal{
		DBInstanceIdentifier: "orders-prod",
		Region:               "us-east-1",
		CurrentInstanceClass: "db.r6g.large",
		TargetInstanceClass:  "db.r6g.xlarge",
		ApplyImmediately:     true,
		SuccessCriteria: action.SuccessCriteria{
			Metric:             "CPUUtilization",
			ThresholdPercent:   60,
			VerificationWindow: "5m",
		},
		Reasoning: "test",
	}
}

func TestBuildRDSResizePolicy_NarrowsModifyToInstanceARN(t *testing.T) {
	p := BuildRDSResizePolicy(sampleProposal(), "123456789012")

	wantARN := "arn:aws:rds:us-east-1:123456789012:db:orders-prod"
	var modifyStmt *Statement
	for i := range p.Statement {
		s := &p.Statement[i]
		if len(s.Action) == 1 && s.Action[0] == "rds:ModifyDBInstance" {
			modifyStmt = s
		}
	}
	if modifyStmt == nil {
		t.Fatal("missing rds:ModifyDBInstance statement")
	}
	if modifyStmt.Effect != "Allow" {
		t.Errorf("effect: got %q want Allow", modifyStmt.Effect)
	}
	if len(modifyStmt.Resource) != 1 || modifyStmt.Resource[0] != wantARN {
		t.Errorf("resource: got %v want [%s]", modifyStmt.Resource, wantARN)
	}
}

func TestBuildRDSResizePolicy_HasExpectedShape(t *testing.T) {
	p := BuildRDSResizePolicy(sampleProposal(), "123456789012")
	if p.Version != "2012-10-17" {
		t.Errorf("version: got %q", p.Version)
	}
	if len(p.Statement) != 3 {
		t.Errorf("statement count: got %d want 3", len(p.Statement))
	}
	wantActions := map[string]bool{
		"rds:ModifyDBInstance":          false,
		"rds:DescribeDBInstances":       false,
		"cloudwatch:GetMetricStatistics": false,
	}
	for _, s := range p.Statement {
		for _, a := range s.Action {
			if _, ok := wantActions[a]; ok {
				wantActions[a] = true
			}
		}
	}
	for action, found := range wantActions {
		if !found {
			t.Errorf("missing action: %s", action)
		}
	}
}

func TestBuildRDSResizePolicy_DoesNotAllowOtherInstances(t *testing.T) {
	p := BuildRDSResizePolicy(sampleProposal(), "123456789012")
	for _, s := range p.Statement {
		for _, a := range s.Action {
			if a != "rds:ModifyDBInstance" {
				continue
			}
			for _, r := range s.Resource {
				if !strings.Contains(r, "orders-prod") {
					t.Errorf("ModifyDBInstance resource %q does not name orders-prod", r)
				}
				if r == "*" {
					t.Errorf("ModifyDBInstance resource is wildcard — bounded authority broken")
				}
			}
		}
	}
}

func TestSessionPolicy_HashIsStable(t *testing.T) {
	a := BuildRDSResizePolicy(sampleProposal(), "123456789012")
	b := BuildRDSResizePolicy(sampleProposal(), "123456789012")
	ha, err := a.Hash()
	if err != nil {
		t.Fatal(err)
	}
	hb, err := b.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Errorf("hashes differ: %q vs %q", ha, hb)
	}
	if len(ha) != 64 {
		t.Errorf("hash length: got %d want 64", len(ha))
	}
}

func TestSessionPolicy_HashChangesWithResource(t *testing.T) {
	a := BuildRDSResizePolicy(sampleProposal(), "123456789012")
	other := sampleProposal()
	other.DBInstanceIdentifier = "different-db"
	b := BuildRDSResizePolicy(other, "123456789012")
	ha, _ := a.Hash()
	hb, _ := b.Hash()
	if ha == hb {
		t.Errorf("different resources should produce different policy hashes")
	}
}

func TestAccountIDFromRoleARN(t *testing.T) {
	cases := []struct {
		arn     string
		want    string
		wantErr bool
	}{
		{"arn:aws:iam::123456789012:role/casper-execution", "123456789012", false},
		{"arn:aws:iam::000000000000:role/foo", "000000000000", false},
		{"not-an-arn", "", true},
		{"arn:aws:iam:::role/no-account", "", true},
	}
	for _, c := range cases {
		got, err := AccountIDFromRoleARN(c.arn)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err=%v wantErr=%v", c.arn, err, c.wantErr)
		}
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.arn, got, c.want)
		}
	}
}

func TestNewSessionName_FitsAWSLimit(t *testing.T) {
	name := newSessionName("rds-resize", "orders-prod")
	if len(name) > 64 {
		t.Errorf("session name too long: %d chars", len(name))
	}
	if !strings.HasPrefix(name, "casper-rds-resize-orders-prod-") {
		t.Errorf("unexpected prefix: %s", name)
	}

	// Long resource — should still fit in 64 chars.
	long := newSessionName("rds-resize", strings.Repeat("a", 80))
	if len(long) > 64 {
		t.Errorf("long session name overflowed: %d chars", len(long))
	}
}
