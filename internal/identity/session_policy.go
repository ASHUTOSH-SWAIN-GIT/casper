package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// SessionPolicy is a v2 IAM policy document. AWS limits inline session
// policies to ~2KB and intersects them with the assumed role's policy
// (the result is the *narrower* of the two), so this acts as a hard
// resource cap regardless of how broad the underlying role is.
type SessionPolicy struct {
	Version   string      `json:"Version"`
	Statement []Statement `json:"Statement"`
}

type Statement struct {
	Sid      string   `json:"Sid,omitempty"`
	Effect   string   `json:"Effect"`
	Action   []string `json:"Action"`
	Resource []string `json:"Resource"`
}

// BuildRDSResizePolicy returns the minimal session policy for a single
// RDS resize. Concretely:
//
//   - rds:ModifyDBInstance scoped to *this* instance ARN — the bounded
//     authority bound. Even with the same credentials, you cannot
//     modify a different instance.
//   - rds:DescribeDBInstances on "*" — the AWS API does not support
//     resource-level scoping for describes (the Action accepts the
//     argument and the response is naturally filtered to what the
//     caller asked about). This is the smallest grant AWS allows.
//   - cloudwatch:GetMetricStatistics on "*" — same story; no resource
//     ARN support for this action class.
//
// accountID and the proposal together produce a fully-qualified ARN
// like arn:aws:rds:us-east-1:123456789012:db:orders-prod.
func BuildRDSResizePolicy(p action.RDSResizeProposal, accountID string) SessionPolicy {
	dbARN := fmt.Sprintf("arn:aws:rds:%s:%s:db:%s",
		p.Region, accountID, p.DBInstanceIdentifier)

	return SessionPolicy{
		Version: "2012-10-17",
		Statement: []Statement{
			{
				Sid:      "ModifyThisInstanceOnly",
				Effect:   "Allow",
				Action:   []string{"rds:ModifyDBInstance"},
				Resource: []string{dbARN},
			},
			{
				Sid:      "DescribeAnyInstance",
				Effect:   "Allow",
				Action:   []string{"rds:DescribeDBInstances"},
				Resource: []string{"*"},
			},
			{
				Sid:      "ReadCloudWatchMetrics",
				Effect:   "Allow",
				Action:   []string{"cloudwatch:GetMetricStatistics"},
				Resource: []string{"*"},
			},
		},
	}
}

// Marshal returns the JSON form of the policy, suitable to pass to
// sts:AssumeRole's Policy parameter.
func (p SessionPolicy) Marshal() ([]byte, error) {
	return json.Marshal(p)
}

// Hash returns a stable hex sha256 over the canonical JSON form.
// Stored in the audit event so anyone can verify "the credentials that
// ran this step had exactly this scope."
func (p SessionPolicy) Hash() (string, error) {
	b, err := p.Marshal()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// AccountIDFromRoleARN extracts the AWS account ID from a role ARN,
// e.g. arn:aws:iam::123456789012:role/casper-execution → "123456789012".
// Used to construct resource ARNs without requiring an extra
// sts:GetCallerIdentity round-trip per session.
func AccountIDFromRoleARN(roleARN string) (string, error) {
	parts := strings.Split(roleARN, ":")
	if len(parts) < 6 || parts[0] != "arn" {
		return "", fmt.Errorf("not an ARN: %q", roleARN)
	}
	if parts[4] == "" {
		return "", fmt.Errorf("ARN missing account id: %q", roleARN)
	}
	return parts[4], nil
}
