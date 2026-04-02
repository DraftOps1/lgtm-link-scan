package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DraftOps1/lgtm-link-scan/internal/config"
)

func TestRunDoctorSuccess(t *testing.T) {
	t.Parallel()

	configPath := writeDoctorTestConfig(t, "http://loki.invalid", "http://mimir.invalid", "http://tempo.invalid")

	var output bytes.Buffer
	if err := runDoctorWithDependencies(&output, configPath, doctorDependencies{
		newLokiClient: func(config.Config) (lokiDoctorClient, error) {
			return fakeLokiDoctorClient{ready: "ready", streams: 1}, nil
		},
		newMimirClient: func(config.Config) (mimirDoctorClient, error) {
			return fakeMimirDoctorClient{ready: "ready", targets: 2}, nil
		},
		newTempoClient: func(config.Config) (tempoDoctorClient, error) {
			return fakeTempoDoctorClient{ready: "ready", traces: 1}, nil
		},
	}); err != nil {
		t.Fatalf("runDoctor: %v", err)
	}

	for _, want := range []string{
		"loki: ok",
		"mimir: ok",
		"tempo: ok",
		"status: doctor checks passed",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, output.String())
		}
	}
}

func TestRunDoctorFailureWhenNoTempoData(t *testing.T) {
	t.Parallel()

	configPath := writeDoctorTestConfig(t, "http://loki.invalid", "http://mimir.invalid", "http://tempo.invalid")

	var output bytes.Buffer
	err := runDoctorWithDependencies(&output, configPath, doctorDependencies{
		newLokiClient: func(config.Config) (lokiDoctorClient, error) {
			return fakeLokiDoctorClient{ready: "ready", streams: 1}, nil
		},
		newMimirClient: func(config.Config) (mimirDoctorClient, error) {
			return fakeMimirDoctorClient{ready: "ready", targets: 1}, nil
		},
		newTempoClient: func(config.Config) (tempoDoctorClient, error) {
			return fakeTempoDoctorClient{ready: "ready", traces: 0}, nil
		},
	})
	if err == nil {
		t.Fatalf("runDoctor returned nil error")
	}
	if !strings.Contains(output.String(), "status: doctor checks failed") {
		t.Fatalf("output missing failed status:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "tempo: error") {
		t.Fatalf("output missing tempo error:\n%s", output.String())
	}
}

func writeDoctorTestConfig(t *testing.T, lokiURL, mimirURL, tempoURL string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte("endpoints:\n" +
		"  loki: " + lokiURL + "\n" +
		"  mimir: " + mimirURL + "\n" +
		"  tempo: " + tempoURL + "\n\n" +
		"auth:\n" +
		"  bearer_token: \"\"\n\n" +
		"scan:\n" +
		"  lookback: 30m\n" +
		"  services:\n" +
		"    - checkout\n\n" +
		"rules:\n" +
		"  log_trace:\n" +
		"    required_trace_keys: [\"trace_id\"]\n" +
		"    min_coverage_percent: 80\n" +
		"  metric_trace:\n" +
		"    min_exemplar_coverage_percent: 50\n" +
		"  trace_metrics:\n" +
		"    required_metric_names: [\"calls\"]\n" +
		"  labels:\n" +
		"    required_shared_labels: [\"service.name\"]\n" +
		"  severity:\n" +
		"    critical: 40\n" +
		"    high: 60\n" +
		"    medium: 80\n")

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return path
}

type fakeLokiDoctorClient struct {
	ready      string
	readyErr   error
	streams    int
	streamsErr error
}

func (f fakeLokiDoctorClient) Ready(context.Context) (string, error) {
	return f.ready, f.readyErr
}

func (f fakeLokiDoctorClient) CountStreams(context.Context, time.Time, time.Time) (int, error) {
	return f.streams, f.streamsErr
}

type fakeMimirDoctorClient struct {
	ready      string
	readyErr   error
	targets    float64
	targetsErr error
}

func (f fakeMimirDoctorClient) Ready(context.Context) (string, error) {
	return f.ready, f.readyErr
}

func (f fakeMimirDoctorClient) ActiveTargetsInLookback(context.Context, string) (float64, error) {
	return f.targets, f.targetsErr
}

type fakeTempoDoctorClient struct {
	ready     string
	readyErr  error
	traces    int
	tracesErr error
}

func (f fakeTempoDoctorClient) Ready(context.Context) (string, error) {
	return f.ready, f.readyErr
}

func (f fakeTempoDoctorClient) SearchTraceQL(context.Context, time.Time, time.Time, string, int) (int, error) {
	return f.traces, f.tracesErr
}
