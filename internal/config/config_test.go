package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadDefaultsAndEnvOverride(t *testing.T) {
	t.Setenv("LEANMCP_UPSTREAM_MCP_URL", "http://upstream/mcp")
	t.Setenv("LEANMCP_REDIS_URL", "redis://localhost:6379")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.UpstreamMCPURL != "http://upstream/mcp" {
		t.Fatalf("upstream = %q", cfg.UpstreamMCPURL)
	}
	if cfg.OptimizeThresholdTokens != 1500 {
		t.Fatalf("default threshold = %d, want 1500", cfg.OptimizeThresholdTokens)
	}
	if cfg.ExpandTTL != 10*time.Minute {
		t.Fatalf("default ttl = %v, want 10m", cfg.ExpandTTL)
	}
	if !cfg.ShadowMode {
		t.Fatalf("shadow_mode default should be true")
	}
}

func TestLoadMissingUpstreamFails(t *testing.T) {
	os.Clearenv()
	if _, err := Load(""); err == nil {
		t.Fatalf("expected error when upstream URL missing")
	}
}
