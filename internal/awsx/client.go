// Package awsx implements interpreter.Client over aws-sdk-go-v2.
//
// This package is the only one in Casper that imports the AWS SDK.
// Every AWS call the interpreter makes goes through Client.Call here.
// SDK retries are disabled — failures are step failures, recorded as
// such, never silently retried.
package awsx

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsmw "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/smithy-go/middleware"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/interpreter"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/plan"
)

// Client dispatches plan.APICall to typed aws-sdk-go-v2 clients.
type Client struct {
	rds        *rds.Client
	cloudwatch *cloudwatch.Client
	now        func() time.Time
}

// New builds a Client from an aws.Config. Retries are forced to 1 so
// the interpreter sees every failure verbatim instead of the SDK
// retrying behind its back.
func New(cfg aws.Config) *Client {
	cfg.RetryMaxAttempts = 1
	return &Client{
		rds:        rds.NewFromConfig(cfg),
		cloudwatch: cloudwatch.NewFromConfig(cfg),
		now:        time.Now,
	}
}

// Call dispatches by service then operation. Unknown service/operation
// is a step failure; we never silently no-op on something unexpected.
func (c *Client) Call(ctx context.Context, call plan.APICall) (interpreter.Response, error) {
	switch call.Service {
	case "rds":
		return c.callRDS(ctx, call)
	case "cloudwatch":
		return c.callCloudWatch(ctx, call)
	default:
		return interpreter.Response{}, fmt.Errorf("unsupported service: %q", call.Service)
	}
}

func (c *Client) callRDS(ctx context.Context, call plan.APICall) (interpreter.Response, error) {
	switch call.Operation {
	case "DescribeDBInstances":
		var in rds.DescribeDBInstancesInput
		if err := remarshal(call.Params, &in); err != nil {
			return interpreter.Response{}, fmt.Errorf("decode params: %w", err)
		}
		out, err := c.rds.DescribeDBInstances(ctx, &in)
		if err != nil {
			return interpreter.Response{}, err
		}
		return wrap(out, out.ResultMetadata)
	case "ModifyDBInstance":
		var in rds.ModifyDBInstanceInput
		if err := remarshal(call.Params, &in); err != nil {
			return interpreter.Response{}, fmt.Errorf("decode params: %w", err)
		}
		out, err := c.rds.ModifyDBInstance(ctx, &in)
		if err != nil {
			return interpreter.Response{}, err
		}
		return wrap(out, out.ResultMetadata)

	default:
		return interpreter.Response{}, fmt.Errorf("unsupported rds operation: %q", call.Operation)
	}
}

func (c *Client) callCloudWatch(ctx context.Context, call plan.APICall) (interpreter.Response, error) {
	switch call.Operation {
	case "GetMetricStatistics":
		params, err := expandWindow(call.Params, c.now)
		if err != nil {
			return interpreter.Response{}, err
		}
		var in cloudwatch.GetMetricStatisticsInput
		if err := remarshal(params, &in); err != nil {
			return interpreter.Response{}, fmt.Errorf("decode params: %w", err)
		}
		out, err := c.cloudwatch.GetMetricStatistics(ctx, &in)
		if err != nil {
			return interpreter.Response{}, err
		}
		body, err := toBody(out)
		if err != nil {
			return interpreter.Response{}, err
		}
		// CloudWatch returns Datapoints as a list. The plan asserts on
		// "Datapoints.avg", so reduce the list to {avg, raw} so the
		// predicate evaluator has a single number to compare.
		if dps, ok := body["Datapoints"].([]any); ok {
			body["Datapoints"] = map[string]any{
				"avg": averageDatapoints(dps),
				"raw": dps,
			}
		}
		reqID, _ := awsmw.GetRequestIDMetadata(out.ResultMetadata)
		return interpreter.Response{Body: body, RequestID: reqID}, nil

	default:
		return interpreter.Response{}, fmt.Errorf("unsupported cloudwatch operation: %q", call.Operation)
	}
}

// expandWindow converts a relative "Window" param ("5m") into concrete
// StartTime/EndTime stamps in RFC3339, so the rest of the call uses
// only standard CloudWatch input fields. Action specs talk about
// windows; the SDK talks about timestamps.
func expandWindow(in map[string]any, now func() time.Time) (map[string]any, error) {
	winRaw, ok := in["Window"]
	if !ok {
		return in, nil
	}
	winStr, ok := winRaw.(string)
	if !ok {
		return nil, fmt.Errorf("Window must be a string, got %T", winRaw)
	}
	d, err := time.ParseDuration(winStr)
	if err != nil {
		return nil, fmt.Errorf("invalid Window %q: %w", winStr, err)
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if k == "Window" {
			continue
		}
		out[k] = v
	}
	t := now().UTC()
	out["StartTime"] = t.Add(-d).Format(time.RFC3339)
	out["EndTime"] = t.Format(time.RFC3339)
	return out, nil
}

func averageDatapoints(dps []any) float64 {
	var sum float64
	var n int
	for _, dp := range dps {
		m, ok := dp.(map[string]any)
		if !ok {
			continue
		}
		v, ok := m["Average"].(float64)
		if !ok {
			continue
		}
		sum += v
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// wrap serializes a typed AWS output into a generic body and pulls the
// AWS request ID off the response metadata for the audit log.
func wrap(out any, md middleware.Metadata) (interpreter.Response, error) {
	body, err := toBody(out)
	if err != nil {
		return interpreter.Response{}, err
	}
	reqID, _ := awsmw.GetRequestIDMetadata(md)
	return interpreter.Response{Body: body, RequestID: reqID}, nil
}

func remarshal(in any, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func toBody(out any) (map[string]any, error) {
	b, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}
