package analyzers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/DraftOps1/lgtm-link-scan/internal/model"
)

const labelsConsistencyWeight = 20

type TraceAttributeSampler interface {
	SampleTraceResourceAttributes(ctx context.Context, start, end time.Time, query string, limit int) ([]map[string]string, error)
}

type LabelsConsistencyParams struct {
	Lookback         time.Duration
	RequestedService string
	ServiceLabelKey  string
	RequiredLabels   []string
	MaxLogSamples    int
	MaxTraceSamples  int
}

type LabelsConsistencyAnalyzer struct {
	loki  LogSampler
	mimir MetricSeriesFinder
	tempo TraceAttributeSampler
}

func NewLabelsConsistencyAnalyzer(loki LogSampler, mimir MetricSeriesFinder, tempo TraceAttributeSampler) *LabelsConsistencyAnalyzer {
	return &LabelsConsistencyAnalyzer{
		loki:  loki,
		mimir: mimir,
		tempo: tempo,
	}
}

func (a *LabelsConsistencyAnalyzer) Analyze(ctx context.Context, now time.Time, params LabelsConsistencyParams) (model.LabelsConsistencyCheck, model.CategoryScore, []model.Finding, error) {
	labelKey := params.ServiceLabelKey
	if labelKey == "" {
		labelKey = "service_name"
	}

	requiredLabels := append([]string(nil), params.RequiredLabels...)
	if len(requiredLabels) == 0 {
		requiredLabels = []string{"service.name", "service.namespace", "deployment.environment"}
	}

	maxLogSamples := params.MaxLogSamples
	if maxLogSamples <= 0 {
		maxLogSamples = 200
	}
	maxTraceSamples := params.MaxTraceSamples
	if maxTraceSamples <= 0 {
		maxTraceSamples = 10
	}

	start := now.Add(-params.Lookback)

	logEntries, observedLogServices, logsRequestedFound, err := a.collectLogLabels(ctx, start, now, params.RequestedService, maxLogSamples)
	if err != nil {
		return model.LabelsConsistencyCheck{}, model.CategoryScore{}, nil, fmt.Errorf("sample log labels from loki: %w", err)
	}

	metricSeries, observedMetricServices, metricsRequestedFound, err := a.collectMetricLabels(ctx, start, now, params.RequestedService, labelKey)
	if err != nil {
		return model.LabelsConsistencyCheck{}, model.CategoryScore{}, nil, fmt.Errorf("sample metric labels from mimir: %w", err)
	}

	traceAttributes, observedTraceServices, tracesRequestedFound, err := a.collectTraceLabels(ctx, start, now, params.RequestedService, maxTraceSamples)
	if err != nil {
		return model.LabelsConsistencyCheck{}, model.CategoryScore{}, nil, fmt.Errorf("sample trace labels from tempo: %w", err)
	}

	check := model.LabelsConsistencyCheck{
		RequestedService:             params.RequestedService,
		ServiceLabelKey:              labelKey,
		RequiredLabels:               requiredLabels,
		LogsRequestedServiceFound:    logsRequestedFound,
		MetricsRequestedServiceFound: metricsRequestedFound,
		TracesRequestedServiceFound:  tracesRequestedFound,
		ObservedLogServices:          observedLogServices,
		ObservedMetricServices:       observedMetricServices,
		ObservedTraceServices:        observedTraceServices,
		ObservedLabels:               make(map[string]model.SignalLabelValues, len(requiredLabels)),
	}

	for _, label := range requiredLabels {
		observation := model.SignalLabelValues{
			Logs:    uniqueLogLabelValues(logEntries, logFieldKeys(label)),
			Metrics: uniqueMetricLabelValues(metricSeries, metricLabelKeys(label)),
			Traces:  uniqueTraceLabelValues(traceAttributes, traceLabelKeys(label)),
		}
		check.ObservedLabels[label] = observation

		switch {
		case isConsistentSignalValue(observation):
			check.ConsistentLabels = append(check.ConsistentLabels, label)
		case len(observation.Logs) == 0 || len(observation.Metrics) == 0 || len(observation.Traces) == 0:
			check.MissingLabels = append(check.MissingLabels, label)
		default:
			check.MismatchedLabels = append(check.MismatchedLabels, label)
		}
	}

	check.ConsistencyPercent = percentage(len(check.ConsistentLabels), len(requiredLabels))
	findings := buildLabelsConsistencyFindings(check)
	score := buildLabelsConsistencyScore(check)
	return check, score, findings, nil
}

type logLabelEntry = map[string]any

func (a *LabelsConsistencyAnalyzer) collectLogLabels(ctx context.Context, start, end time.Time, requestedService string, limit int) ([]logLabelEntry, []string, bool, error) {
	samples, err := a.loki.SampleLogs(ctx, start, end, limit)
	if err != nil {
		return nil, nil, false, err
	}

	entries := make([]logLabelEntry, 0, len(samples))
	observedServices := map[string]struct{}{}
	requestedFound := strings.TrimSpace(requestedService) == ""
	for _, sample := range samples {
		var payload map[string]any
		if err := json.Unmarshal([]byte(sample.Line), &payload); err != nil {
			continue
		}
		serviceName := firstStringField(payload, logFieldKeys("service.name")...)
		if serviceName != "" {
			observedServices[serviceName] = struct{}{}
			if serviceName == requestedService {
				requestedFound = true
			}
		}
		entries = append(entries, payload)
	}

	selected := entries
	if requestedService != "" && requestedFound {
		filtered := make([]logLabelEntry, 0, len(entries))
		for _, entry := range entries {
			if firstStringField(entry, logFieldKeys("service.name")...) == requestedService {
				filtered = append(filtered, entry)
			}
		}
		selected = filtered
	}

	return selected, sortedKeys(observedServices), requestedFound, nil
}

func (a *LabelsConsistencyAnalyzer) collectMetricLabels(ctx context.Context, start, end time.Time, requestedService, serviceLabelKey string) ([]MetricSeries, []string, bool, error) {
	requestedFound := strings.TrimSpace(requestedService) == ""
	selector, _ := buildMetricSelector(serviceLabelKey, requestedService)
	series, err := a.mimir.MatchSeries(ctx, start, end, selector)
	if err != nil {
		return nil, nil, false, err
	}
	if requestedService == "" || len(series) > 0 {
		if requestedService != "" {
			requestedFound = true
		}
		return series, uniqueLabelValues(series, serviceLabelKey), requestedFound, nil
	}

	fallbackSelector, _ := buildMetricSelector(serviceLabelKey, "")
	series, err = a.mimir.MatchSeries(ctx, start, end, fallbackSelector)
	if err != nil {
		return nil, nil, false, err
	}
	observedServices := uniqueLabelValues(series, serviceLabelKey)
	return series, observedServices, containsString(observedServices, requestedService), nil
}

func (a *LabelsConsistencyAnalyzer) collectTraceLabels(ctx context.Context, start, end time.Time, requestedService string, limit int) ([]map[string]string, []string, bool, error) {
	requestedFound := strings.TrimSpace(requestedService) == ""
	traceQuery, _ := buildTraceMetricsTraceQuery(requestedService)
	attributes, err := a.tempo.SampleTraceResourceAttributes(ctx, start, end, traceQuery, limit)
	if err != nil {
		return nil, nil, false, err
	}
	if requestedService == "" || len(attributes) > 0 {
		if requestedService != "" {
			requestedFound = true
		}
		return attributes, uniqueTraceLabelValues(attributes, traceLabelKeys("service.name")), requestedFound, nil
	}

	attributes, err = a.tempo.SampleTraceResourceAttributes(ctx, start, end, "{}", limit)
	if err != nil {
		return nil, nil, false, err
	}
	observedServices := uniqueTraceLabelValues(attributes, traceLabelKeys("service.name"))
	return attributes, observedServices, containsString(observedServices, requestedService), nil
}

func buildLabelsConsistencyFindings(check model.LabelsConsistencyCheck) []model.Finding {
	findings := make([]model.Finding, 0, 3)

	if check.RequestedService != "" && (!check.LogsRequestedServiceFound || !check.MetricsRequestedServiceFound || !check.TracesRequestedServiceFound) {
		findings = append(findings, model.Finding{
			Severity: "HIGH",
			Title:    "Requested service does not line up across sampled signals",
			Fact: fmt.Sprintf(
				"Requested service=%s; logs found=%t (%s), metrics found=%t (%s), traces found=%t (%s).",
				check.RequestedService,
				check.LogsRequestedServiceFound,
				displayValues(check.ObservedLogServices),
				check.MetricsRequestedServiceFound,
				displayValues(check.ObservedMetricServices),
				check.TracesRequestedServiceFound,
				displayValues(check.ObservedTraceServices),
			),
			Impact:         "Service-scoped label comparisons fall back to unrelated telemetry, which usually means service.name drift between signals.",
			Recommendation: "Align service.name across logs, metrics, and traces before relying on service-scoped scans.",
		})
	}

	if len(check.MissingLabels) > 0 {
		details := make([]string, 0, len(check.MissingLabels))
		for _, label := range check.MissingLabels {
			details = append(details, fmt.Sprintf("%s missing in %s", label, strings.Join(missingSignals(check.ObservedLabels[label]), "/")))
		}
		findings = append(findings, model.Finding{
			Severity:       "HIGH",
			Title:          "Required shared labels are missing from one or more signals",
			Fact:           strings.Join(details, "; ") + ".",
			Impact:         "Cross-signal pivots become fragile when logs, metrics, and traces do not carry the same identifying attributes.",
			Recommendation: "Promote the required resource attributes into log fields, metric labels, and trace resources consistently across the pipeline.",
		})
	}

	if len(check.MismatchedLabels) > 0 {
		details := make([]string, 0, len(check.MismatchedLabels))
		for _, label := range check.MismatchedLabels {
			details = append(details, fmt.Sprintf("%s=%s", label, describeSignalValues(check.ObservedLabels[label])))
		}

		severity := "MEDIUM"
		if containsString(check.MismatchedLabels, "service.name") {
			severity = "HIGH"
		}
		findings = append(findings, model.Finding{
			Severity:       severity,
			Title:          "Shared label values differ across signals",
			Fact:           strings.Join(details, "; ") + ".",
			Impact:         "Operators will see different service identities depending on whether they start from logs, metrics, or traces.",
			Recommendation: "Standardize shared resource attributes at the SDK or collector layer so every signal emits the same canonical values.",
		})
	}

	return findings
}

func buildLabelsConsistencyScore(check model.LabelsConsistencyCheck) model.CategoryScore {
	ratio := 0.0
	if len(check.RequiredLabels) > 0 {
		ratio = float64(len(check.ConsistentLabels)) / float64(len(check.RequiredLabels))
	}
	earned := int(math.Round(float64(labelsConsistencyWeight) * ratio))
	return toModelCategoryScore(buildCategoryScore("labels_consistency", "Shared Label Consistency", earned, labelsConsistencyWeight))
}

func uniqueLogLabelValues(entries []logLabelEntry, keys []string) []string {
	values := map[string]struct{}{}
	for _, entry := range entries {
		if value := firstStringField(entry, keys...); value != "" {
			values[value] = struct{}{}
		}
	}
	return sortedKeys(values)
}

func uniqueMetricLabelValues(series []MetricSeries, keys []string) []string {
	values := map[string]struct{}{}
	for _, item := range series {
		for _, key := range keys {
			if value := strings.TrimSpace(item.Labels[key]); value != "" {
				values[value] = struct{}{}
				break
			}
		}
	}
	return sortedKeys(values)
}

func uniqueTraceLabelValues(attributes []map[string]string, keys []string) []string {
	values := map[string]struct{}{}
	for _, resourceAttributes := range attributes {
		for _, key := range keys {
			if value := strings.TrimSpace(resourceAttributes[key]); value != "" {
				values[value] = struct{}{}
				break
			}
		}
	}
	return sortedKeys(values)
}

func isConsistentSignalValue(observation model.SignalLabelValues) bool {
	return len(observation.Logs) == 1 &&
		len(observation.Metrics) == 1 &&
		len(observation.Traces) == 1 &&
		observation.Logs[0] == observation.Metrics[0] &&
		observation.Metrics[0] == observation.Traces[0]
}

func missingSignals(observation model.SignalLabelValues) []string {
	missing := make([]string, 0, 3)
	if len(observation.Logs) == 0 {
		missing = append(missing, "logs")
	}
	if len(observation.Metrics) == 0 {
		missing = append(missing, "metrics")
	}
	if len(observation.Traces) == 0 {
		missing = append(missing, "traces")
	}
	return missing
}

func describeSignalValues(observation model.SignalLabelValues) string {
	return fmt.Sprintf(
		"logs=%s, metrics=%s, traces=%s",
		displayValues(observation.Logs),
		displayValues(observation.Metrics),
		displayValues(observation.Traces),
	)
}

func displayValues(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func logFieldKeys(label string) []string {
	switch label {
	case "service.name", "service_name":
		return []string{"service_name", "service.name"}
	case "service.namespace", "service_namespace":
		return []string{"service_namespace", "service.namespace"}
	case "deployment.environment", "deployment_environment":
		return []string{"deployment_environment", "deployment.environment"}
	default:
		return []string{label}
	}
}

func metricLabelKeys(label string) []string {
	switch label {
	case "service.name", "service_name":
		return []string{"service_name", "service.name"}
	case "service.namespace", "service_namespace":
		return []string{"service_namespace", "service.namespace"}
	case "deployment.environment", "deployment_environment":
		return []string{"deployment_environment", "deployment.environment"}
	default:
		return []string{label}
	}
}

func traceLabelKeys(label string) []string {
	switch label {
	case "service.name", "service_name":
		return []string{"service.name", "service_name"}
	case "service.namespace", "service_namespace":
		return []string{"service.namespace", "service_namespace"}
	case "deployment.environment", "deployment_environment":
		return []string{"deployment.environment", "deployment_environment"}
	default:
		return []string{label}
	}
}
