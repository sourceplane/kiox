package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/workspace"
)

func newStatusCommand(root *rootOptions) *cobra.Command {
	var verbose bool
	var short bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the current workspace, providers, tools, shims, and environment",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if verbose && short {
				return fmt.Errorf("status accepts only one of --verbose or --short")
			}
			globalHome, err := ensureGlobalHome(root.Home)
			if err != nil {
				return err
			}
			target, err := resolveSelectedWorkspaceTarget(root, globalHome)
			if err != nil {
				return err
			}
			if target == nil {
				scope, err := inspectDefaultScope(globalHome, true)
				if err != nil {
					return err
				}
				renderDefaultStatus(cmd.OutOrStdout(), scope, verbose, short)
				return nil
			}
			if err := requireReadyWorkspaceTarget(target); err != nil {
				return err
			}
			result, err := workspace.Sync(cmd.Context(), target.Root, target.Config, workspace.SyncOptions{Out: cmd.ErrOrStderr(), GlobalHome: globalHome})
			if err != nil {
				return err
			}
			shellEnv, err := workspace.BuildShellEnvironment(target.Root, result.Home, result.Aliases, workspace.ShellBuildOptions{Out: cmd.ErrOrStderr(), GlobalHome: globalHome})
			if err != nil {
				return err
			}
			providers, err := inspectProviderInventory(result.Home, true)
			if err != nil {
				return err
			}
			tools, err := inspectToolInventory(result.Home, providers)
			if err != nil {
				return err
			}
			renderWorkspaceStatus(cmd.OutOrStdout(), target, result.Home, shellEnv, providers, tools, verbose, short)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show detailed workspace status")
	cmd.Flags().BoolVarP(&short, "short", "s", false, "show a compact one-line status")
	return cmd
}

func renderWorkspaceStatus(w io.Writer, target *workspaceTarget, home string, shellEnv workspace.ShellEnvironment, providers []providerInventory, tools []toolInventory, verbose, short bool) {
	workspaceName := target.DisplayName()
	if workspaceName == "" {
		workspaceName = filepath.Base(target.Root)
	}
	path := displayRelativePath(target.Root)
	shims := shimState(target, providers)
	if short {
		writeLine(w, "%s | %d providers | shims %s", workspaceName, len(providers), shims)
		return
	}
	writeLine(w, "tinx workspace: %s", workspaceName)
	writeLine(w, "path: %s", path)
	writeLine(w, "shims: %s", shims)
	writeLine(w, "")
	writeLine(w, "providers:")
	renderStatusProviders(w, providers, false)
	writeLine(w, "")
	writeLine(w, "tools:")
	renderToolTable(w, tools)
	if !verbose {
		return
	}
	writeLine(w, "")
	writeLine(w, "details:")
	writeLine(w, "  home: %s", displayRelativePath(home))
	writeLine(w, "  shim dir: %s", displayRelativePath(shellEnv.ShimDir))
	writeLine(w, "  env file: %s", displayRelativePath(shellEnv.EnvFile))
	writeLine(w, "  path file: %s", displayRelativePath(shellEnv.PathFile))
	writeLine(w, "  providers dir: %s", displayRelativePath(filepath.Join(home, "providers")))
	if len(shellEnv.PathEntries) > 0 {
		entries := make([]string, 0, len(shellEnv.PathEntries))
		for _, entry := range shellEnv.PathEntries {
			entries = append(entries, displayRelativePath(entry))
		}
		writeLine(w, "  path entries: %s", strings.Join(entries, ", "))
	}
}

func renderDefaultStatus(w io.Writer, scope inventoryScope, verbose, short bool) {
	shims := "inactive"
	if short {
		writeLine(w, "none | %d providers | shims %s", len(scope.Providers), shims)
		return
	}
	writeLine(w, "tinx workspace: none")
	writeLine(w, "path: -")
	writeLine(w, "shims: %s", shims)
	writeLine(w, "")
	writeLine(w, "providers:")
	renderStatusProviders(w, scope.Providers, false)
	writeLine(w, "")
	writeLine(w, "tools:")
	renderToolTable(w, scope.Tools)
	if !verbose {
		return
	}
	writeLine(w, "")
	writeLine(w, "details:")
	writeLine(w, "  home: %s", displayRelativePath(scope.Home))
}

func renderStatusProviders(w io.Writer, providers []providerInventory, withHeader bool) {
	if len(providers) == 0 {
		writeLine(w, "  none")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if withHeader {
		fmt.Fprintln(tw, "  ALIAS\tPROVIDER\tVERSION\tSTATUS")
	}
	for _, provider := range providers {
		alias := displayAliases(provider.Aliases)
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n",
			alias,
			provider.Ref,
			fallbackDisplay(provider.Version),
			fallbackDisplay(provider.Status),
		)
	}
	_ = tw.Flush()
}

func displayRelativePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "-"
	}
	absPath, err := filepath.Abs(trimmed)
	if err != nil {
		return filepath.Clean(trimmed)
	}
	wd, err := os.Getwd()
	if err != nil {
		return absPath
	}
	rel, err := filepath.Rel(wd, absPath)
	if err != nil {
		return absPath
	}
	rel = filepath.Clean(rel)
	if rel == "." {
		return "."
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return rel
	}
	return "." + string(os.PathSeparator) + rel
}

func shimState(target *workspaceTarget, providers []providerInventory) string {
	if target == nil || len(providers) == 0 {
		return "inactive"
	}
	return "active"
}
