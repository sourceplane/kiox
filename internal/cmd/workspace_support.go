package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	cmdruntime "github.com/sourceplane/tinx/internal/runtime"
	"github.com/sourceplane/tinx/internal/state"
	"github.com/sourceplane/tinx/internal/workspace"
)

type workspaceTarget struct {
	Root       string
	ConfigPath string
	Config     workspace.Config
}

func executeCLI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	parsedRoot, strippedArgs, err := extractRootArgs(args)
	if err != nil {
		return err
	}
	if len(strippedArgs) > 0 && strippedArgs[0] == "--" {
		fallback := &cobra.Command{}
		fallback.SetOut(stdout)
		fallback.SetErr(stderr)
		return runWorkspaceCommand(fallback, &parsedRoot, strippedArgs[1:])
	}
	root := newRootCommand(&parsedRoot)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if shouldTreatAsRemovedDirectExecution(root, strippedArgs) {
		return directExecutionRemovedError(strippedArgs)
	}
	root.SetArgs(strippedArgs)
	return root.ExecuteContext(ctx)
}

func shouldTreatAsRemovedDirectExecution(root *cobra.Command, args []string) bool {
	if len(args) == 0 {
		return false
	}
	if strings.HasPrefix(args[0], "-") {
		return false
	}
	_, _, err := root.Find(args)
	return err != nil
}

func ensureGlobalHome(override string) (string, error) {
	home, err := state.ResolveHome(override)
	if err != nil {
		return "", err
	}
	if err := state.EnsureHome(home); err != nil {
		return "", err
	}
	return home, nil
}

func ensureHome(override string) (string, error) {
	if override != "" || os.Getenv("TINX_HOME") != "" {
		return ensureGlobalHome(override)
	}
	if discovery, err := workspace.Discover(mustGetwd()); err == nil && discovery != nil {
		home := workspace.Home(discovery.Root)
		if err := state.EnsureHome(home); err != nil {
			return "", err
		}
		return home, nil
	} else if err != nil {
		return "", err
	}
	return ensureGlobalHome(override)
}

func ensureWorkspaceHome(root string) (string, error) {
	home := workspace.Home(root)
	if err := state.EnsureHome(home); err != nil {
		return "", err
	}
	return home, nil
}

func splitArgsAtDash(cmd *cobra.Command, args []string) ([]string, []string) {
	if position := cmd.ArgsLenAtDash(); position >= 0 {
		return args[:position], args[position:]
	}
	for index, arg := range args {
		if arg == "--" {
			return args[:index], args[index+1:]
		}
	}
	return args, nil
}

func runWorkspaceCommand(cmd *cobra.Command, root *rootOptions, command []string) error {
	globalHome, err := ensureGlobalHome(root.Home)
	if err != nil {
		return err
	}
	target, err := resolveSelectedWorkspaceTarget(root, globalHome)
	if err != nil {
		return err
	}
	if target == nil {
		return fmt.Errorf("no active workspace; run tinx workspace activate <workspace> first or execute inside a workspace")
	}
	result, err := workspace.Sync(cmd.Context(), target.Root, target.Config, workspace.SyncOptions{
		Out: cmd.ErrOrStderr(),
	})
	if err != nil {
		return err
	}
	shellEnv, err := workspace.BuildShellEnvironment(target.Root, result.Home, result.Aliases, workspace.ShellBuildOptions{
		Out: cmd.ErrOrStderr(),
	})
	if err != nil {
		return err
	}
	runtimeOpts := cmdruntime.ShellOptions{
		WorkingDir:  workspaceWorkingDir(target.Root),
		Env:         shellEnv.Env,
		PathEntries: shellEnv.PathEntries,
		Stdout:      cmd.OutOrStdout(),
		Stderr:      cmd.ErrOrStderr(),
		Stdin:       os.Stdin,
	}
	if len(command) == 0 {
		return cmdruntime.RunInteractiveShell(runtimeOpts)
	}
	return cmdruntime.RunCommand(cmdruntime.ShellCommandOptions{
		ShellOptions: runtimeOpts,
		Command:      command,
	})
}

func resolveSelectedWorkspaceTarget(root *rootOptions, globalHome string) (*workspaceTarget, error) {
	if root != nil {
		if reference := strings.TrimSpace(root.Workspace); reference != "" {
			return resolveWorkspaceTarget(reference, globalHome)
		}
	}
	return resolveCurrentWorkspaceTarget(globalHome)
}

func resolveCurrentWorkspaceTarget(globalHome string) (*workspaceTarget, error) {
	if discovery, err := workspace.Discover(mustGetwd()); err == nil && discovery != nil {
		return &workspaceTarget{Root: discovery.Root, ConfigPath: discovery.ConfigPath, Config: discovery.Config}, nil
	} else if err != nil {
		return nil, err
	}
	activeRoot, err := state.LoadActiveWorkspace(globalHome)
	if err != nil {
		return nil, err
	}
	if activeRoot == "" {
		return nil, nil
	}
	return resolveWorkspaceTarget(activeRoot, globalHome)
}

func resolveWorkspaceTarget(reference, globalHome string) (*workspaceTarget, error) {
	trimmed := strings.TrimSpace(reference)
	if trimmed == "" {
		return nil, fmt.Errorf("workspace is required")
	}
	if target, err := workspaceTargetFromPath(trimmed); err != nil {
		return nil, err
	} else if target != nil {
		return target, nil
	}
	known, err := state.LoadWorkspaces(globalHome)
	if err != nil {
		return nil, err
	}
	if path := strings.TrimSpace(known[trimmed]); path != "" {
		return workspaceTargetFromPath(path)
	}
	names := make([]string, 0, len(known))
	for name := range known {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("workspace %q was not found", trimmed)
	}
	return nil, fmt.Errorf("workspace %q was not found; known workspaces: %s", trimmed, strings.Join(names, ", "))
}

func workspaceTargetFromPath(path string) (*workspaceTarget, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace path: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat workspace path: %w", err)
	}
	if info.IsDir() {
		discovery, err := workspace.Discover(absPath)
		if err != nil {
			return nil, err
		}
		if discovery == nil {
			return nil, fmt.Errorf("no workspace manifest found under %s", absPath)
		}
		return &workspaceTarget{Root: discovery.Root, ConfigPath: discovery.ConfigPath, Config: discovery.Config}, nil
	}
	config, err := workspace.Load(absPath)
	if err != nil {
		return nil, err
	}
	return &workspaceTarget{Root: filepath.Dir(absPath), ConfigPath: absPath, Config: config}, nil
}

func rememberWorkspaceTarget(globalHome string, target *workspaceTarget) error {
	if target == nil {
		return nil
	}
	if err := state.RememberWorkspace(globalHome, target.Config.Name(), target.Root); err != nil {
		return err
	}
	if base := filepath.Base(target.Root); base != "" && base != "." && base != target.Config.Name() {
		if err := state.RememberWorkspace(globalHome, base, target.Root); err != nil {
			return err
		}
	}
	return nil
}

func workspaceWorkingDir(root string) string {
	workingDir := mustGetwd()
	if root == "" {
		return workingDir
	}
	relativePath, err := filepath.Rel(root, workingDir)
	if err == nil && relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(os.PathSeparator)) {
		return workingDir
	}
	return root
}

func looksLikeManifestOrScript(name string) bool {
	lower := strings.ToLower(name)
	for _, suffix := range []string{".yaml", ".yml", ".js", ".mjs", ".cjs"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}
