package report

import (
	"fmt"
	"strings"

	"github.com/DraftOps1/lgtm-link-scan/internal/model"
)

func renderMarkdown(result model.ScanResult) []byte {
	var b strings.Builder

	b.WriteString("# LGTM Link Quality Report\n\n")
	b.WriteString("## Overview\n\n")
	fmt.Fprintf(&b, "- Generated at: `%s`\n", result.GeneratedAt.UTC().Format("2006-01-02 15:04:05 MST"))
	if result.Service != "" {
		fmt.Fprintf(&b, "- Service: `%s`\n", result.Service)
	} else {
		b.WriteString("- Service: all services\n")
	}
	fmt.Fprintf(&b, "- Lookback: `%s`\n", result.Lookback)
	fmt.Fprintf(&b, "- Overall score: `%d/%d (%d%%)`\n", result.Score.Earned, result.Score.Available, result.Score.Percent)
	fmt.Fprintf(&b, "- Severity: `%s`\n", result.Score.Severity)
	fmt.Fprintf(&b, "- Partial: `%t`\n", result.Partial)

	b.WriteString("\n## Checks\n\n")
	b.WriteString("| Check | Score | Status |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, check := range result.Checks {
		fmt.Fprintf(&b, "| %s | `%d/%d (%d%%)` | `%s` |\n", check.Title, check.Earned, check.Available, check.Percent, strings.ToUpper(check.Status))
	}

	b.WriteString("\n## Findings\n\n")
	if len(result.Findings) == 0 {
		b.WriteString("No findings.\n")
	} else {
		for _, finding := range result.Findings {
			fmt.Fprintf(&b, "### [%s] %s\n\n", finding.Severity, finding.Title)
			fmt.Fprintf(&b, "- Fact: %s\n", finding.Fact)
			fmt.Fprintf(&b, "- Impact: %s\n", finding.Impact)
			fmt.Fprintf(&b, "- Recommendation: %s\n\n", finding.Recommendation)
		}
	}

	b.WriteString("## Details\n\n")
	writeLogTraceDetails(&b, result.LogTrace)
	writeMetricTraceDetails(&b, result.MetricTrace)
	writeTraceMetricsDetails(&b, result.TraceMetrics)
	writeLabelsConsistencyDetails(&b, result.LabelsConsistency)

	return []byte(strings.TrimRight(b.String(), "\n"))
}

func writeLogTraceDetails(b *strings.Builder, check *model.LogTraceCheck) {
	if check == nil {
		return
	}
	b.WriteString("### Log -> Trace\n\n")
	fmt.Fprintf(b, "- Sampled logs: `%d`\n", check.SampledLogs)
	fmt.Fprintf(b, "- Logs with trace_id: `%d` (`%.1f%%`)\n", check.LogsWithTraceID, check.TraceCoveragePercent)
	fmt.Fprintf(b, "- Logs with span_id: `%d` (`%.1f%%`)\n", check.LogsWithSpanID, check.SpanCoveragePercent)
	fmt.Fprintf(b, "- Resolved trace IDs: `%d/%d` (`%.1f%%`)\n\n", check.TraceIDsResolved, check.TraceIDsChecked, check.ResolutionPercent)
}

func writeMetricTraceDetails(b *strings.Builder, check *model.MetricTraceCheck) {
	if check == nil {
		return
	}
	b.WriteString("### Metric -> Trace\n\n")
	fmt.Fprintf(b, "- Selector: `%s`\n", check.Selector)
	fmt.Fprintf(b, "- Series matched: `%d`\n", check.SeriesMatched)
	fmt.Fprintf(b, "- Series with exemplars: `%d`\n", check.SeriesWithExemplars)
	fmt.Fprintf(b, "- Exemplars found: `%d`\n", check.ExemplarsFound)
	fmt.Fprintf(b, "- Exemplar coverage: `%.1f%%`\n", check.ExemplarCoveragePercent)
	fmt.Fprintf(b, "- Resolved exemplar trace IDs: `%d/%d` (`%.1f%%`)\n\n", check.TraceIDsResolved, check.TraceIDsChecked, check.ResolutionPercent)
}

func writeTraceMetricsDetails(b *strings.Builder, check *model.TraceMetricsCheck) {
	if check == nil {
		return
	}
	b.WriteString("### Trace -> Metric\n\n")
	fmt.Fprintf(b, "- Trace query: `%s`\n", check.TraceQuery)
	fmt.Fprintf(b, "- Metric selector: `%s`\n", check.MetricSelector)
	fmt.Fprintf(b, "- Trace services: `%s`\n", joinOrNone(check.TraceServices))
	fmt.Fprintf(b, "- Metric services: `%s`\n", joinOrNone(check.MetricServices))
	fmt.Fprintf(b, "- Matched required metrics: `%s`\n", joinOrNone(check.MatchedRequiredMetrics))
	fmt.Fprintf(b, "- Missing required metrics: `%s`\n", joinOrNone(check.MissingRequiredMetrics))
	fmt.Fprintf(b, "- Service name aligned: `%t`\n\n", check.ServiceNameAligned)
}

func writeLabelsConsistencyDetails(b *strings.Builder, check *model.LabelsConsistencyCheck) {
	if check == nil {
		return
	}
	b.WriteString("### Shared Label Consistency\n\n")
	fmt.Fprintf(b, "- Required labels: `%s`\n", joinOrNone(check.RequiredLabels))
	fmt.Fprintf(b, "- Consistent labels: `%s`\n", joinOrNone(check.ConsistentLabels))
	fmt.Fprintf(b, "- Missing labels: `%s`\n", joinOrNone(check.MissingLabels))
	fmt.Fprintf(b, "- Mismatched labels: `%s`\n", joinOrNone(check.MismatchedLabels))
	fmt.Fprintf(b, "- Consistency: `%.1f%%`\n\n", check.ConsistencyPercent)
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}
