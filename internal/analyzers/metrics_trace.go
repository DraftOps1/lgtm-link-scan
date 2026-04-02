package analyzers

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/DraftOps1/lgtm-link-scan/internal/model"
)

const metricTraceWeight = 30

type MetricSeries struct {
	Labels map[string]string
}

type Exemplar struct {
	Labels    map[string]string
	Timestamp time.Time
}

type ExemplarSeries struct {
	SeriesLabels map[string]string
	Exemplars    []Exemplar
}

type MetricSeriesFinder interface {
	MatchSeries(ctx context.Context, start, end time.Time, selector string) ([]MetricSeries, error)
	QueryExemplars(ctx context.Context, start, end time.Time, selector string) ([]ExemplarSeries, error)
}

type MetricTraceParams struct {
	Lookback                    time.Duration
	RequestedService            string
	ServiceLabelKey             string
	MinExemplarCoveragePercent  float64
	RequiredExemplarTraceLabels []string
}

type MetricTraceAnalyzer struct {
	mimir MetricSeriesFinder
	tempo TraceResolver
}

func NewMetricTraceAnalyzer(mimir MetricSeriesFinder, tempo TraceResolver) *MetricTraceAnalyzer {
	return &MetricTraceAnalyzer{
		mimir: mimir,
		tempo: tempo,
	}
}

func (a *MetricTraceAnalyzer) Analyze(ctx context.Context, now time.Time, params MetricTraceParams) (model.MetricTraceCheck, model.CategoryScore, []model.Finding, error) {
	labelKey := params.ServiceLabelKey
	if labelKey == "" {
		labelKey = "service_name"
	}

	traceKeys := params.RequiredExemplarTraceLabels
	if len(traceKeys) == 0 {
		traceKeys = []string{"trace_id", "traceid"}
	}

	selector, selectorScope := buildMetricSelector(labelKey, params.RequestedService)
	start := now.Add(-params.Lookback)

	series, err := a.mimir.MatchSeries(ctx, start, now, selector)
	if err != nil {
		return model.MetricTraceCheck{}, model.CategoryScore{}, nil, fmt.Errorf("match series in mimir: %w", err)
	}

	check := model.MetricTraceCheck{
		RequestedService: params.RequestedService,
		ServiceLabelKey:  labelKey,
		Selector:         selector,
		SeriesMatched:    len(series),
		ObservedMetrics:  uniqueMetricNames(series),
	}

	if len(series) == 0 {
		findings := []model.Finding{{
			Severity:       "HIGH",
			Title:          "No metrics matched the requested service selector",
			Fact:           fmt.Sprintf("Mimir returned 0 series for %s in the last %s.", selectorScope, params.Lookback),
			Impact:         "The scanner cannot tell whether metrics can pivot to traces because no service-scoped metrics were found.",
			Recommendation: "Confirm that service labels are promoted into Prometheus series and that the requested service name matches metric labels.",
		}}
		scoreValues := buildCategoryScore("metric_trace", "Metric -> Trace", 0, metricTraceWeight)
		return check, toModelCategoryScore(scoreValues), findings, nil
	}

	exemplarSeries, err := a.mimir.QueryExemplars(ctx, start, now, selector)
	if err != nil {
		return model.MetricTraceCheck{}, model.CategoryScore{}, nil, fmt.Errorf("query exemplars from mimir: %w", err)
	}

	check.SeriesWithExemplars = len(exemplarSeries)
	check.ExemplarCoveragePercent = 0
	if len(exemplarSeries) > 0 {
		check.ExemplarCoveragePercent = 100
	}

	traceResolutionCache := make(map[string]bool)
	exemplarMetricNames := map[string]struct{}{}
	for _, series := range exemplarSeries {
		if metricName := strings.TrimSpace(series.SeriesLabels["__name__"]); metricName != "" {
			exemplarMetricNames[metricName] = struct{}{}
		}
		for _, exemplar := range series.Exemplars {
			check.ExemplarsFound++
			traceID := firstStringLabel(exemplar.Labels, traceKeys...)
			if traceID == "" {
				continue
			}
			check.TraceIDsChecked++
			resolved, ok := traceResolutionCache[traceID]
			if !ok {
				resolved, err = a.tempo.TraceExists(ctx, traceID)
				if err != nil {
					return model.MetricTraceCheck{}, model.CategoryScore{}, nil, fmt.Errorf("resolve exemplar trace_id %s in tempo: %w", traceID, err)
				}
				traceResolutionCache[traceID] = resolved
			}
			if resolved {
				check.TraceIDsResolved++
			}
		}
	}

	check.ExemplarMetrics = sortedKeys(exemplarMetricNames)
	check.ResolutionPercent = percentage(check.TraceIDsResolved, check.TraceIDsChecked)

	findings := buildMetricTraceFindings(check, params, selectorScope)
	score := buildMetricTraceScore(check)

	return check, score, findings, nil
}

func buildMetricTraceFindings(check model.MetricTraceCheck, params MetricTraceParams, selectorScope string) []model.Finding {
	findings := make([]model.Finding, 0, 3)

	if check.ExemplarCoveragePercent < params.MinExemplarCoveragePercent {
		findings = append(findings, model.Finding{
			Severity:       severityForCoverage(check.ExemplarCoveragePercent),
			Title:          "Metrics are missing trace-linked exemplars",
			Fact:           fmt.Sprintf("%.1f%% exemplar coverage for %s; configured minimum is %.1f%%.", check.ExemplarCoveragePercent, selectorScope, params.MinExemplarCoveragePercent),
			Impact:         "Operators cannot pivot from Prometheus-style metrics to traces from the metric view.",
			Recommendation: "Enable exemplar emission in your metrics SDK or collector pipeline and verify that trace_id is attached to sampled metric points.",
		})
	}

	if check.ExemplarsFound > 0 && check.TraceIDsChecked == 0 {
		findings = append(findings, model.Finding{
			Severity:       "HIGH",
			Title:          "Exemplars exist but they do not include trace IDs",
			Fact:           fmt.Sprintf("%d exemplars were returned, but none carried trace_id labels.", check.ExemplarsFound),
			Impact:         "Metrics appear to emit exemplars, but they still cannot pivot to traces.",
			Recommendation: "Emit trace_id as an exemplar label and verify your collector does not strip exemplar metadata.",
		})
	}

	if check.TraceIDsChecked > 0 && check.ResolutionPercent < 100 {
		findings = append(findings, model.Finding{
			Severity:       severityForCoverage(check.ResolutionPercent),
			Title:          "Some exemplar trace IDs do not resolve in Tempo",
			Fact:           fmt.Sprintf("%.1f%% of exemplar trace IDs resolved in Tempo.", check.ResolutionPercent),
			Impact:         "Metric exemplars are present, but some pivots from metrics to traces will still fail.",
			Recommendation: "Verify Tempo retention, trace export timing, and service label alignment between metrics and traces.",
		})
	}

	return findings
}

func buildMetricTraceScore(check model.MetricTraceCheck) model.CategoryScore {
	coverageRatio := 0.0
	if check.ExemplarsFound > 0 {
		coverageRatio = 1
	}
	resolutionRatio := 0.0
	if check.TraceIDsChecked > 0 {
		resolutionRatio = float64(check.TraceIDsResolved) / float64(check.TraceIDsChecked)
	} else if check.ExemplarsFound > 0 {
		resolutionRatio = 1
	}

	earned := int(math.Round(float64(metricTraceWeight) * coverageRatio * resolutionRatio))
	return toModelCategoryScore(buildCategoryScore("metric_trace", "Metric -> Trace", earned, metricTraceWeight))
}

func buildMetricSelector(labelKey, service string) (string, string) {
	if strings.TrimSpace(service) == "" {
		return fmt.Sprintf(`{%s=~".+"}`, labelKey), fmt.Sprintf("%s=* ", labelKey)
	}
	return fmt.Sprintf(`{%s=%q}`, labelKey, service), fmt.Sprintf("%s=%s", labelKey, service)
}

func uniqueMetricNames(series []MetricSeries) []string {
	names := map[string]struct{}{}
	for _, item := range series {
		if name := strings.TrimSpace(item.Labels["__name__"]); name != "" {
			names[name] = struct{}{}
		}
	}
	return sortedKeys(names)
}

func firstStringLabel(labels map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(labels[key]); value != "" {
			return value
		}
	}
	return ""
}

func toModelCategoryScore(values CategoryScoreValues) model.CategoryScore {
	return model.CategoryScore{
		ID:        values.ID,
		Title:     values.Title,
		Earned:    values.Earned,
		Available: values.Available,
		Percent:   values.Percent,
		Status:    values.Status,
	}
}
