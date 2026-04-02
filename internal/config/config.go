package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Endpoints Endpoints `yaml:"endpoints"`
	Auth      Auth      `yaml:"auth"`
	Scan      Scan      `yaml:"scan"`
	Rules     Rules     `yaml:"rules"`
}

type Endpoints struct {
	Loki  string `yaml:"loki"`
	Mimir string `yaml:"mimir"`
	Tempo string `yaml:"tempo"`
}

type Auth struct {
	BearerToken string `yaml:"bearer_token"`
}

type Scan struct {
	Lookback string   `yaml:"lookback"`
	Services []string `yaml:"services"`
}

type Rules struct {
	LogTrace     LogTraceRule     `yaml:"log_trace"`
	MetricTrace  MetricTraceRule  `yaml:"metric_trace"`
	TraceMetrics TraceMetricsRule `yaml:"trace_metrics"`
	Labels       LabelsRule       `yaml:"labels"`
	Severity     SeverityRule     `yaml:"severity"`
}

type LogTraceRule struct {
	RequiredTraceKeys  []string `yaml:"required_trace_keys"`
	MinCoveragePercent float64  `yaml:"min_coverage_percent"`
}

type MetricTraceRule struct {
	MinExemplarCoveragePercent float64 `yaml:"min_exemplar_coverage_percent"`
}

type TraceMetricsRule struct {
	RequiredMetricNames []string `yaml:"required_metric_names"`
}

type LabelsRule struct {
	RequiredSharedLabels []string `yaml:"required_shared_labels"`
}

type SeverityRule struct {
	Critical int `yaml:"critical"`
	High     int `yaml:"high"`
	Medium   int `yaml:"medium"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}

	return cfg, nil
}
