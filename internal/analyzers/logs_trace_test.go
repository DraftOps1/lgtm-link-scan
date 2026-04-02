package analyzers

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLogTraceAnalyzerGoodCoverage(t *testing.T) {
	t.Parallel()

	analyzer := NewLogTraceAnalyzer(
		fakeLogSampler{samples: []LogSample{
			{Line: `{"service_name":"checkout","trace_id":"trace-1","span_id":"span-1"}`},
			{Line: `{"service_name":"checkout","trace_id":"trace-2","span_id":"span-2"}`},
		}},
		fakeTraceResolver{resolved: map[string]bool{"trace-1": true, "trace-2": true}},
	)

	check, score, findings, err := analyzer.Analyze(context.Background(), time.Unix(200, 0), LogTraceParams{
		Lookback:           30 * time.Minute,
		RequestedService:   "checkout",
		RequiredTraceKeys:  []string{"trace_id"},
		RequiredSpanKeys:   []string{"span_id"},
		MinCoveragePercent: 80,
		MaxLogSamples:      50,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if check.SampledLogs != 2 || check.TraceIDsResolved != 2 {
		t.Fatalf("unexpected check: %+v", check)
	}
	if score.Earned != 30 || score.Status != "pass" {
		t.Fatalf("unexpected score: %+v", score)
	}
	if len(findings) != 0 {
		t.Fatalf("unexpected findings: %+v", findings)
	}
}

func TestLogTraceAnalyzerMissingTraceContext(t *testing.T) {
	t.Parallel()

	analyzer := NewLogTraceAnalyzer(
		fakeLogSampler{samples: []LogSample{
			{Line: `{"service_name":"checkout-api","msg":"started"}`},
			{Line: `{"service_name":"checkout-api","msg":"completed"}`},
		}},
		fakeTraceResolver{},
	)

	check, score, findings, err := analyzer.Analyze(context.Background(), time.Unix(200, 0), LogTraceParams{
		Lookback:           30 * time.Minute,
		RequestedService:   "checkout",
		RequiredTraceKeys:  []string{"trace_id"},
		RequiredSpanKeys:   []string{"span_id"},
		MinCoveragePercent: 80,
		MaxLogSamples:      50,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if check.SampledLogs != 2 || check.TraceCoveragePercent != 0 {
		t.Fatalf("unexpected check: %+v", check)
	}
	if score.Earned != 0 || score.Status != "fail" {
		t.Fatalf("unexpected score: %+v", score)
	}
	if len(findings) < 2 {
		t.Fatalf("expected mismatch and coverage findings, got %+v", findings)
	}
}

func TestLogTraceAnalyzerResolverError(t *testing.T) {
	t.Parallel()

	analyzer := NewLogTraceAnalyzer(
		fakeLogSampler{samples: []LogSample{
			{Line: `{"service_name":"checkout","trace_id":"trace-1","span_id":"span-1"}`},
		}},
		fakeTraceResolver{err: errors.New("tempo unavailable")},
	)

	_, _, _, err := analyzer.Analyze(context.Background(), time.Unix(200, 0), LogTraceParams{
		Lookback:           30 * time.Minute,
		RequestedService:   "checkout",
		RequiredTraceKeys:  []string{"trace_id"},
		RequiredSpanKeys:   []string{"span_id"},
		MinCoveragePercent: 80,
		MaxLogSamples:      50,
	})
	if err == nil {
		t.Fatal("Analyze returned nil error")
	}
}

type fakeLogSampler struct {
	samples []LogSample
	err     error
}

func (f fakeLogSampler) SampleLogs(context.Context, time.Time, time.Time, int) ([]LogSample, error) {
	return f.samples, f.err
}

type fakeTraceResolver struct {
	resolved map[string]bool
	err      error
}

func (f fakeTraceResolver) TraceExists(_ context.Context, traceID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.resolved[traceID], nil
}
