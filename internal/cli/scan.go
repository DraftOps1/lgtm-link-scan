package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/DraftOps1/lgtm-link-scan/internal/analyzers"
	"github.com/DraftOps1/lgtm-link-scan/internal/clients"
	"github.com/DraftOps1/lgtm-link-scan/internal/config"
	"github.com/DraftOps1/lgtm-link-scan/internal/model"
	"github.com/spf13/cobra"
)

func newScanCmd() *cobra.Command {
	var configPath string
	var service string
	var lookback string
	var output string
	var failUnder int

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Run a link quality scan",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runScan(cmd.OutOrStdout(), configPath, service, lookback, output, failUnder)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "lgtm-link-scan.yaml", "Path to the YAML config file")
	cmd.Flags().StringVar(&service, "service", "", "Only scan a single service")
	cmd.Flags().StringVar(&lookback, "lookback", "", "Override the configured lookback window")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Write JSON results to a file")
	cmd.Flags().IntVar(&failUnder, "fail-under", 0, "Exit non-zero when the overall score is below this threshold")

	return cmd
}

type lokiScanClient interface {
	SampleLogs(ctx context.Context, start, end time.Time, limit int) ([]clients.LokiLogEntry, error)
}

type tempoScanClient interface {
	TraceExists(ctx context.Context, traceID string) (bool, error)
	SearchServiceNames(ctx context.Context, start, end time.Time, query string, limit int) ([]string, error)
	SampleTraceResourceAttributes(ctx context.Context, start, end time.Time, query string, limit int) ([]map[string]string, error)
}

type mimirScanClient interface {
	MatchSeries(ctx context.Context, start, end time.Time, selector string) ([]clients.MetricSeries, error)
	QueryExemplars(ctx context.Context, start, end time.Time, selector string) ([]clients.ExemplarSeries, error)
}

type scanDependencies struct {
	newLokiClient  func(cfg config.Config) (lokiScanClient, error)
	newMimirClient func(cfg config.Config) (mimirScanClient, error)
	newTempoClient func(cfg config.Config) (tempoScanClient, error)
	now            func() time.Time
}

func runScan(out io.Writer, configPath, serviceOverride, lookbackOverride, outputPath string, failUnder int) error {
	return runScanWithDependencies(out, configPath, serviceOverride, lookbackOverride, outputPath, failUnder, scanDependencies{
		newLokiClient: func(cfg config.Config) (lokiScanClient, error) {
			return clients.NewLokiClient(clients.ClientConfig{
				BaseURL:     cfg.Endpoints.Loki,
				BearerToken: cfg.Auth.BearerToken,
			})
		},
		newMimirClient: func(cfg config.Config) (mimirScanClient, error) {
			return clients.NewMimirClient(clients.ClientConfig{
				BaseURL:     cfg.Endpoints.Mimir,
				BearerToken: cfg.Auth.BearerToken,
			})
		},
		newTempoClient: func(cfg config.Config) (tempoScanClient, error) {
			return clients.NewTempoClient(clients.ClientConfig{
				BaseURL:     cfg.Endpoints.Tempo,
				BearerToken: cfg.Auth.BearerToken,
			})
		},
		now: time.Now,
	})
}

func runScanWithDependencies(out io.Writer, configPath, serviceOverride, lookbackOverride, outputPath string, failUnder int, deps scanDependencies) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	requestedService := firstNonEmpty(serviceOverride, firstService(cfg.Scan.Services))
	effectiveLookback := firstNonEmpty(lookbackOverride, cfg.Scan.Lookback)

	lookbackDuration, err := time.ParseDuration(effectiveLookback)
	if err != nil {
		return fmt.Errorf("parse lookback %q: %w", effectiveLookback, err)
	}

	lokiClient, err := deps.newLokiClient(cfg)
	if err != nil {
		return fmt.Errorf("create loki client: %w", err)
	}
	mimirClient, err := deps.newMimirClient(cfg)
	if err != nil {
		return fmt.Errorf("create mimir client: %w", err)
	}
	tempoClient, err := deps.newTempoClient(cfg)
	if err != nil {
		return fmt.Errorf("create tempo client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logAnalyzer := analyzers.NewLogTraceAnalyzer(
		logSamplerAdapter{client: lokiClient},
		traceResolverAdapter{client: tempoClient},
	)

	logTrace, logTraceScore, findings, err := logAnalyzer.Analyze(ctx, deps.now(), analyzers.LogTraceParams{
		Lookback:           lookbackDuration,
		RequestedService:   requestedService,
		RequiredTraceKeys:  cfg.Rules.LogTrace.RequiredTraceKeys,
		RequiredSpanKeys:   []string{"span_id", "spanid"},
		MinCoveragePercent: cfg.Rules.LogTrace.MinCoveragePercent,
		MaxLogSamples:      200,
	})
	if err != nil {
		return err
	}

	metricAnalyzer := analyzers.NewMetricTraceAnalyzer(
		metricSeriesFinderAdapter{client: mimirClient},
		traceResolverAdapter{client: tempoClient},
	)

	metricTrace, metricTraceScore, metricFindings, err := metricAnalyzer.Analyze(ctx, deps.now(), analyzers.MetricTraceParams{
		Lookback:                    lookbackDuration,
		RequestedService:            requestedService,
		ServiceLabelKey:             "service_name",
		MinExemplarCoveragePercent:  cfg.Rules.MetricTrace.MinExemplarCoveragePercent,
		RequiredExemplarTraceLabels: []string{"trace_id", "traceid"},
	})
	if err != nil {
		return err
	}

	traceMetricsAnalyzer := analyzers.NewTraceMetricsAnalyzer(
		traceServiceFinderAdapter{client: tempoClient},
		metricSeriesFinderAdapter{client: mimirClient},
	)

	traceMetrics, traceMetricsScore, traceMetricFindings, err := traceMetricsAnalyzer.Analyze(ctx, deps.now(), analyzers.TraceMetricsParams{
		Lookback:            lookbackDuration,
		RequestedService:    requestedService,
		ServiceLabelKey:     "service_name",
		RequiredMetricNames: cfg.Rules.TraceMetrics.RequiredMetricNames,
	})
	if err != nil {
		return err
	}

	labelsAnalyzer := analyzers.NewLabelsConsistencyAnalyzer(
		logSamplerAdapter{client: lokiClient},
		metricSeriesFinderAdapter{client: mimirClient},
		traceAttributeSamplerAdapter{client: tempoClient},
	)

	labelsConsistency, labelsScore, labelFindings, err := labelsAnalyzer.Analyze(ctx, deps.now(), analyzers.LabelsConsistencyParams{
		Lookback:         lookbackDuration,
		RequestedService: requestedService,
		ServiceLabelKey:  "service_name",
		RequiredLabels:   cfg.Rules.Labels.RequiredSharedLabels,
		MaxLogSamples:    200,
		MaxTraceSamples:  10,
	})
	if err != nil {
		return err
	}

	findings = append(findings, metricFindings...)
	findings = append(findings, traceMetricFindings...)
	findings = append(findings, labelFindings...)
	checks := []model.CategoryScore{logTraceScore, metricTraceScore, traceMetricsScore, labelsScore}
	result := model.ScanResult{
		GeneratedAt:       deps.now().UTC(),
		Partial:           false,
		ImplementedChecks: []string{"log_trace", "metric_trace", "trace_metrics", "labels_consistency"},
		Service:           requestedService,
		Lookback:          effectiveLookback,
		Score:             buildScanScore(checks, cfg),
		Checks:            checks,
		Findings:          findings,
		LogTrace:          &logTrace,
		MetricTrace:       &metricTrace,
		TraceMetrics:      &traceMetrics,
		LabelsConsistency: &labelsConsistency,
	}

	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal scan result: %w", err)
	}

	if outputPath == "" {
		if _, err := fmt.Fprintln(out, string(encoded)); err != nil {
			return err
		}
	} else {
		if err := os.WriteFile(outputPath, append(encoded, '\n'), 0o644); err != nil {
			return fmt.Errorf("write scan result %q: %w", outputPath, err)
		}
		if _, err := fmt.Fprintf(out, "scan: wrote %s\nscore: %d/%d (%d%%)\nfindings: %d\n", outputPath, result.Score.Earned, result.Score.Available, result.Score.Percent, len(result.Findings)); err != nil {
			return err
		}
	}

	if failUnder > 0 && result.Score.Percent < failUnder {
		return fmt.Errorf("scan score %d is below fail-under %d", result.Score.Percent, failUnder)
	}

	return nil
}

type logSamplerAdapter struct {
	client lokiScanClient
}

func (a logSamplerAdapter) SampleLogs(ctx context.Context, start, end time.Time, limit int) ([]analyzers.LogSample, error) {
	entries, err := a.client.SampleLogs(ctx, start, end, limit)
	if err != nil {
		return nil, err
	}

	samples := make([]analyzers.LogSample, 0, len(entries))
	for _, entry := range entries {
		samples = append(samples, analyzers.LogSample{
			Timestamp: entry.Timestamp,
			Line:      entry.Line,
		})
	}
	return samples, nil
}

type traceResolverAdapter struct {
	client tempoScanClient
}

func (a traceResolverAdapter) TraceExists(ctx context.Context, traceID string) (bool, error) {
	return a.client.TraceExists(ctx, traceID)
}

type traceServiceFinderAdapter struct {
	client tempoScanClient
}

func (a traceServiceFinderAdapter) SearchServiceNames(ctx context.Context, start, end time.Time, query string, limit int) ([]string, error) {
	return a.client.SearchServiceNames(ctx, start, end, query, limit)
}

type traceAttributeSamplerAdapter struct {
	client tempoScanClient
}

func (a traceAttributeSamplerAdapter) SampleTraceResourceAttributes(ctx context.Context, start, end time.Time, query string, limit int) ([]map[string]string, error) {
	return a.client.SampleTraceResourceAttributes(ctx, start, end, query, limit)
}

type metricSeriesFinderAdapter struct {
	client mimirScanClient
}

func (a metricSeriesFinderAdapter) MatchSeries(ctx context.Context, start, end time.Time, selector string) ([]analyzers.MetricSeries, error) {
	series, err := a.client.MatchSeries(ctx, start, end, selector)
	if err != nil {
		return nil, err
	}
	result := make([]analyzers.MetricSeries, 0, len(series))
	for _, item := range series {
		result = append(result, analyzers.MetricSeries{Labels: item.Labels})
	}
	return result, nil
}

func (a metricSeriesFinderAdapter) QueryExemplars(ctx context.Context, start, end time.Time, selector string) ([]analyzers.ExemplarSeries, error) {
	series, err := a.client.QueryExemplars(ctx, start, end, selector)
	if err != nil {
		return nil, err
	}
	result := make([]analyzers.ExemplarSeries, 0, len(series))
	for _, item := range series {
		exemplars := make([]analyzers.Exemplar, 0, len(item.Exemplars))
		for _, exemplar := range item.Exemplars {
			exemplars = append(exemplars, analyzers.Exemplar{
				Labels:    exemplar.Labels,
				Timestamp: exemplar.Timestamp,
			})
		}
		result = append(result, analyzers.ExemplarSeries{
			SeriesLabels: item.SeriesLabels,
			Exemplars:    exemplars,
		})
	}
	return result, nil
}

func buildScanScore(checks []model.CategoryScore, cfg config.Config) model.Score {
	earned := 0
	available := 0
	for _, check := range checks {
		earned += check.Earned
		available += check.Available
	}
	percent := 0
	if available > 0 {
		percent = int(float64(earned) * 100 / float64(available))
	}
	return model.Score{
		Earned:    earned,
		Available: available,
		Percent:   percent,
		Severity:  scoreSeverity(percent, cfg),
	}
}

func scoreSeverity(percent int, cfg config.Config) string {
	switch {
	case percent <= cfg.Rules.Severity.Critical:
		return "CRITICAL"
	case percent <= cfg.Rules.Severity.High:
		return "HIGH"
	case percent <= cfg.Rules.Severity.Medium:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

func firstService(services []string) string {
	if len(services) == 0 {
		return ""
	}
	return services[0]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
