package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func (c Config) Validate() error {
	var problems []error

	if strings.TrimSpace(c.Endpoints.Loki) == "" {
		problems = append(problems, errors.New("endpoints.loki is required"))
	}
	if strings.TrimSpace(c.Endpoints.Mimir) == "" {
		problems = append(problems, errors.New("endpoints.mimir is required"))
	}
	if strings.TrimSpace(c.Endpoints.Tempo) == "" {
		problems = append(problems, errors.New("endpoints.tempo is required"))
	}

	if strings.TrimSpace(c.Scan.Lookback) == "" {
		problems = append(problems, errors.New("scan.lookback is required"))
	} else if _, err := time.ParseDuration(c.Scan.Lookback); err != nil {
		problems = append(problems, fmt.Errorf("scan.lookback: %w", err))
	}

	if c.Rules.LogTrace.MinCoveragePercent < 0 || c.Rules.LogTrace.MinCoveragePercent > 100 {
		problems = append(problems, errors.New("rules.log_trace.min_coverage_percent must be between 0 and 100"))
	}
	if c.Rules.MetricTrace.MinExemplarCoveragePercent < 0 || c.Rules.MetricTrace.MinExemplarCoveragePercent > 100 {
		problems = append(problems, errors.New("rules.metric_trace.min_exemplar_coverage_percent must be between 0 and 100"))
	}

	if c.Rules.Severity.Critical == 0 {
		c.Rules.Severity.Critical = 40
	}
	if c.Rules.Severity.High == 0 {
		c.Rules.Severity.High = 60
	}
	if c.Rules.Severity.Medium == 0 {
		c.Rules.Severity.Medium = 80
	}

	return errors.Join(problems...)
}
