package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newDiffCmd() *cobra.Command {
	var before string
	var after string

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compare two saved scan results",
		RunE: func(_ *cobra.Command, _ []string) error {
			_ = before
			_ = after
			return errors.New("diff is not implemented yet")
		},
	}

	cmd.Flags().StringVar(&before, "before", "", "Path to the older JSON result")
	cmd.Flags().StringVar(&after, "after", "", "Path to the newer JSON result")
	_ = cmd.MarkFlagRequired("before")
	_ = cmd.MarkFlagRequired("after")

	return cmd
}
