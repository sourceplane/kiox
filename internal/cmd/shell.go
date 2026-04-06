package cmd

import "github.com/spf13/cobra"

func newShellCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Launch an interactive workspace shell",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceCommand(cmd, root, nil)
		},
	}
	return cmd
}