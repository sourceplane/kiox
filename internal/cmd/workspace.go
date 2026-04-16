package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/state"
	"github.com/sourceplane/tinx/internal/workspace"
)

func newWorkspaceCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workspace",
		Aliases: []string{"ws", "workspaces"},
		Short:   "Manage workspaces",
	}
	cmd.AddCommand(newWorkspaceListCommand(root))
	cmd.AddCommand(newWorkspaceCreateCommand(root))
	cmd.AddCommand(newWorkspaceUseCommand(root))
	cmd.AddCommand(newWorkspaceCurrentCommand(root))
	cmd.AddCommand(newWorkspaceDeleteCommand(root))
	cmd.AddCommand(newWorkspaceActivateCommand(root))
	return cmd
}

func newWorkspaceListCommand(root *rootOptions) *cobra.Command {
	var short bool
	var readyOnly bool
	var missingOnly bool
	var activeOnly bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List known workspaces",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if readyOnly && missingOnly {
				return fmt.Errorf("workspace list accepts only one of --ready or --missing")
			}
			globalHome, err := ensureGlobalHome(root.Home)
			if err != nil {
				return err
			}
			scopes, err := listWorkspaceScopes(globalHome)
			if err != nil {
				return err
			}
			statusFilter := ""
			switch {
			case readyOnly:
				statusFilter = "ready"
			case missingOnly:
				statusFilter = "missing"
			}
			activeName := activeWorkspaceScopeName(scopes)
			scopes = filterWorkspaceScopes(scopes, statusFilter, activeOnly)
			renderWorkspaceScopes(cmd.OutOrStdout(), scopes, workspaceListOptions{
				Short:      short,
				ActiveName: activeName,
			})
			return nil
		},
	}
	cmd.Flags().BoolVarP(&short, "short", "s", false, "show only workspace names")
	cmd.Flags().BoolVar(&readyOnly, "ready", false, "show only ready workspaces")
	cmd.Flags().BoolVar(&missingOnly, "missing", false, "show only missing workspaces")
	cmd.Flags().BoolVar(&activeOnly, "active", false, "show only the active workspace")
	return cmd
}

func newWorkspaceCreateCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:                "create [workspace-or-config] [-p <provider-source> [as <alias>]]...",
		Short:              "Create or materialize a workspace",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInitCommand(cmd, root, args, ".")
		},
	}
	return cmd
}

func newWorkspaceUseCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use <workspace> [-- command...]",
		Short: "Select a workspace and optionally run a command inside its shell",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			beforeDash, afterDash := splitArgsAtDash(cmd, args)
			if len(beforeDash) == 0 {
				return fmt.Errorf("workspace is required")
			}
			return useWorkspace(cmd, root, beforeDash[0], afterDash)
		},
	}
	return cmd
}

func newWorkspaceActivateCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:        "activate <workspace> [-- command...]",
		Short:      "Deprecated: use a workspace",
		Hidden:     true,
		Deprecated: "use 'tinx workspace use <workspace>' instead",
		Args:       cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			beforeDash, afterDash := splitArgsAtDash(cmd, args)
			if len(beforeDash) == 0 {
				return fmt.Errorf("workspace is required")
			}
			return useWorkspace(cmd, root, beforeDash[0], afterDash)
		},
	}
	return cmd
}

func newWorkspaceCurrentCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "current",
		Short: "Show the current workspace",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			globalHome, err := ensureGlobalHome(root.Home)
			if err != nil {
				return err
			}
			target, err := resolveSelectedWorkspaceTarget(root, globalHome)
			if err != nil {
				return err
			}
			if target == nil {
				writeLine(cmd.OutOrStdout(), "workspace: none")
				return nil
			}
			writeLine(cmd.OutOrStdout(), "workspace: %s", target.DisplayName())
			writeLine(cmd.OutOrStdout(), "root: %s", displayInventoryPath(target.Root))
			if target.IsMissing() {
				writeLine(cmd.OutOrStdout(), "status: missing")
				return nil
			}
			writeLine(cmd.OutOrStdout(), "home: %s", displayInventoryPath(workspace.Home(target.Root)))
			return nil
		},
	}
	return cmd
}

func newWorkspaceDeleteCommand(root *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <workspace>",
		Short: "Delete workspace runtime state and unregister it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			globalHome, err := ensureGlobalHome(root.Home)
			if err != nil {
				return err
			}
			target, err := resolveWorkspaceTarget(args[0], globalHome)
			if err != nil {
				return err
			}
			if err := state.ForgetWorkspace(globalHome, target.Root); err != nil {
				return err
			}
			if target.IsMissing() {
				writeLine(cmd.OutOrStdout(), "unregistered missing workspace %s", target.DisplayName())
				writeLine(cmd.OutOrStdout(), "root: %s", displayInventoryPath(target.Root))
				return nil
			}
			for _, path := range []string{
				workspace.Home(target.Root),
				workspace.LockPath(target.Root),
				workspace.ManifestPath(target.Root),
			} {
				if err := os.RemoveAll(path); err != nil {
					return fmt.Errorf("delete workspace path %s: %w", path, err)
				}
			}
			writeLine(cmd.OutOrStdout(), "deleted workspace %s", target.DisplayName())
			writeLine(cmd.OutOrStdout(), "root: %s", displayInventoryPath(target.Root))
			if entries, err := os.ReadDir(target.Root); err == nil && len(entries) == 0 {
				_ = os.Remove(target.Root)
			}
			return nil
		},
	}
	return cmd
}

func useWorkspace(cmd *cobra.Command, root *rootOptions, reference string, command []string) error {
	globalHome, err := ensureGlobalHome(root.Home)
	if err != nil {
		return err
	}
	target, err := resolveWorkspaceTarget(reference, globalHome)
	if err != nil {
		return err
	}
	if err := requireReadyWorkspaceTarget(target); err != nil {
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
	if err := state.SaveActiveWorkspace(globalHome, target.Root); err != nil {
		return err
	}
	if len(command) == 0 {
		writeLine(cmd.OutOrStdout(), "active workspace: %s", target.DisplayName())
		writeLine(cmd.OutOrStdout(), "root: %s", displayInventoryPath(target.Root))
		return nil
	}
	shellEnv, err := workspace.BuildShellEnvironment(target.Root, result.Home, result.Aliases, workspace.ShellBuildOptions{
		Out:        cmd.ErrOrStderr(),
		GlobalHome: globalHome,
	})
	if err != nil {
		return err
	}
	return runPreparedWorkspaceCommand(cmd, target.Root, result, shellEnv, command)
}
