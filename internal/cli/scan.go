package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newScanCmd() *cobra.Command {
	var configPath string
	var service string
	var lookback string
	var output string
	var failUnder int

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Run a link quality scan",
		RunE: func(_ *cobra.Command, _ []string) error {
			_ = configPath
			_ = service
			_ = lookback
			_ = output
			_ = failUnder
			return errors.New("scan is not implemented yet; use docs/demo.md to bootstrap the local stack first")
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "lgtm-link-scan.yaml", "Path to the YAML config file")
	cmd.Flags().StringVar(&service, "service", "", "Only scan a single service")
	cmd.Flags().StringVar(&lookback, "lookback", "", "Override the configured lookback window")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Write JSON results to a file")
	cmd.Flags().IntVar(&failUnder, "fail-under", 0, "Exit non-zero when the overall score is below this threshold")

	return cmd
}
