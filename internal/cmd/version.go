package cmd

import (
	"github.com/spf13/cobra"

	"github.com/sourceplane/kiox/pkg/version"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the kiox version",
		Run: func(cmd *cobra.Command, _ []string) {
			writeLine(cmd.OutOrStdout(), "kiox %s", version.String())
		},
	}
}
