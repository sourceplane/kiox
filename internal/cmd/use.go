package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	cmdruntime "github.com/sourceplane/tinx/internal/runtime"
	"github.com/sourceplane/tinx/internal/state"
	"github.com/sourceplane/tinx/internal/workspace"
)

func newUseCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use <workspace> [-- command...]",
		Short: "Activate a workspace and optionally run a command inside it",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			beforeDash, afterDash := splitArgsAtDash(cmd, args)
			if len(beforeDash) == 0 {
				return fmt.Errorf("workspace is required")
			}
			globalHome, err := ensureGlobalHome(root.Home)
			if err != nil {
				return err
			}
			target, err := resolveWorkspaceTarget(beforeDash[0], globalHome)
			if err != nil {
				return err
			}
			result, err := workspace.Sync(cmd.Context(), target.Root, target.Config, workspace.SyncOptions{
				Out: cmd.ErrOrStderr(),
			})
			if err != nil {
				return err
			}
			if err := rememberWorkspaceTarget(globalHome, target); err != nil {
				return err
			}
			if err := state.SaveActiveWorkspace(globalHome, target.Root); err != nil {
				return err
			}
			if len(afterDash) == 0 {
				writeLine(cmd.OutOrStdout(), "active workspace: %s", target.Config.Name())
				writeLine(cmd.OutOrStdout(), "root: %s", target.Root)
				return nil
			}
			return cmdruntime.Dispatch(cmdruntime.DispatchOptions{
				Home:       result.Home,
				WorkingDir: mustGetwd(),
				Commands:   providerCommandsFromAliases(result.Aliases),
				Command:    afterDash,
				Stdout:     cmd.OutOrStdout(),
				Stderr:     cmd.ErrOrStderr(),
				Stdin:      os.Stdin,
			})
		},
	}
	return cmd
}
