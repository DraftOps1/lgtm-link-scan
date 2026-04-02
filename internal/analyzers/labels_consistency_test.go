package analyzers

import (
	"context"
	"testing"
	"time"
)

func TestLabelsConsistencyAnalyzerGoodCoverage(t *testing.T) {
	t.Parallel()

	analyzer := NewLabelsConsistencyAnalyzer(
		fakeLogSampler{samples: []LogSample{
			{Line: `{"service_name":"checkout","service_namespace":"store","deployment_environment":"demo","trace_id":"trace-1"}`},
		}},
		fakeMetricSeriesFinder{
			series: []MetricSeries{
				{Labels: map[string]string{
					"__name__":               "demo_requests_total",
					"service_name":           "checkout",
					"service_namespace":      "store",
					"deployment_environment": "demo",
				}},
			},
		},
		fakeTraceAttributeSampler{attributes: []map[string]string{{
			"service.name":           "checkout",
			"service.namespace":      "store",
			"deployment.environment": "demo",
		}}},
	)

	check, score, findings, err := analyzer.Analyze(context.Background(), time.Unix(200, 0), LabelsConsistencyParams{
		Lookback:         30 * time.Minute,
		RequestedService: "checkout",
		ServiceLabelKey:  "service_name",
		RequiredLabels:   []string{"service.name", "service.namespace", "deployment.environment"},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if len(check.ConsistentLabels) != 3 || check.ConsistencyPercent != 100 {
		t.Fatalf("unexpected check: %+v", check)
	}
	if score.Earned != 20 || score.Status != "pass" {
		t.Fatalf("unexpected score: %+v", score)
	}
	if len(findings) != 0 {
		t.Fatalf("unexpected findings: %+v", findings)
	}
}

func TestLabelsConsistencyAnalyzerServiceMismatch(t *testing.T) {
	t.Parallel()

	analyzer := NewLabelsConsistencyAnalyzer(
		fakeLogSampler{samples: []LogSample{
			{Line: `{"service_name":"checkout-api","service_namespace":"store","deployment_environment":"demo"}`},
		}},
		fakeMetricSeriesFinder{
			series: []MetricSeries{
				{Labels: map[string]string{
					"__name__":               "demo_requests_total",
					"service_name":           "checkout",
					"service_namespace":      "store",
					"deployment_environment": "demo",
				}},
			},
		},
		fakeTraceAttributeSampler{attributes: []map[string]string{{
			"service.name":           "checkout",
			"service.namespace":      "store",
			"deployment.environment": "demo",
		}}},
	)

	check, score, findings, err := analyzer.Analyze(context.Background(), time.Unix(200, 0), LabelsConsistencyParams{
		Lookback:         30 * time.Minute,
		RequestedService: "checkout",
		ServiceLabelKey:  "service_name",
		RequiredLabels:   []string{"service.name", "service.namespace", "deployment.environment"},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if len(check.MismatchedLabels) != 1 || check.MismatchedLabels[0] != "service.name" {
		t.Fatalf("unexpected mismatched labels: %+v", check)
	}
	if score.Earned != 13 || score.Status != "warn" {
		t.Fatalf("unexpected score: %+v", score)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings")
	}
}

func TestLabelsConsistencyAnalyzerMissingLabel(t *testing.T) {
	t.Parallel()

	analyzer := NewLabelsConsistencyAnalyzer(
		fakeLogSampler{samples: []LogSample{
			{Line: `{"service_name":"checkout","service_namespace":"store"}`},
		}},
		fakeMetricSeriesFinder{
			series: []MetricSeries{
				{Labels: map[string]string{
					"__name__":               "demo_requests_total",
					"service_name":           "checkout",
					"service_namespace":      "store",
					"deployment_environment": "demo",
				}},
			},
		},
		fakeTraceAttributeSampler{attributes: []map[string]string{{
			"service.name":           "checkout",
			"service.namespace":      "store",
			"deployment.environment": "demo",
		}}},
	)

	check, score, findings, err := analyzer.Analyze(context.Background(), time.Unix(200, 0), LabelsConsistencyParams{
		Lookback:         30 * time.Minute,
		RequestedService: "checkout",
		ServiceLabelKey:  "service_name",
		RequiredLabels:   []string{"service.name", "service.namespace", "deployment.environment"},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if len(check.MissingLabels) != 1 || check.MissingLabels[0] != "deployment.environment" {
		t.Fatalf("unexpected missing labels: %+v", check)
	}
	if score.Earned != 13 || score.Status != "warn" {
		t.Fatalf("unexpected score: %+v", score)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings")
	}
}

type fakeTraceAttributeSampler struct {
	attributes []map[string]string
	err        error
}

func (f fakeTraceAttributeSampler) SampleTraceResourceAttributes(context.Context, time.Time, time.Time, string, int) ([]map[string]string, error) {
	return f.attributes, f.err
}
