package analyzers

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTraceMetricsAnalyzerGoodCoverage(t *testing.T) {
	t.Parallel()

	analyzer := NewTraceMetricsAnalyzer(
		fakeTraceServiceFinder{services: []string{"checkout"}},
		fakeMetricSeriesFinder{
			series: []MetricSeries{
				{Labels: map[string]string{"__name__": "traces_span_metrics_calls_total", "service_name": "checkout"}},
				{Labels: map[string]string{"__name__": "traces_span_metrics_duration_milliseconds_count", "service_name": "checkout"}},
			},
		},
	)

	check, score, findings, err := analyzer.Analyze(context.Background(), time.Unix(200, 0), TraceMetricsParams{
		Lookback:            30 * time.Minute,
		RequestedService:    "checkout",
		ServiceLabelKey:     "service_name",
		RequiredMetricNames: []string{"calls", "duration"},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if !check.ServiceNameAligned || len(check.MissingRequiredMetrics) != 0 {
		t.Fatalf("unexpected check: %+v", check)
	}
	if score.Earned != 20 || score.Status != "pass" {
		t.Fatalf("unexpected score: %+v", score)
	}
	if len(findings) != 0 {
		t.Fatalf("unexpected findings: %+v", findings)
	}
}

func TestTraceMetricsAnalyzerMissingMetrics(t *testing.T) {
	t.Parallel()

	analyzer := NewTraceMetricsAnalyzer(
		fakeTraceServiceFinder{services: []string{"checkout"}},
		fakeMetricSeriesFinder{
			series: []MetricSeries{
				{Labels: map[string]string{"__name__": "demo_requests_total", "service_name": "checkout"}},
			},
		},
	)

	check, score, findings, err := analyzer.Analyze(context.Background(), time.Unix(200, 0), TraceMetricsParams{
		Lookback:            30 * time.Minute,
		RequestedService:    "checkout",
		ServiceLabelKey:     "service_name",
		RequiredMetricNames: []string{"calls"},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if len(check.ObservedMetrics) != 1 || score.Earned != 0 {
		t.Fatalf("unexpected check/score: %+v %+v", check, score)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings")
	}
}

func TestTraceMetricsAnalyzerTempoError(t *testing.T) {
	t.Parallel()

	analyzer := NewTraceMetricsAnalyzer(
		fakeTraceServiceFinder{err: errors.New("tempo unavailable")},
		fakeMetricSeriesFinder{},
	)

	_, _, _, err := analyzer.Analyze(context.Background(), time.Unix(200, 0), TraceMetricsParams{
		Lookback:         30 * time.Minute,
		RequestedService: "checkout",
		ServiceLabelKey:  "service_name",
	})
	if err == nil {
		t.Fatal("Analyze returned nil error")
	}
}

type fakeTraceServiceFinder struct {
	services []string
	err      error
}

func (f fakeTraceServiceFinder) SearchServiceNames(context.Context, time.Time, time.Time, string, int) ([]string, error) {
	return f.services, f.err
}
