package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/workspace"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// handleGetWorkspace surfaces the live workspace context. Values come from
// ~/.casper/config.json first, environment variables second (env wins).
func (s *Server) handleGetWorkspace(w http.ResponseWriter, _ *http.Request) {
	file, _ := workspace.Load()

	region := envOr("AWS_REGION", envOr("AWS_DEFAULT_REGION", file.AWSRegion))
	backend := envOr("CASPER_LLM_BACKEND", file.LLMBackend)
	if backend == "" {
		backend = "anthropic"
	}

	llmAvailable := true
	llmReason := ""
	if _, err := s.deps.LLMConfig(); err != nil {
		llmAvailable = false
		llmReason = err.Error()
	}

	roleARN := envOr("CASPER_ROLE_ARN", file.CasperRoleARN)
	identity := "default_credentials"
	if roleARN != "" {
		identity = "scoped_sts"
	}

	awsConnected := false
	awsAccount := ""
	awsReason := ""
	if callerID, err := stsGetCallerIdentity(context.Background(), file); err != nil {
		awsReason = err.Error()
	} else {
		awsConnected = true
		awsAccount = aws.ToString(callerID.Account)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"region":        region,
		"backend":       backend,
		"llm_available": llmAvailable,
		"llm_reason":    llmReason,
		"identity_mode": identity,
		"aws_connected": awsConnected,
		"aws_account":   awsAccount,
		"aws_reason":    awsReason,
	})
}

// handleUpdateWorkspace accepts a JSON body with credential fields and writes
// them to ~/.casper/config.json after validating via sts:GetCallerIdentity.
func (s *Server) handleUpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req workspace.Config
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	callerID, err := stsGetCallerIdentity(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "aws_auth_failed", err.Error())
		return
	}

	// Merge with existing config so partial updates don't wipe other fields.
	existing, _ := workspace.Load()
	if req.AWSAccessKeyID != "" {
		existing.AWSAccessKeyID = req.AWSAccessKeyID
	}
	if req.AWSSecretAccessKey != "" {
		existing.AWSSecretAccessKey = req.AWSSecretAccessKey
	}
	if req.AWSRegion != "" {
		existing.AWSRegion = req.AWSRegion
	}
	if req.CasperRoleARN != "" {
		existing.CasperRoleARN = req.CasperRoleARN
	}
	if req.LLMBackend != "" {
		existing.LLMBackend = req.LLMBackend
	}
	if req.AnthropicAPIKey != "" {
		existing.AnthropicAPIKey = req.AnthropicAPIKey
	}
	if req.BedrockRegion != "" {
		existing.BedrockRegion = req.BedrockRegion
	}

	if err := workspace.Save(existing); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"aws_account": aws.ToString(callerID.Account),
		"aws_user_id": aws.ToString(callerID.UserId),
		"aws_arn":     aws.ToString(callerID.Arn),
	})
}

// stsGetCallerIdentity builds an AWS config from cfg (falling back to the
// ambient credential chain when cfg has no explicit keys) and calls
// sts:GetCallerIdentity. Used for both validation on PUT and status on GET.
func stsGetCallerIdentity(ctx context.Context, cfg workspace.Config) (*sts.GetCallerIdentityOutput, error) {
	region := envOr("AWS_REGION", envOr("AWS_DEFAULT_REGION", cfg.AWSRegion))
	if region == "" {
		region = "us-east-1"
	}

	var awsOpts []func(*config.LoadOptions) error
	awsOpts = append(awsOpts, config.WithRegion(region))

	if cfg.AWSAccessKeyID != "" && cfg.AWSSecretAccessKey != "" {
		awsOpts = append(awsOpts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, ""),
		))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, awsOpts...)
	if err != nil {
		return nil, err
	}

	return sts.NewFromConfig(awsCfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
