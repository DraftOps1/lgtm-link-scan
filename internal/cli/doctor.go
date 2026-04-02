package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/DraftOps1/lgtm-link-scan/internal/clients"
	"github.com/DraftOps1/lgtm-link-scan/internal/config"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Validate configuration and check backend connectivity",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDoctor(cmd.OutOrStdout(), configPath)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "lgtm-link-scan.yaml", "Path to the YAML config file")

	return cmd
}

type backendStatus struct {
	name    string
	message string
	err     error
}

type lokiDoctorClient interface {
	Ready(ctx context.Context) (string, error)
	CountStreams(ctx context.Context, start, end time.Time) (int, error)
}

type mimirDoctorClient interface {
	Ready(ctx context.Context) (string, error)
	ActiveTargetsInLookback(ctx context.Context, lookback string) (float64, error)
}

type tempoDoctorClient interface {
	Ready(ctx context.Context) (string, error)
	SearchTraceQL(ctx context.Context, start, end time.Time, query string, limit int) (int, error)
}

type doctorDependencies struct {
	newLokiClient  func(cfg config.Config) (lokiDoctorClient, error)
	newMimirClient func(cfg config.Config) (mimirDoctorClient, error)
	newTempoClient func(cfg config.Config) (tempoDoctorClient, error)
}

func runDoctor(out io.Writer, configPath string) error {
	return runDoctorWithDependencies(out, configPath, doctorDependencies{
		newLokiClient: func(cfg config.Config) (lokiDoctorClient, error) {
			return clients.NewLokiClient(clients.ClientConfig{
				BaseURL:     cfg.Endpoints.Loki,
				BearerToken: cfg.Auth.BearerToken,
			})
		},
		newMimirClient: func(cfg config.Config) (mimirDoctorClient, error) {
			return clients.NewMimirClient(clients.ClientConfig{
				BaseURL:     cfg.Endpoints.Mimir,
				BearerToken: cfg.Auth.BearerToken,
			})
		},
		newTempoClient: func(cfg config.Config) (tempoDoctorClient, error) {
			return clients.NewTempoClient(clients.ClientConfig{
				BaseURL:     cfg.Endpoints.Tempo,
				BearerToken: cfg.Auth.BearerToken,
			})
		},
	})
}

func runDoctorWithDependencies(out io.Writer, configPath string, deps doctorDependencies) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	if err := cfg.Validate(); err != nil {
		return err
	}

	lookback, err := time.ParseDuration(cfg.Scan.Lookback)
	if err != nil {
		return fmt.Errorf("parse lookback %q: %w", cfg.Scan.Lookback, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	statuses := []backendStatus{
		checkLoki(ctx, cfg, lookback, deps.newLokiClient),
		checkMimir(ctx, cfg, deps.newMimirClient),
		checkTempo(ctx, cfg, lookback, deps.newTempoClient),
	}

	services := "all services"
	if len(cfg.Scan.Services) > 0 {
		services = strings.Join(cfg.Scan.Services, ", ")
	}

	if _, err := fmt.Fprintf(
		out,
		"config: %s\nlookback: %s\nservices: %s\nloki endpoint: %s\nmimir endpoint: %s\ntempo endpoint: %s\n",
		configPath,
		cfg.Scan.Lookback,
		services,
		cfg.Endpoints.Loki,
		cfg.Endpoints.Mimir,
		cfg.Endpoints.Tempo,
	); err != nil {
		return err
	}

	var problems []error
	for _, status := range statuses {
		if _, err := fmt.Fprintf(out, "%s: %s\n", status.name, status.message); err != nil {
			return err
		}
		if status.err != nil {
			problems = append(problems, status.err)
		}
	}

	if len(problems) > 0 {
		if _, err := fmt.Fprintln(out, "status: doctor checks failed"); err != nil {
			return err
		}
		return errors.Join(problems...)
	}

	if _, err := fmt.Fprintln(out, "status: doctor checks passed"); err != nil {
		return err
	}

	return nil
}

func checkLoki(ctx context.Context, cfg config.Config, lookback time.Duration, newClient func(config.Config) (lokiDoctorClient, error)) backendStatus {
	client, err := newClient(cfg)
	if err != nil {
		return backendStatus{name: "loki", message: "error - invalid client configuration", err: err}
	}

	ready, err := client.Ready(ctx)
	if err != nil {
		return backendStatus{name: "loki", message: fmt.Sprintf("error - %v", err), err: err}
	}

	now := time.Now()
	streams, err := client.CountStreams(ctx, now.Add(-lookback), now)
	if err != nil {
		return backendStatus{name: "loki", message: fmt.Sprintf("error - ready=%q; %v", ready, err), err: err}
	}
	if streams == 0 {
		err := fmt.Errorf("loki query_range: no log streams found in last %s", cfg.Scan.Lookback)
		return backendStatus{name: "loki", message: fmt.Sprintf("error - ready=%q; no log streams found in last %s", ready, cfg.Scan.Lookback), err: err}
	}

	return backendStatus{
		name:    "loki",
		message: fmt.Sprintf("ok - ready=%q; sampled %d log stream(s) in last %s", ready, streams, cfg.Scan.Lookback),
	}
}

func checkMimir(ctx context.Context, cfg config.Config, newClient func(config.Config) (mimirDoctorClient, error)) backendStatus {
	client, err := newClient(cfg)
	if err != nil {
		return backendStatus{name: "mimir", message: "error - invalid client configuration", err: err}
	}

	ready, err := client.Ready(ctx)
	if err != nil {
		return backendStatus{name: "mimir", message: fmt.Sprintf("error - %v", err), err: err}
	}

	targets, err := client.ActiveTargetsInLookback(ctx, cfg.Scan.Lookback)
	if err != nil {
		return backendStatus{name: "mimir", message: fmt.Sprintf("error - ready=%q; %v", ready, err), err: err}
	}
	if targets == 0 {
		err := fmt.Errorf("mimir query: no active targets found in last %s", cfg.Scan.Lookback)
		return backendStatus{name: "mimir", message: fmt.Sprintf("error - ready=%q; no active targets found in last %s", ready, cfg.Scan.Lookback), err: err}
	}

	return backendStatus{
		name:    "mimir",
		message: fmt.Sprintf("ok - ready=%q; %.0f active target(s) in last %s", ready, targets, cfg.Scan.Lookback),
	}
}

func checkTempo(ctx context.Context, cfg config.Config, lookback time.Duration, newClient func(config.Config) (tempoDoctorClient, error)) backendStatus {
	client, err := newClient(cfg)
	if err != nil {
		return backendStatus{name: "tempo", message: "error - invalid client configuration", err: err}
	}

	ready, err := client.Ready(ctx)
	if err != nil {
		return backendStatus{name: "tempo", message: fmt.Sprintf("error - %v", err), err: err}
	}

	traceql := "{}"
	scope := "all traces"
	if len(cfg.Scan.Services) > 0 {
		traceql = fmt.Sprintf(`{ resource.service.name = %q }`, cfg.Scan.Services[0])
		scope = fmt.Sprintf("service=%s", cfg.Scan.Services[0])
	}

	now := time.Now()
	traces, err := client.SearchTraceQL(ctx, now.Add(-lookback), now, traceql, 1)
	if err != nil {
		return backendStatus{name: "tempo", message: fmt.Sprintf("error - ready=%q; %v", ready, err), err: err}
	}
	if traces == 0 {
		err := fmt.Errorf("tempo search: no traces found in last %s for %s", cfg.Scan.Lookback, scope)
		return backendStatus{name: "tempo", message: fmt.Sprintf("error - ready=%q; no traces found in last %s for %s", ready, cfg.Scan.Lookback, scope), err: err}
	}

	return backendStatus{
		name:    "tempo",
		message: fmt.Sprintf("ok - ready=%q; found %d trace result(s) in last %s for %s", ready, traces, cfg.Scan.Lookback, scope),
	}
}
