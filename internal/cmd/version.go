package cmd

import (
	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/pkg/version"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the tinx version",
		Run: func(cmd *cobra.Command, _ []string) {
			writeLine(cmd.OutOrStdout(), "tinx %s", version.String())
		},
	}
}
