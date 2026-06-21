// Package config loads leanmcp configuration from a YAML file (optional) and
// environment variables (which override the file), then validates it.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// ToolRule is the optional per-tool optimization configuration.
type ToolRule struct {
	Compaction string   `yaml:"compaction"`  // "tabular" | "none"
	ArrayPath  string   `yaml:"array_path"`  // dot-path to the dominant array
	MaxItems   int      `yaml:"max_items"`   // 0 = no cap
	KeepFields []string `yaml:"keep_fields"` // opt-in lossy projection; off by default
}

// Config is the full runtime configuration.
type Config struct {
	UpstreamMCPURL          string              `yaml:"upstream_mcp_url"`
	RedisURL                string              `yaml:"redis_url"`
	VerifyEndpoint          string              `yaml:"verify_endpoint"` // empty => static/dev verifier
	OptimizeThresholdTokens int                 `yaml:"optimize_threshold_tokens"`
	ExpandTTL               time.Duration       `yaml:"expand_ttl"`
	MaxRawBytes             int                 `yaml:"max_raw_bytes"`
	ShadowMode              bool                `yaml:"shadow_mode"`
	ListenAddr              string              `yaml:"listen_addr"`
	Tools                   map[string]ToolRule `yaml:"tools"`
}

// Load reads optional YAML at path (if non-empty), applies defaults, overlays
// environment variables, and validates the result.
func Load(path string) (*Config, error) {
	cfg := &Config{
		OptimizeThresholdTokens: 1500,
		ExpandTTL:               10 * time.Minute,
		MaxRawBytes:             5_000_000,
		ShadowMode:              true,
		ListenAddr:              ":8080",
		Tools:                   map[string]ToolRule{},
	}
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config %s: %w", path, err)
		}
		if err := yaml.Unmarshal(b, cfg); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
	}
	overlayEnv(cfg)
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// overlayEnv applies LEANMCP_* environment variables over the config.
func overlayEnv(cfg *Config) {
	if v := os.Getenv("LEANMCP_UPSTREAM_MCP_URL"); v != "" {
		cfg.UpstreamMCPURL = v
	}
	if v := os.Getenv("LEANMCP_REDIS_URL"); v != "" {
		cfg.RedisURL = v
	}
	if v := os.Getenv("LEANMCP_VERIFY_ENDPOINT"); v != "" {
		cfg.VerifyEndpoint = v
	}
	if v := os.Getenv("LEANMCP_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("LEANMCP_SHADOW_MODE"); v == "false" {
		cfg.ShadowMode = false
	}
}

// validate enforces required fields.
func (c *Config) validate() error {
	if c.UpstreamMCPURL == "" {
		return fmt.Errorf("upstream_mcp_url (LEANMCP_UPSTREAM_MCP_URL) is required")
	}
	if c.OptimizeThresholdTokens < 0 {
		return fmt.Errorf("optimize_threshold_tokens must be >= 0")
	}
	return nil
}
