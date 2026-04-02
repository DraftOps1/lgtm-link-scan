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

const logTraceWeight = 30

type LogSample struct {
	Timestamp time.Time
	Line      string
}

type LogSampler interface {
	SampleLogs(ctx context.Context, start, end time.Time, limit int) ([]LogSample, error)
}

type TraceResolver interface {
	TraceExists(ctx context.Context, traceID string) (bool, error)
}

type LogTraceParams struct {
	Lookback           time.Duration
	RequestedService   string
	RequiredTraceKeys  []string
	MinCoveragePercent float64
	MaxLogSamples      int
	RequiredSpanKeys   []string
}

type LogTraceAnalyzer struct {
	loki  LogSampler
	tempo TraceResolver
}

func NewLogTraceAnalyzer(loki LogSampler, tempo TraceResolver) *LogTraceAnalyzer {
	return &LogTraceAnalyzer{
		loki:  loki,
		tempo: tempo,
	}
}

func (a *LogTraceAnalyzer) Analyze(ctx context.Context, now time.Time, params LogTraceParams) (model.LogTraceCheck, model.CategoryScore, []model.Finding, error) {
	start := now.Add(-params.Lookback)
	samples, err := a.loki.SampleLogs(ctx, start, now, params.MaxLogSamples)
	if err != nil {
		return model.LogTraceCheck{}, model.CategoryScore{}, nil, fmt.Errorf("sample logs from loki: %w", err)
	}

	requiredTraceKeys := params.RequiredTraceKeys
	if len(requiredTraceKeys) == 0 {
		requiredTraceKeys = []string{"trace_id", "traceid"}
	}
	requiredSpanKeys := params.RequiredSpanKeys
	if len(requiredSpanKeys) == 0 {
		requiredSpanKeys = []string{"span_id", "spanid"}
	}

	entries := make([]parsedLogEntry, 0, len(samples))
	observedServices := map[string]struct{}{}
	requestedServiceFound := params.RequestedService == ""
	for _, sample := range samples {
		entry, err := parseLogEntry(sample, requiredTraceKeys, requiredSpanKeys)
		if err != nil {
			continue
		}
		if entry.ServiceName != "" {
			observedServices[entry.ServiceName] = struct{}{}
			if entry.ServiceName == params.RequestedService {
				requestedServiceFound = true
			}
		}
		entries = append(entries, entry)
	}

	selectedEntries := entries
	if params.RequestedService != "" {
		filtered := filterEntriesByService(entries, params.RequestedService)
		if len(filtered) > 0 {
			selectedEntries = filtered
		}
	}

	check := model.LogTraceCheck{
		RequestedService:      params.RequestedService,
		RequestedServiceFound: requestedServiceFound,
		ObservedServices:      sortedKeys(observedServices),
		SampledLogs:           len(selectedEntries),
	}

	if len(selectedEntries) == 0 {
		findings := []model.Finding{{
			Severity:       "HIGH",
			Title:          "No logs available for Log -> Trace analysis",
			Fact:           fmt.Sprintf("Loki returned 0 parseable log lines in the last %s.", params.Lookback),
			Impact:         "The scanner cannot measure whether operators can pivot from logs to traces.",
			Recommendation: "Confirm log ingestion into Loki and widen the lookback window if the service is mostly idle.",
		}}
		score := toModelCategoryScore(buildCategoryScore("log_trace", "Log -> Trace", 0, logTraceWeight))
		return check, score, findings, nil
	}

	traceResolutionCache := make(map[string]bool)
	for _, entry := range selectedEntries {
		if entry.TraceID != "" {
			check.LogsWithTraceID++
		}
		if entry.SpanID != "" {
			check.LogsWithSpanID++
		}
		if entry.TraceID == "" {
			continue
		}

		check.TraceIDsChecked++

		resolved, ok := traceResolutionCache[entry.TraceID]
		if !ok {
			resolved, err = a.tempo.TraceExists(ctx, entry.TraceID)
			if err != nil {
				return model.LogTraceCheck{}, model.CategoryScore{}, nil, fmt.Errorf("resolve trace_id %s in tempo: %w", entry.TraceID, err)
			}
			traceResolutionCache[entry.TraceID] = resolved
		}
		if resolved {
			check.TraceIDsResolved++
		}
	}

	check.TraceCoveragePercent = percentage(check.LogsWithTraceID, check.SampledLogs)
	check.SpanCoveragePercent = percentage(check.LogsWithSpanID, check.SampledLogs)
	check.ResolutionPercent = percentage(check.TraceIDsResolved, check.TraceIDsChecked)

	findings := buildLogTraceFindings(check, params)
	score := buildLogTraceScore(check)

	return check, score, findings, nil
}

type parsedLogEntry struct {
	ServiceName string
	TraceID     string
	SpanID      string
}

func parseLogEntry(sample LogSample, traceKeys, spanKeys []string) (parsedLogEntry, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(sample.Line), &payload); err != nil {
		return parsedLogEntry{}, err
	}

	return parsedLogEntry{
		ServiceName: stringField(payload, "service_name"),
		TraceID:     firstStringField(payload, traceKeys...),
		SpanID:      firstStringField(payload, spanKeys...),
	}, nil
}

func filterEntriesByService(entries []parsedLogEntry, service string) []parsedLogEntry {
	filtered := make([]parsedLogEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.ServiceName == service {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func buildLogTraceFindings(check model.LogTraceCheck, params LogTraceParams) []model.Finding {
	findings := make([]model.Finding, 0, 3)

	if params.RequestedService != "" && !check.RequestedServiceFound && len(check.ObservedServices) > 0 {
		findings = append(findings, model.Finding{
			Severity:       "MEDIUM",
			Title:          "Requested service did not match sampled log service names",
			Fact:           fmt.Sprintf("Requested service=%s was not present in sampled logs; observed services=%s.", params.RequestedService, strings.Join(check.ObservedServices, ", ")),
			Impact:         "The Log -> Trace check had to fall back to all sampled logs, which usually points to a service.name mismatch between signals.",
			Recommendation: "Align service.name across logs, metrics, and traces before relying on service-scoped scans.",
		})
	}

	if check.TraceCoveragePercent < params.MinCoveragePercent {
		findings = append(findings, model.Finding{
			Severity:       severityForCoverage(check.TraceCoveragePercent),
			Title:          "Logs are missing trace correlation fields",
			Fact:           fmt.Sprintf("%.1f%% of sampled logs contain a trace ID; the configured minimum is %.1f%%.", check.TraceCoveragePercent, params.MinCoveragePercent),
			Impact:         "Operators cannot pivot from logs to traces reliably during incident investigation.",
			Recommendation: "Inject trace_id and span_id into structured logs at the application middleware layer.",
		})
	}

	if check.SpanCoveragePercent < check.TraceCoveragePercent {
		findings = append(findings, model.Finding{
			Severity:       "MEDIUM",
			Title:          "Logs have trace IDs without matching span IDs",
			Fact:           fmt.Sprintf("%.1f%% of sampled logs include span_id, while %.1f%% include trace_id.", check.SpanCoveragePercent, check.TraceCoveragePercent),
			Impact:         "Investigators may find the right trace but still lose span-level context from the originating log line.",
			Recommendation: "Emit both trace_id and span_id from the same structured logging hook.",
		})
	}

	if check.TraceIDsChecked > 0 && check.ResolutionPercent < 100 {
		findings = append(findings, model.Finding{
			Severity:       severityForCoverage(check.ResolutionPercent),
			Title:          "Some trace IDs from logs do not resolve in Tempo",
			Fact:           fmt.Sprintf("%.1f%% of trace IDs sampled from logs resolved in Tempo.", check.ResolutionPercent),
			Impact:         "Log lines appear correlated, but Tempo pivots will fail for unresolved traces.",
			Recommendation: "Verify that traces and logs are exported from the same service identity and retained for the same time window.",
		})
	}

	return findings
}

func buildLogTraceScore(check model.LogTraceCheck) model.CategoryScore {
	traceCoverageRatio := float64(check.LogsWithTraceID) / float64(max(check.SampledLogs, 1))
	resolutionRatio := 0.0
	if check.TraceIDsChecked > 0 {
		resolutionRatio = float64(check.TraceIDsResolved) / float64(check.TraceIDsChecked)
	}

	earned := int(math.Round(float64(logTraceWeight) * traceCoverageRatio * resolutionRatio))
	return toModelCategoryScore(buildCategoryScore("log_trace", "Log -> Trace", earned, logTraceWeight))
}

func stringField(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func firstStringField(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringField(payload, key); value != "" {
			return value
		}
	}
	return ""
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
