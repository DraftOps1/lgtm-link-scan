package cli

import "github.com/spf13/cobra"

func Execute() error {
	return newRootCmd().Execute()
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lgtm-link-scan",
		Short: "Check cross-signal link quality across logs, metrics, and traces",
	}

	cmd.AddCommand(
		newDoctorCmd(),
		newScanCmd(),
		newReportCmd(),
		newDiffCmd(),
	)

	return cmd
}
