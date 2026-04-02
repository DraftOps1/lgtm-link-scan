package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	scanreport "github.com/DraftOps1/lgtm-link-scan/internal/report"
	"github.com/spf13/cobra"
)

func newReportCmd() *cobra.Command {
	var input string
	var format string
	var output string

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Render a saved scan result into a human-friendly report",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReport(cmd.OutOrStdout(), input, format, output)
		},
	}

	cmd.Flags().StringVar(&input, "input", "", "Path to a JSON result file")
	cmd.Flags().StringVar(&format, "format", "markdown", "Output format: markdown or json")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Path to the rendered report")
	_ = cmd.MarkFlagRequired("input")

	return cmd
}

func runReport(out io.Writer, inputPath, format, outputPath string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read report input %q: %w", inputPath, err)
	}

	rendered, err := scanreport.Render(data, format)
	if err != nil {
		return err
	}

	if outputPath == "" {
		_, err := fmt.Fprintln(out, string(rendered))
		return err
	}

	if err := os.WriteFile(outputPath, append(rendered, '\n'), 0o644); err != nil {
		return fmt.Errorf("write rendered report %q: %w", outputPath, err)
	}
	_, err = fmt.Fprintf(out, "report: wrote %s (%s)\n", outputPath, strings.ToLower(strings.TrimSpace(format)))
	return err
}
