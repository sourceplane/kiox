package cmd

import "github.com/spf13/cobra"

func newRemoveCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <provider-or-alias>",
		Short: "Remove a provider from the current or selected workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemoveProviderCommand(cmd, root, args[0])
		},
	}
	return cmd
}