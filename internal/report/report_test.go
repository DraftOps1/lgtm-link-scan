package report

import (
	"strings"
	"testing"
)

func TestRenderMarkdown(t *testing.T) {
	t.Parallel()

	rendered, err := Render([]byte(sampleReportInput), "markdown")
	if err != nil {
		t.Fatalf("Render(markdown): %v", err)
	}

	text := string(rendered)
	if !strings.Contains(text, "# LGTM Link Quality Report") {
		t.Fatalf("missing title: %s", text)
	}
	if !strings.Contains(text, "## Findings") {
		t.Fatalf("missing findings section: %s", text)
	}
	if !strings.Contains(text, "Shared Label Consistency") {
		t.Fatalf("missing labels section: %s", text)
	}
}

func TestRenderJSON(t *testing.T) {
	t.Parallel()

	rendered, err := Render([]byte(sampleReportInput), "json")
	if err != nil {
		t.Fatalf("Render(json): %v", err)
	}
	if !strings.Contains(string(rendered), "\"generated_at\"") {
		t.Fatalf("unexpected output: %s", string(rendered))
	}
}

func TestRenderRejectsBadJSON(t *testing.T) {
	t.Parallel()

	_, err := Render([]byte("{"), "markdown")
	if err == nil {
		t.Fatal("Render returned nil error")
	}
}

const sampleReportInput = `{
  "generated_at": "2026-04-02T11:36:10Z",
  "partial": false,
  "service": "checkout",
  "lookback": "30m",
  "score": {"earned": 100, "available": 100, "percent": 100, "severity": "LOW"},
  "checks": [
    {"id": "log_trace", "title": "Log -> Trace", "earned": 30, "available": 30, "percent": 100, "status": "pass"}
  ],
  "findings": [],
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
  "labels_consistency": {
    "required_labels": ["service.name"],
    "consistent_labels": ["service.name"],
    "consistency_percent": 100
  }
}`
