// Package llmcfg derives proposer/router LLM configuration from the
// environment. Shared by casperctl and casperd so a single source of
// truth governs which backend is used and which models override the
// defaults.
package llmcfg

import (
	"fmt"
	"os"
	"strings"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/proposer"
)

// Config bundles the env-derived backend, credentials, region, and
// per-role model overrides used to build proposer.Config and
// proposer.RouterConfig.
type Config struct {
	Backend       proposer.Backend
	APIKey        string
	Region        string
	ProposerModel string // optional; "" means "use the proposer's default"
	RouterModel   string // optional; "" means "use the router's default"
}

// FromEnv reads CASPER_LLM_BACKEND and the appropriate credentials/model
// overrides from the environment. Defaults to the Anthropic-API path;
// switching to Bedrock requires CASPER_LLM_BACKEND=bedrock plus AWS
// credentials in the standard SDK chain.
func FromEnv() (Config, error) {
	backendStr := strings.ToLower(strings.TrimSpace(os.Getenv("CASPER_LLM_BACKEND")))
	switch backendStr {
	case "", "anthropic":
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return Config{}, fmt.Errorf("ANTHROPIC_API_KEY is required (or set CASPER_LLM_BACKEND=bedrock)")
		}
		return Config{
			Backend:       proposer.BackendAnthropic,
			APIKey:        key,
			ProposerModel: os.Getenv("CASPER_PROPOSER_MODEL"),
			RouterModel:   os.Getenv("CASPER_ROUTER_MODEL"),
		}, nil
	case "bedrock":
		region := os.Getenv("AWS_REGION")
		if region == "" {
			region = os.Getenv("AWS_DEFAULT_REGION")
		}
		propModel := os.Getenv("CASPER_BEDROCK_PROPOSER_MODEL")
		routerModel := os.Getenv("CASPER_BEDROCK_ROUTER_MODEL")
		if propModel == "" || routerModel == "" {
			return Config{}, fmt.Errorf(
				"CASPER_BEDROCK_PROPOSER_MODEL and CASPER_BEDROCK_ROUTER_MODEL are required when CASPER_LLM_BACKEND=bedrock\n" +
					"  (Bedrock IDs are version-pinned and account-specific — set them to the inference profile IDs you have access to,\n" +
					"   e.g. \"us.anthropic.claude-sonnet-4-5-20250929-v1:0\" / \"us.anthropic.claude-haiku-4-5-20251001-v1:0\")")
		}
		return Config{
			Backend:       proposer.BackendBedrock,
			Region:        region,
			ProposerModel: propModel,
			RouterModel:   routerModel,
		}, nil
	default:
		return Config{}, fmt.Errorf("unknown CASPER_LLM_BACKEND=%q (expected \"anthropic\" or \"bedrock\")", backendStr)
	}
}
