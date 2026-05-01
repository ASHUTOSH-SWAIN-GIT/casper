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

func (s *Server) handleGetWorkspace(w http.ResponseWriter, _ *http.Request) {
	file, _ := workspace.EnsureExternalID()

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
	authMethod := ""
	awsConnected := false
	awsAccount := ""
	awsARN := ""
	awsReason := ""

	switch {
	case roleARN != "":
		authMethod = "role"
		out, err := stsAssumeRole(context.Background(), file, roleARN)
		if err != nil {
			awsReason = err.Error()
		} else {
			awsConnected = true
			awsARN = aws.ToString(out.AssumedRoleUser.Arn)
			awsAccount, _ = callerAccount(context.Background(), file)
		}

	case file.AWSAccessKeyID != "":
		authMethod = "keys"
		out, err := stsGetCallerIdentity(context.Background(), file)
		if err != nil {
			awsReason = err.Error()
		} else {
			awsConnected = true
			awsAccount = aws.ToString(out.Account)
			awsARN = aws.ToString(out.Arn)
		}

	default:
		awsReason = "no credentials configured"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"region":        region,
		"backend":       backend,
		"llm_available": llmAvailable,
		"llm_reason":    llmReason,
		"role_arn":      roleARN,
		"external_id":   file.ExternalID,
		"auth_method":   authMethod,
		"aws_connected": awsConnected,
		"aws_account":   awsAccount,
		"aws_arn":       awsARN,
		"aws_reason":    awsReason,
	})
}

func (s *Server) handleUpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		// Role-based
		RoleARN string `json:"role_arn"`
		// Key-based
		AWSAccessKeyID     string `json:"aws_access_key_id"`
		AWSSecretAccessKey string `json:"aws_secret_access_key"`
		// Shared
		AWSRegion       string `json:"aws_region"`
		LLMBackend      string `json:"llm_backend"`
		AnthropicAPIKey string `json:"anthropic_api_key"`
		BedrockRegion   string `json:"bedrock_region"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	cfg, _ := workspace.EnsureExternalID()

	// Validate and persist whichever auth method was sent.
	switch {
	case req.RoleARN != "":
		// Merge key into a temp config so AssumeRole can use them if present.
		probe := cfg
		out, err := stsAssumeRole(r.Context(), probe, req.RoleARN)
		if err != nil {
			writeError(w, http.StatusBadRequest, "assume_role_failed", err.Error())
			return
		}
		// Clear any stored static keys when switching to role-based auth.
		cfg.CasperRoleARN = req.RoleARN
		cfg.AWSAccessKeyID = ""
		cfg.AWSSecretAccessKey = ""
		if err := workspace.Save(cfg); err != nil {
			writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":          true,
			"auth_method": "role",
			"assumed_arn": aws.ToString(out.AssumedRoleUser.Arn),
		})
		return

	case req.AWSAccessKeyID != "" && req.AWSSecretAccessKey != "":
		probe := cfg
		probe.AWSAccessKeyID = req.AWSAccessKeyID
		probe.AWSSecretAccessKey = req.AWSSecretAccessKey
		out, err := stsGetCallerIdentity(r.Context(), probe)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_keys", err.Error())
			return
		}
		// Clear any stored role ARN when switching to key-based auth.
		cfg.AWSAccessKeyID = req.AWSAccessKeyID
		cfg.AWSSecretAccessKey = req.AWSSecretAccessKey
		cfg.CasperRoleARN = ""
		if req.AWSRegion != "" {
			cfg.AWSRegion = req.AWSRegion
		}
		if err := workspace.Save(cfg); err != nil {
			writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":          true,
			"auth_method": "keys",
			"aws_account": aws.ToString(out.Account),
			"aws_arn":     aws.ToString(out.Arn),
		})
		return
	}

	// Partial update — shared fields only.
	if req.AWSRegion != "" {
		cfg.AWSRegion = req.AWSRegion
	}
	if req.LLMBackend != "" {
		cfg.LLMBackend = req.LLMBackend
	}
	if req.AnthropicAPIKey != "" {
		cfg.AnthropicAPIKey = req.AnthropicAPIKey
	}
	if req.BedrockRegion != "" {
		cfg.BedrockRegion = req.BedrockRegion
	}
	if err := workspace.Save(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// stsAssumeRole calls AssumeRole using ambient creds (or stored static keys if present).
func stsAssumeRole(ctx context.Context, cfg workspace.Config, roleARN string) (*sts.AssumeRoleOutput, error) {
	awsCfg, err := awsConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return sts.NewFromConfig(awsCfg).AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleARN),
		RoleSessionName: aws.String("casper-verify"),
		ExternalId:      aws.String(cfg.ExternalID),
		DurationSeconds: aws.Int32(900),
	})
}

// stsGetCallerIdentity verifies static keys.
func stsGetCallerIdentity(ctx context.Context, cfg workspace.Config) (*sts.GetCallerIdentityOutput, error) {
	awsCfg, err := awsConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return sts.NewFromConfig(awsCfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
}

// callerAccount returns the account ID from the ambient credential chain.
func callerAccount(ctx context.Context, cfg workspace.Config) (string, error) {
	out, err := stsGetCallerIdentity(ctx, cfg)
	if err != nil {
		return "", err
	}
	return aws.ToString(out.Account), nil
}

// awsConfig builds an AWS config. If cfg has static keys they are used;
// otherwise the ambient credential chain is used.
func awsConfig(ctx context.Context, cfg workspace.Config) (aws.Config, error) {
	region := envOr("AWS_REGION", envOr("AWS_DEFAULT_REGION", cfg.AWSRegion))
	if region == "" {
		region = "us-east-1"
	}
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}
	if cfg.AWSAccessKeyID != "" && cfg.AWSSecretAccessKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, ""),
		))
	}
	return config.LoadDefaultConfig(ctx, opts...)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
