package cmd

import "github.com/spf13/cobra"

func newUpdateCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [provider-or-alias...]",
		Short: "Refresh tool metadata for the current or selected workspace",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdateProviderCommand(cmd, root, args)
		},
	}
	return cmd
}
