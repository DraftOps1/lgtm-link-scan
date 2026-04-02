package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DraftOps1/lgtm-link-scan/internal/clients"
	"github.com/DraftOps1/lgtm-link-scan/internal/config"
	"github.com/DraftOps1/lgtm-link-scan/internal/model"
)

func TestRunScanWritesJSONFile(t *testing.T) {
	t.Parallel()

	configPath := writeDoctorTestConfig(t, "http://loki.invalid", "http://mimir.invalid", "http://tempo.invalid")
	outputPath := filepath.Join(t.TempDir(), "result.json")

	var stdout bytes.Buffer
	err := runScanWithDependencies(&stdout, configPath, "", "", outputPath, 0, scanDependencies{
		newLokiClient: func(config.Config) (lokiScanClient, error) {
			return fakeLokiScanClient{entries: []clients.LokiLogEntry{
				{Timestamp: time.Unix(100, 0), Line: `{"service_name":"checkout","service_namespace":"store","deployment_environment":"demo","trace_id":"trace-1","span_id":"span-1"}`},
			}}, nil
		},
		newMimirClient: func(config.Config) (mimirScanClient, error) {
			return fakeMimirScanClient{
				series: []clients.MetricSeries{
					{Labels: map[string]string{"__name__": "demo_request_duration_seconds_bucket", "service_name": "checkout", "service_namespace": "store", "deployment_environment": "demo"}},
					{Labels: map[string]string{"__name__": "traces_span_metrics_calls_total", "service_name": "checkout", "service_namespace": "store", "deployment_environment": "demo"}},
				},
				exemplars: []clients.ExemplarSeries{
					{
						SeriesLabels: map[string]string{"__name__": "demo_request_duration_seconds_bucket"},
						Exemplars:    []clients.Exemplar{{Labels: map[string]string{"trace_id": "trace-1"}}},
					},
				},
			}, nil
		},
		newTempoClient: func(config.Config) (tempoScanClient, error) {
			return fakeTempoScanClient{
				resolved: map[string]bool{"trace-1": true},
				services: []string{"checkout"},
				traceAttributes: []map[string]string{{
					"service.name":           "checkout",
					"service.namespace":      "store",
					"deployment.environment": "demo",
				}},
			}, nil
		},
		now: func() time.Time { return time.Unix(200, 0).UTC() },
	})
	if err != nil {
		t.Fatalf("runScanWithDependencies: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var result model.ScanResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if result.Score.Percent != 100 || result.Score.Available != 100 {
		t.Fatalf("unexpected score: %+v", result.Score)
	}
	if !strings.Contains(stdout.String(), "scan: wrote") {
		t.Fatalf("stdout missing write confirmation: %s", stdout.String())
	}
}

func TestRunScanFailUnder(t *testing.T) {
	t.Parallel()

	configPath := writeDoctorTestConfig(t, "http://loki.invalid", "http://mimir.invalid", "http://tempo.invalid")

	err := runScanWithDependencies(io.Discard, configPath, "", "", "", 70, scanDependencies{
		newLokiClient: func(config.Config) (lokiScanClient, error) {
			return fakeLokiScanClient{entries: []clients.LokiLogEntry{
				{Timestamp: time.Unix(100, 0), Line: `{"service_name":"checkout"}`},
			}}, nil
		},
		newMimirClient: func(config.Config) (mimirScanClient, error) {
			return fakeMimirScanClient{
				series: []clients.MetricSeries{
					{Labels: map[string]string{"__name__": "demo_requests_total", "service_name": "checkout"}},
				},
			}, nil
		},
		newTempoClient: func(config.Config) (tempoScanClient, error) {
			return fakeTempoScanClient{services: []string{"checkout"}}, nil
		},
		now: func() time.Time { return time.Unix(200, 0).UTC() },
	})
	if err == nil {
		t.Fatal("runScanWithDependencies returned nil error")
	}
	if !strings.Contains(err.Error(), "fail-under") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type fakeLokiScanClient struct {
	entries []clients.LokiLogEntry
	err     error
}

func (f fakeLokiScanClient) SampleLogs(context.Context, time.Time, time.Time, int) ([]clients.LokiLogEntry, error) {
	return f.entries, f.err
}

type fakeMimirScanClient struct {
	series    []clients.MetricSeries
	seriesErr error
	exemplars []clients.ExemplarSeries
	exErr     error
}

func (f fakeMimirScanClient) MatchSeries(context.Context, time.Time, time.Time, string) ([]clients.MetricSeries, error) {
	return f.series, f.seriesErr
}

func (f fakeMimirScanClient) QueryExemplars(context.Context, time.Time, time.Time, string) ([]clients.ExemplarSeries, error) {
	return f.exemplars, f.exErr
}

type fakeTempoScanClient struct {
	resolved        map[string]bool
	err             error
	services        []string
	traceAttributes []map[string]string
}

func (f fakeTempoScanClient) TraceExists(_ context.Context, traceID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if f.resolved == nil {
		return false, nil
	}
	return f.resolved[traceID], nil
}

func (f fakeTempoScanClient) SearchServiceNames(context.Context, time.Time, time.Time, string, int) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.services, nil
}

func (f fakeTempoScanClient) SampleTraceResourceAttributes(context.Context, time.Time, time.Time, string, int) ([]map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.traceAttributes, nil
}

func TestRunScanPropagatesTempoError(t *testing.T) {
	t.Parallel()

	configPath := writeDoctorTestConfig(t, "http://loki.invalid", "http://mimir.invalid", "http://tempo.invalid")

	err := runScanWithDependencies(io.Discard, configPath, "", "", "", 0, scanDependencies{
		newLokiClient: func(config.Config) (lokiScanClient, error) {
			return fakeLokiScanClient{entries: []clients.LokiLogEntry{
				{Timestamp: time.Unix(100, 0), Line: `{"service_name":"checkout","service_namespace":"store","deployment_environment":"demo","trace_id":"trace-1","span_id":"span-1"}`},
			}}, nil
		},
		newMimirClient: func(config.Config) (mimirScanClient, error) {
			return fakeMimirScanClient{
				series: []clients.MetricSeries{
					{Labels: map[string]string{"__name__": "demo_request_duration_seconds_bucket", "service_name": "checkout", "service_namespace": "store", "deployment_environment": "demo"}},
					{Labels: map[string]string{"__name__": "traces_span_metrics_calls_total", "service_name": "checkout", "service_namespace": "store", "deployment_environment": "demo"}},
				},
				exemplars: []clients.ExemplarSeries{
					{
						SeriesLabels: map[string]string{"__name__": "demo_request_duration_seconds_bucket"},
						Exemplars:    []clients.Exemplar{{Labels: map[string]string{"trace_id": "trace-1"}}},
					},
				},
			}, nil
		},
		newTempoClient: func(config.Config) (tempoScanClient, error) {
			return fakeTempoScanClient{err: errors.New("tempo down")}, nil
		},
		now: func() time.Time { return time.Unix(200, 0).UTC() },
	})
	if err == nil {
		t.Fatal("runScanWithDependencies returned nil error")
	}
	if !strings.Contains(err.Error(), "tempo down") {
		t.Fatalf("unexpected error: %v", err)
	}
}
