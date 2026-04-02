package cli

import (
	"fmt"
	"strings"

	"github.com/DraftOps1/lgtm-link-scan/internal/config"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Validate configuration and print the configured backend endpoints",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}

			if err := cfg.Validate(); err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			services := "all services"
			if len(cfg.Scan.Services) > 0 {
				services = strings.Join(cfg.Scan.Services, ", ")
			}

			_, err = fmt.Fprintf(
				out,
				"config: %s\nloki: %s\nmimir: %s\ntempo: %s\nlookback: %s\nservices: %s\nstatus: config validation passed\n",
				configPath,
				cfg.Endpoints.Loki,
				cfg.Endpoints.Mimir,
				cfg.Endpoints.Tempo,
				cfg.Scan.Lookback,
				services,
			)
			return err
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "lgtm-link-scan.yaml", "Path to the YAML config file")

	return cmd
}
