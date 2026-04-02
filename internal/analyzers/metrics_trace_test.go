package analyzers

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMetricTraceAnalyzerGoodCoverage(t *testing.T) {
	t.Parallel()

	analyzer := NewMetricTraceAnalyzer(
		fakeMetricSeriesFinder{
			series: []MetricSeries{
				{Labels: map[string]string{"__name__": "demo_request_duration_seconds_bucket", "service_name": "checkout"}},
			},
			exemplarSeries: []ExemplarSeries{
				{
					SeriesLabels: map[string]string{"__name__": "demo_request_duration_seconds_bucket"},
					Exemplars: []Exemplar{
						{Labels: map[string]string{"trace_id": "trace-1"}},
						{Labels: map[string]string{"trace_id": "trace-2"}},
					},
				},
			},
		},
		fakeTraceResolver{resolved: map[string]bool{"trace-1": true, "trace-2": true}},
	)

	check, score, findings, err := analyzer.Analyze(context.Background(), time.Unix(200, 0), MetricTraceParams{
		Lookback:                   30 * time.Minute,
		RequestedService:           "checkout",
		ServiceLabelKey:            "service_name",
		MinExemplarCoveragePercent: 50,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if check.ExemplarsFound != 2 || check.TraceIDsResolved != 2 {
		t.Fatalf("unexpected check: %+v", check)
	}
	if score.Earned != 30 || score.Status != "pass" {
		t.Fatalf("unexpected score: %+v", score)
	}
	if len(findings) != 0 {
		t.Fatalf("unexpected findings: %+v", findings)
	}
}

func TestMetricTraceAnalyzerMissingExemplars(t *testing.T) {
	t.Parallel()

	analyzer := NewMetricTraceAnalyzer(
		fakeMetricSeriesFinder{
			series: []MetricSeries{
				{Labels: map[string]string{"__name__": "demo_requests_total", "service_name": "checkout"}},
			},
		},
		fakeTraceResolver{},
	)

	check, score, findings, err := analyzer.Analyze(context.Background(), time.Unix(200, 0), MetricTraceParams{
		Lookback:                   30 * time.Minute,
		RequestedService:           "checkout",
		ServiceLabelKey:            "service_name",
		MinExemplarCoveragePercent: 50,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if check.ExemplarCoveragePercent != 0 || check.ExemplarsFound != 0 {
		t.Fatalf("unexpected check: %+v", check)
	}
	if score.Earned != 0 || score.Status != "fail" {
		t.Fatalf("unexpected score: %+v", score)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings")
	}
}

func TestMetricTraceAnalyzerResolverError(t *testing.T) {
	t.Parallel()

	analyzer := NewMetricTraceAnalyzer(
		fakeMetricSeriesFinder{
			series: []MetricSeries{
				{Labels: map[string]string{"__name__": "demo_request_duration_seconds_bucket", "service_name": "checkout"}},
			},
			exemplarSeries: []ExemplarSeries{
				{
					SeriesLabels: map[string]string{"__name__": "demo_request_duration_seconds_bucket"},
					Exemplars: []Exemplar{
						{Labels: map[string]string{"trace_id": "trace-1"}},
					},
				},
			},
		},
		fakeTraceResolver{err: errors.New("tempo unavailable")},
	)

	_, _, _, err := analyzer.Analyze(context.Background(), time.Unix(200, 0), MetricTraceParams{
		Lookback:                   30 * time.Minute,
		RequestedService:           "checkout",
		ServiceLabelKey:            "service_name",
		MinExemplarCoveragePercent: 50,
	})
	if err == nil {
		t.Fatal("Analyze returned nil error")
	}
}

type fakeMetricSeriesFinder struct {
	series         []MetricSeries
	seriesErr      error
	exemplarSeries []ExemplarSeries
	exemplarErr    error
}

func (f fakeMetricSeriesFinder) MatchSeries(context.Context, time.Time, time.Time, string) ([]MetricSeries, error) {
	return f.series, f.seriesErr
}

func (f fakeMetricSeriesFinder) QueryExemplars(context.Context, time.Time, time.Time, string) ([]ExemplarSeries, error) {
	return f.exemplarSeries, f.exemplarErr
}
