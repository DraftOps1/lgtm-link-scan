package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newReportCmd() *cobra.Command {
	var input string
	var format string
	var output string

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Render a saved scan result into a human-friendly report",
		RunE: func(_ *cobra.Command, _ []string) error {
			_ = input
			_ = format
			_ = output
			return errors.New("report is not implemented yet")
		},
	}

	cmd.Flags().StringVar(&input, "input", "", "Path to a JSON result file")
	cmd.Flags().StringVar(&format, "format", "markdown", "Output format")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Path to the rendered report")
	_ = cmd.MarkFlagRequired("input")

	return cmd
}
