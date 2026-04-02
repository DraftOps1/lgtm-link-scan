package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunReportWritesMarkdownFile(t *testing.T) {
	t.Parallel()

	inputPath := filepath.Join(t.TempDir(), "result.json")
	outputPath := filepath.Join(t.TempDir(), "report.md")
	if err := os.WriteFile(inputPath, []byte(sampleScanResultJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(input): %v", err)
	}

	var stdout bytes.Buffer
	if err := runReport(&stdout, inputPath, "markdown", outputPath); err != nil {
		t.Fatalf("runReport: %v", err)
	}

	rendered, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(output): %v", err)
	}
	if !strings.Contains(string(rendered), "# LGTM Link Quality Report") {
		t.Fatalf("unexpected markdown: %s", string(rendered))
	}
	if !strings.Contains(stdout.String(), "report: wrote") {
		t.Fatalf("missing write confirmation: %s", stdout.String())
	}
}

func TestRunReportWritesJSONToStdout(t *testing.T) {
	t.Parallel()

	inputPath := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(inputPath, []byte(sampleScanResultJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(input): %v", err)
	}

	var stdout bytes.Buffer
	if err := runReport(&stdout, inputPath, "json", ""); err != nil {
		t.Fatalf("runReport: %v", err)
	}
	if !strings.Contains(stdout.String(), "\"score\"") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
}

func TestRunReportRejectsUnknownFormat(t *testing.T) {
	t.Parallel()

	inputPath := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(inputPath, []byte(sampleScanResultJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(input): %v", err)
	}

	err := runReport(&bytes.Buffer{}, inputPath, "html", "")
	if err == nil {
		t.Fatal("runReport returned nil error")
	}
	if !strings.Contains(err.Error(), "unsupported report format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

const sampleScanResultJSON = `{
  "generated_at": "2026-04-02T11:36:10Z",
  "partial": false,
  "implemented_checks": ["log_trace", "metric_trace", "trace_metrics", "labels_consistency"],
  "service": "checkout",
  "lookback": "30m",
  "score": {"earned": 90, "available": 100, "percent": 90, "severity": "LOW"},
  "checks": [
    {"id": "log_trace", "title": "Log -> Trace", "earned": 30, "available": 30, "percent": 100, "status": "pass"},
    {"id": "metric_trace", "title": "Metric -> Trace", "earned": 20, "available": 30, "percent": 67, "status": "warn"}
  ],
  "findings": [
    {
      "severity": "MEDIUM",
      "title": "Metrics are missing trace-linked exemplars",
      "fact": "67.0% exemplar coverage for service_name=checkout; configured minimum is 80.0%.",
      "impact": "Operators cannot pivot from metrics to traces reliably.",
      "recommendation": "Enable exemplar emission."
    }
  ],
  "log_trace": {
    "sampled_logs": 10,
    "logs_with_trace_id": 10,
    "logs_with_span_id": 10,
    "trace_ids_checked": 10,
    "trace_ids_resolved": 10,
    "trace_coverage_percent": 100,
    "span_coverage_percent": 100,
    "resolution_percent": 100
  },
  "metric_trace": {
    "selector": "{service_name=\"checkout\"}",
    "series_matched": 20,
    "series_with_exemplars": 3,
    "exemplars_found": 12,
    "trace_ids_checked": 12,
    "trace_ids_resolved": 12,
    "exemplar_coverage_percent": 67,
    "resolution_percent": 100
  },
  "trace_metrics": {
    "trace_query": "{ resource.service.name = \"checkout\" }",
    "metric_selector": "{__name__=~\"traces_.*\",service_name=\"checkout\"}",
    "trace_services": ["checkout"],
    "metric_services": ["checkout"],
    "matched_required_metrics": ["calls"],
    "missing_required_metrics": ["duration"],
    "service_name_aligned": true
  },
  "labels_consistency": {
    "required_labels": ["service.name", "service.namespace", "deployment.environment"],
    "consistent_labels": ["service.name", "service.namespace"],
    "missing_labels": ["deployment.environment"],
    "consistency_percent": 67
  }
}`
