package interpreter

import (
	"context"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/plan"
)

// Response is the verbatim AWS response captured for the audit log.
// Body is the parsed payload; RequestID is the AWS request ID used
// to cross-reference CloudTrail.
type Response struct {
	Body      map[string]any
	RequestID string
}

// Client is the only abstraction over AWS the interpreter knows about.
// The single-implementation rule (only one type touches the SDK) is
// enforced by the package layout: the interpreter imports this package
// but not aws-sdk-go-v2 directly.
type Client interface {
	Call(ctx context.Context, call plan.APICall) (Response, error)
}
