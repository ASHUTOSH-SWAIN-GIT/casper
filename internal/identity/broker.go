// Package identity is Casper's bounded-authority component.
//
// The broker turns a proposal into per-action AWS credentials whose
// IAM permissions are narrowed to exactly the resource the proposal
// names. It is the last line of defense in the trust layer: even if
// schema validation, the simulator, and the policy engine all
// misbehaved, AWS itself would reject a call outside the session
// policy's scope.
//
// The broker is intentionally small. It does one thing: AssumeRole
// with a session policy and a short TTL, and surface enough metadata
// for the audit log to record exactly what was minted.
package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// SessionTTL is the lifetime of a minted credential. 15 minutes is
// the minimum AWS allows for AssumeRole and the maximum we want for
// a one-action session. A resize that needs longer (rare) should be
// re-evaluated, not extended.
const SessionTTL = 15 * time.Minute

// Broker mints scoped, short-lived AWS credentials for one action.
//
// Construction binds the broker to a specific role ARN and external
// ID; calls to MintForRDSResize then produce a fresh aws.Config whose
// credentials are sts:AssumeRole'd from that role with a per-action
// session policy.
type Broker struct {
	stsClient  *sts.Client
	roleARN    string
	externalID string
	accountID  string
}

// Config is what callers pass to New. roleARN is the cross-account (or
// same-account) role Casper is permitted to assume; externalID is the
// shared secret the role's trust policy expects, defending against
// confused-deputy attacks.
type Config struct {
	RoleARN    string
	ExternalID string
}

// New constructs a Broker. The accountID is parsed out of the role
// ARN so we don't need an extra GetCallerIdentity round-trip later.
func New(awsCfg aws.Config, c Config) (*Broker, error) {
	if c.RoleARN == "" {
		return nil, fmt.Errorf("RoleARN is required")
	}
	if c.ExternalID == "" {
		return nil, fmt.Errorf("ExternalID is required")
	}
	accountID, err := AccountIDFromRoleARN(c.RoleARN)
	if err != nil {
		return nil, fmt.Errorf("parse role arn: %w", err)
	}
	return &Broker{
		stsClient:  sts.NewFromConfig(awsCfg),
		roleARN:    c.RoleARN,
		externalID: c.ExternalID,
		accountID:  accountID,
	}, nil
}

// Session is the result of a successful Mint call. Cfg is what the
// AWS client (awsx) consumes; the rest is metadata recorded in the
// audit log so anyone reading the chain can answer "what scope ran
// this?"
type Session struct {
	Cfg          aws.Config
	SessionName  string
	PolicyHash   string
	Policy       SessionPolicy
	Expires      time.Time
}

// MintForRDSResize assumes the configured role with a session policy
// scoped to exactly this proposal's resource. The returned aws.Config
// can be handed straight to awsx.New; everything else in Session is
// audit-shaped metadata.
func (b *Broker) MintForRDSResize(ctx context.Context, p action.RDSResizeProposal) (Session, error) {
	policy := BuildRDSResizePolicy(p, b.accountID)
	policyJSON, err := policy.Marshal()
	if err != nil {
		return Session{}, fmt.Errorf("marshal session policy: %w", err)
	}
	policyHash, err := policy.Hash()
	if err != nil {
		return Session{}, fmt.Errorf("hash session policy: %w", err)
	}

	sessionName := newSessionName("rds-resize", p.DBInstanceIdentifier)
	policyStr := string(policyJSON)
	durationSeconds := int32(SessionTTL.Seconds())

	out, err := b.stsClient.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(b.roleARN),
		RoleSessionName: aws.String(sessionName),
		DurationSeconds: aws.Int32(durationSeconds),
		ExternalId:      aws.String(b.externalID),
		Policy:          aws.String(policyStr),
	})
	if err != nil {
		return Session{}, fmt.Errorf("assume role: %w", err)
	}
	if out.Credentials == nil {
		return Session{}, fmt.Errorf("assume role returned nil credentials")
	}

	creds := credentials.NewStaticCredentialsProvider(
		aws.ToString(out.Credentials.AccessKeyId),
		aws.ToString(out.Credentials.SecretAccessKey),
		aws.ToString(out.Credentials.SessionToken),
	)
	cfg := aws.Config{
		Region:      p.Region,
		Credentials: aws.NewCredentialsCache(creds),
	}

	exp := time.Time{}
	if out.Credentials.Expiration != nil {
		exp = *out.Credentials.Expiration
	}

	return Session{
		Cfg:         cfg,
		SessionName: sessionName,
		PolicyHash:  policyHash,
		Policy:      policy,
		Expires:     exp,
	}, nil
}

// newSessionName produces a unique-per-call RoleSessionName. The name
// shows up in CloudTrail and is the cross-reference for "which Casper
// invocation produced this AWS call?"
func newSessionName(action, resource string) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	suffix := hex.EncodeToString(b[:])
	name := fmt.Sprintf("casper-%s-%s-%s", action, resource, suffix)
	if len(name) > 64 {
		// AWS caps session names at 64 chars; truncate the resource portion.
		name = name[:64]
	}
	return name
}
