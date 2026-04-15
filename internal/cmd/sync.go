package cmd

import (
	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/workspace"
)

func newSyncCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Reconcile workspace state from tinx.yaml",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSyncCommand(cmd, root)
		},
	}
	return cmd
}

func runSyncCommand(cmd *cobra.Command, root *rootOptions) error {
	globalHome, target, err := resolveRequiredWorkspaceTarget(cmd, root)
	if err != nil {
		return err
	}
	result, err := workspace.Sync(cmd.Context(), target.Root, target.Config, workspace.SyncOptions{
		Out:        cmd.ErrOrStderr(),
		GlobalHome: globalHome,
	})
	if err != nil {
		return err
	}
	if err := rememberWorkspaceTarget(globalHome, target); err != nil {
		return err
	}
	writeLine(cmd.OutOrStdout(), "synced workspace %s", target.DisplayName())
	writeLine(cmd.OutOrStdout(), "manifest: %s", workspace.ManifestPath(target.Root))
	writeLine(cmd.OutOrStdout(), "home: %s", result.Home)
	return nil
}