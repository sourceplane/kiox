package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	cmdruntime "github.com/sourceplane/tinx/internal/runtime"
	"github.com/sourceplane/tinx/internal/state"
	"github.com/sourceplane/tinx/internal/workspace"
)

func newWorkspaceCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Manage workspaces",
	}
	cmd.AddCommand(newWorkspaceActivateCommand(root))
	return cmd
}

func newWorkspaceActivateCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "activate <workspace> [-- command...]",
		Short: "Activate a workspace and optionally run a command inside its shell",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			beforeDash, afterDash := splitArgsAtDash(cmd, args)
			if len(beforeDash) == 0 {
				return fmt.Errorf("workspace is required")
			}
			return activateWorkspace(cmd, root, beforeDash[0], afterDash)
		},
	}
	return cmd
}

func activateWorkspace(cmd *cobra.Command, root *rootOptions, reference string, command []string) error {
	globalHome, err := ensureGlobalHome(root.Home)
	if err != nil {
		return err
	}
	target, err := resolveWorkspaceTarget(reference, globalHome)
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
	if len(command) == 0 {
		writeLine(cmd.OutOrStdout(), "active workspace: %s", target.Config.Name())
		writeLine(cmd.OutOrStdout(), "root: %s", target.Root)
		return nil
	}
	shellEnv, err := workspace.BuildShellEnvironment(target.Root, result.Home, result.Aliases, workspace.ShellBuildOptions{
		Out: cmd.ErrOrStderr(),
	})
	if err != nil {
		return err
	}
	return cmdruntime.RunCommand(cmdruntime.ShellCommandOptions{
		ShellOptions: cmdruntime.ShellOptions{
			WorkingDir:  workspaceWorkingDir(target.Root),
			Env:         shellEnv.Env,
			PathEntries: shellEnv.PathEntries,
			Stdout:      cmd.OutOrStdout(),
			Stderr:      cmd.ErrOrStderr(),
			Stdin:       os.Stdin,
		},
		Command: command,
	})
}
