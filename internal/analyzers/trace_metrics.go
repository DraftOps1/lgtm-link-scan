package analyzers

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/DraftOps1/lgtm-link-scan/internal/model"
)

const traceMetricsWeight = 20

type TraceServiceFinder interface {
	SearchServiceNames(ctx context.Context, start, end time.Time, query string, limit int) ([]string, error)
}

type TraceMetricsParams struct {
	Lookback            time.Duration
	RequestedService    string
	ServiceLabelKey     string
	RequiredMetricNames []string
}

type TraceMetricsAnalyzer struct {
	tempo TraceServiceFinder
	mimir MetricSeriesFinder
}

func NewTraceMetricsAnalyzer(tempo TraceServiceFinder, mimir MetricSeriesFinder) *TraceMetricsAnalyzer {
	return &TraceMetricsAnalyzer{
		tempo: tempo,
		mimir: mimir,
	}
}

func (a *TraceMetricsAnalyzer) Analyze(ctx context.Context, now time.Time, params TraceMetricsParams) (model.TraceMetricsCheck, model.CategoryScore, []model.Finding, error) {
	labelKey := params.ServiceLabelKey
	if labelKey == "" {
		labelKey = "service_name"
	}

	start := now.Add(-params.Lookback)
	traceQuery, traceScope := buildTraceMetricsTraceQuery(params.RequestedService)
	traceServices, err := a.tempo.SearchServiceNames(ctx, start, now, traceQuery, 20)
	if err != nil {
		return model.TraceMetricsCheck{}, model.CategoryScore{}, nil, fmt.Errorf("search trace services in tempo: %w", err)
	}

	metricSelector := buildTraceMetricsMetricSelector(labelKey, params.RequestedService)
	series, err := a.mimir.MatchSeries(ctx, start, now, metricSelector)
	if err != nil {
		return model.TraceMetricsCheck{}, model.CategoryScore{}, nil, fmt.Errorf("match trace-derived metrics in mimir: %w", err)
	}

	traceMetricNames := uniqueMetricNames(series)
	metricServices := uniqueLabelValues(series, labelKey)
	requiredMatched, requiredMissing := evaluateRequiredTraceMetrics(traceMetricNames, params.RequiredMetricNames)

	serviceAligned := false
	if params.RequestedService != "" {
		serviceAligned = containsString(traceServices, params.RequestedService) && containsString(metricServices, params.RequestedService)
	} else {
		serviceAligned = anyCommonString(traceServices, metricServices)
	}

	check := model.TraceMetricsCheck{
		RequestedService:       params.RequestedService,
		ServiceLabelKey:        labelKey,
		TraceQuery:             traceQuery,
		MetricSelector:         metricSelector,
		TraceServices:          traceServices,
		MetricServices:         metricServices,
		ObservedMetrics:        traceMetricNames,
		RequiredMetricNames:    append([]string(nil), params.RequiredMetricNames...),
		MatchedRequiredMetrics: requiredMatched,
		MissingRequiredMetrics: requiredMissing,
		ServiceNameAligned:     serviceAligned,
	}

	findings := buildTraceMetricsFindings(check, traceScope)
	score := buildTraceMetricsScore(check)
	return check, score, findings, nil
}

func buildTraceMetricsFindings(check model.TraceMetricsCheck, traceScope string) []model.Finding {
	findings := make([]model.Finding, 0, 3)

	if len(check.TraceServices) == 0 {
		findings = append(findings, model.Finding{
			Severity:       "HIGH",
			Title:          "No traces found for Trace -> Metric analysis",
			Fact:           fmt.Sprintf("Tempo returned no trace services for %s.", traceScope),
			Impact:         "The scanner cannot determine whether trace-derived metrics are flowing because no matching traces were found.",
			Recommendation: "Verify trace ingestion and widen the lookback if the service is mostly idle.",
		})
	}

	if len(check.ObservedMetrics) == 0 {
		findings = append(findings, model.Finding{
			Severity:       "HIGH",
			Title:          "Trace-derived metrics are missing from Mimir",
			Fact:           fmt.Sprintf("Mimir returned 0 trace-derived metrics for selector %s.", check.MetricSelector),
			Impact:         "Operators can pivot from traces to metrics only if span metrics or RED metrics are exported for the same service.",
			Recommendation: "Enable span metrics generation and remote write those metrics into Mimir or Prometheus.",
		})
	}

	if len(check.MissingRequiredMetrics) > 0 {
		findings = append(findings, model.Finding{
			Severity:       "MEDIUM",
			Title:          "Required trace-derived metrics are missing",
			Fact:           fmt.Sprintf("Missing required metrics or tokens: %s.", strings.Join(check.MissingRequiredMetrics, ", ")),
			Impact:         "Some trace-to-metric pivots expected by the scan policy are unavailable.",
			Recommendation: "Export the missing trace-derived metric families or update the rule set to the metric names your stack actually emits.",
		})
	}

	if len(check.TraceServices) > 0 && len(check.MetricServices) > 0 && !check.ServiceNameAligned {
		findings = append(findings, model.Finding{
			Severity:       "HIGH",
			Title:          "Trace and metric service names do not align",
			Fact:           fmt.Sprintf("Tempo trace services=%s; Mimir metric services=%s.", strings.Join(check.TraceServices, ", "), strings.Join(check.MetricServices, ", ")),
			Impact:         "Trace -> Metric pivots will be unreliable because the same service is not represented consistently across traces and trace-derived metrics.",
			Recommendation: "Align service.name resource attributes before span metrics are generated.",
		})
	}

	return findings
}

func buildTraceMetricsScore(check model.TraceMetricsCheck) model.CategoryScore {
	requiredRatio := 1.0
	if len(check.RequiredMetricNames) > 0 {
		requiredRatio = float64(len(check.MatchedRequiredMetrics)) / float64(len(check.RequiredMetricNames))
	}
	if len(check.ObservedMetrics) == 0 {
		requiredRatio = 0
	}

	alignmentRatio := 0.0
	if check.ServiceNameAligned {
		alignmentRatio = 1
	}

	earned := int(math.Round(float64(traceMetricsWeight) * requiredRatio * alignmentRatio))
	return toModelCategoryScore(buildCategoryScore("trace_metrics", "Trace -> Metric", earned, traceMetricsWeight))
}

func buildTraceMetricsTraceQuery(service string) (string, string) {
	if strings.TrimSpace(service) == "" {
		return "{}", "all traces"
	}
	return fmt.Sprintf(`{ resource.service.name = %q }`, service), fmt.Sprintf("service=%s", service)
}

func buildTraceMetricsMetricSelector(labelKey, service string) string {
	if strings.TrimSpace(service) == "" {
		return fmt.Sprintf(`{__name__=~"traces_.*",%s=~".+"}`, labelKey)
	}
	return fmt.Sprintf(`{__name__=~"traces_.*",%s=%q}`, labelKey, service)
}

func evaluateRequiredTraceMetrics(observed []string, required []string) ([]string, []string) {
	if len(required) == 0 {
		return nil, nil
	}

	matched := make([]string, 0, len(required))
	missing := make([]string, 0, len(required))
	for _, requirement := range required {
		if requirementSatisfied(observed, requirement) {
			matched = append(matched, requirement)
		} else {
			missing = append(missing, requirement)
		}
	}
	return matched, missing
}

func requirementSatisfied(observed []string, requirement string) bool {
	for _, name := range observed {
		if name == requirement || strings.Contains(name, requirement) {
			return true
		}
	}
	return false
}

func uniqueLabelValues(series []MetricSeries, labelKey string) []string {
	values := map[string]struct{}{}
	for _, item := range series {
		if value := strings.TrimSpace(item.Labels[labelKey]); value != "" {
			values[value] = struct{}{}
		}
	}
	return sortedKeys(values)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func anyCommonString(left, right []string) bool {
	set := make(map[string]struct{}, len(left))
	for _, item := range left {
		set[item] = struct{}{}
	}
	for _, item := range right {
		if _, ok := set[item]; ok {
			return true
		}
	}
	return false
}
