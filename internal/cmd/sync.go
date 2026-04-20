package cmd

import (
	"github.com/spf13/cobra"

	"github.com/sourceplane/kiox/internal/workspace"
)

func newSyncCommand(root *rootOptions) *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Reconcile workspace state from kiox.yaml",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSyncCommand(cmd, root, verbose)
		},
	}
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show detailed provider sync progress")
	return cmd
}

func runSyncCommand(cmd *cobra.Command, root *rootOptions, verbose bool) error {
	globalHome, target, err := resolveRequiredWorkspaceTarget(cmd, root)
	if err != nil {
		return err
	}
	_, err = workspace.Sync(cmd.Context(), target.Root, target.Config, workspace.SyncOptions{
		Out:        cmd.ErrOrStderr(),
		GlobalHome: globalHome,
		Verbose:    verbose,
	})
	if err != nil {
		return err
	}
	if err := rememberWorkspaceTarget(globalHome, target); err != nil {
		return err
	}
	writeLine(cmd.OutOrStdout(), "synced workspace %s", target.DisplayName())
	writeLine(cmd.OutOrStdout(), "manifest: %s", displayWorkspaceSummaryFilePath(workspace.ManifestPath(target.Root)))
	writeLine(cmd.OutOrStdout(), "home: %s", displayWorkspaceSummaryDirPath(target.Root))
	return nil
}
