// Package workspace manages the persisted workspace config at ~/.casper/config.json.
// Environment variables always take precedence over the file so that CI/CD
// and docker deployments don't need the file at all.
package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config is the shape persisted to ~/.casper/config.json. Zero values mean
// "not set" — the server merges env vars on top when building the live view.
type Config struct {
	// ExternalID is generated once and embedded in the IAM role trust policy
	// so only this Casper installation can assume the role.
	ExternalID string `json:"external_id,omitempty"`

	// Role-based auth (recommended).
	CasperRoleARN string `json:"casper_role_arn,omitempty"`

	// Static key auth (fallback). File is mode 0600 — owner-only.
	AWSAccessKeyID     string `json:"aws_access_key_id,omitempty"`
	AWSSecretAccessKey string `json:"aws_secret_access_key,omitempty"`

	AWSRegion       string `json:"aws_region,omitempty"`
	LLMBackend      string `json:"llm_backend,omitempty"`
	AnthropicAPIKey string `json:"anthropic_api_key,omitempty"`
	BedrockRegion   string `json:"bedrock_region,omitempty"`
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".casper", "config.json"), nil
}

// Load reads the config file. Returns a zero Config (not an error) when the
// file doesn't exist yet — first run scenario.
func Load() (Config, error) {
	path, err := configPath()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	return c, json.Unmarshal(data, &c)
}

// EnsureExternalID loads the config, generates an ExternalID if one doesn't
// exist yet, saves it, and returns the (possibly updated) config.
func EnsureExternalID() (Config, error) {
	cfg, err := Load()
	if err != nil {
		return cfg, err
	}
	if cfg.ExternalID != "" {
		return cfg, nil
	}
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return cfg, fmt.Errorf("generate external id: %w", err)
	}
	h := hex.EncodeToString(b)
	cfg.ExternalID = fmt.Sprintf("casper-%s-%s-%s", h[0:8], h[8:16], h[16:24])
	return cfg, Save(cfg)
}

// Save writes cfg atomically to ~/.casper/config.json, creating the directory
// if needed. The file is written mode 0600 (owner-only).
func Save(cfg Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
