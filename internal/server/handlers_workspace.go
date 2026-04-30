package server

import (
	"net/http"
	"os"
)

// handleGetWorkspace surfaces the live workspace context the dashboard
// needs to render its topbar pill: AWS region/account hints, default
// LLM backend, whether an identity broker role is configured.
//
// All values are read from the environment. The endpoint is read-only;
// changing them today means restarting casperd with new env vars.
func (s *Server) handleGetWorkspace(w http.ResponseWriter, _ *http.Request) {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	backend := os.Getenv("CASPER_LLM_BACKEND")
	if backend == "" {
		backend = "anthropic"
	}

	llmAvailable := true
	llmReason := ""
	if _, err := s.deps.LLMConfig(); err != nil {
		llmAvailable = false
		llmReason = err.Error()
	}

	roleARN := os.Getenv("CASPER_ROLE_ARN")
	identity := "default_credentials"
	if roleARN != "" {
		identity = "scoped_sts"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"region":        region,
		"backend":       backend,
		"llm_available": llmAvailable,
		"llm_reason":    llmReason,
		"identity_mode": identity,
	})
}
