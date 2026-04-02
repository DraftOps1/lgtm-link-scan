package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndValidate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
endpoints:
  loki: http://localhost:3100
  mimir: http://localhost:9090
  tempo: http://localhost:3200

auth:
  bearer_token: ""

scan:
  lookback: 30m
  services:
    - checkout

rules:
  log_trace:
    required_trace_keys: ["trace_id", "traceid"]
    min_coverage_percent: 80
  metric_trace:
    min_exemplar_coverage_percent: 50
  trace_metrics:
    required_metric_names:
      - calls
  labels:
    required_shared_labels:
      - service.name
  severity:
    critical: 40
    high: 60
    medium: 80
`)

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate config: %v", err)
	}
}
