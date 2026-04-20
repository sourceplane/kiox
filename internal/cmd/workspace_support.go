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

	cmdruntime "github.com/sourceplane/kiox/internal/runtime"
	"github.com/sourceplane/kiox/internal/state"
	"github.com/sourceplane/kiox/internal/workspace"
)

type workspaceTarget struct {
	Root       string
	ConfigPath string
	Config     workspace.Config
	Name       string
	Status     string
	Detail     string
}

func (target *workspaceTarget) DisplayName() string {
	if target == nil {
		return ""
	}
	if name := strings.TrimSpace(target.Config.Name()); name != "" {
		return name
	}
	if name := strings.TrimSpace(target.Name); name != "" {
		return name
	}
	if root := strings.TrimSpace(target.Root); root != "" {
		return filepath.Base(root)
	}
	return ""
}

func (target *workspaceTarget) DeleteReference() string {
	if target == nil {
		return ""
	}
	if name := strings.TrimSpace(target.Name); name != "" {
		return name
	}
	return target.Root
}

func (target *workspaceTarget) IsMissing() bool {
	return target != nil && strings.EqualFold(target.Status, "missing")
}

func (target *workspaceTarget) MissingError() error {
	if !target.IsMissing() {
		return nil
	}
	name := target.DisplayName()
	if name == "" {
		name = target.Root
	}
	return fmt.Errorf("workspace %q is missing: root %s no longer exists; run kiox workspace delete %q to unregister it", name, displayInventoryPath(target.Root), target.DeleteReference())
}

func displayWorkspaceSummaryFilePath(path string) string {
	return displayWorkspaceSummaryPath(path, false)
}

func displayWorkspaceSummaryDirPath(path string) string {
	return displayWorkspaceSummaryPath(path, true)
}

func displayWorkspaceSummaryPath(path string, directory bool) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "-"
	}
	if directory {
		absPath, absErr := filepath.Abs(trimmed)
		cwd, cwdErr := os.Getwd()
		if absErr == nil && cwdErr == nil {
			absCWD, absCWDErr := filepath.Abs(cwd)
			if absCWDErr == nil && filepath.Clean(absPath) == filepath.Clean(absCWD) {
				base := filepath.Base(absPath)
				if base == "." || base == string(os.PathSeparator) || base == "" {
					return "." + string(os.PathSeparator)
				}
				return base + string(os.PathSeparator)
			}
		}
	}
	display := displayInventoryPath(trimmed)
	prefix := "." + string(os.PathSeparator)
	display = strings.TrimPrefix(display, prefix)
	if !directory {
		return display
	}
	if display == "." {
		base := filepath.Base(filepath.Clean(trimmed))
		if base == "." || base == string(os.PathSeparator) || base == "" {
			return "." + string(os.PathSeparator)
		}
		return base + string(os.PathSeparator)
	}
	if display != "-" && !strings.HasSuffix(display, string(os.PathSeparator)) {
		display += string(os.PathSeparator)
	}
	return display
}

func executeCLI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	commandCtx := defaultCommandContext(ctx)
	parsedRoot, strippedArgs, err := extractRootArgs(args)
	if err != nil {
		return err
	}
	if len(strippedArgs) > 0 && strippedArgs[0] == "--" {
		fallback := &cobra.Command{}
		fallback.SetContext(commandCtx)
		fallback.SetOut(stdout)
		fallback.SetErr(stderr)
		return runWorkspaceCommand(fallback, &parsedRoot, strippedArgs[1:])
	}
	root := newRootCommand(&parsedRoot)
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(strippedArgs)
	return root.ExecuteContext(commandCtx)
}

func defaultCommandContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
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
	if override != "" || os.Getenv("KIOX_HOME") != "" {
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
	commandCtx := defaultCommandContext(cmd.Context())
	globalHome, err := ensureGlobalHome(root.Home)
	if err != nil {
		return err
	}
	target, err := resolveSelectedWorkspaceTarget(root, globalHome)
	if err != nil {
		return err
	}
	if target == nil {
		return fmt.Errorf("no active workspace; run kiox workspace use <workspace> first or execute inside a workspace")
	}
	if err := requireReadyWorkspaceTarget(target); err != nil {
		return err
	}
	result, err := prepareWorkspaceState(commandCtx, cmd.ErrOrStderr(), globalHome, target)
	if err != nil {
		return err
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

func prepareWorkspaceState(ctx context.Context, out io.Writer, globalHome string, target *workspaceTarget) (workspace.SyncResult, error) {
	if target == nil {
		return workspace.SyncResult{}, fmt.Errorf("workspace target is required")
	}
	if prepared, ok, err := workspace.LoadPreparedState(target.Root, target.Config); err != nil {
		return workspace.SyncResult{}, err
	} else if ok {
		return prepared, nil
	}
	return workspace.Sync(ctx, target.Root, target.Config, workspace.SyncOptions{
		Out:        out,
		GlobalHome: globalHome,
	})
}

func runPreparedWorkspaceCommand(cmd *cobra.Command, root string, result workspace.SyncResult, shellEnv workspace.ShellEnvironment, command []string) error {
	runtimeOpts := cmdruntime.ShellOptions{
		WorkingDir:  workspaceWorkingDir(root),
		Env:         shellEnv.Env,
		PathEntries: shellEnv.PathEntries,
		Stdout:      cmd.OutOrStdout(),
		Stderr:      cmd.ErrOrStderr(),
		Stdin:       os.Stdin,
	}
	if len(command) == 0 {
		return cmdruntime.RunInteractiveShell(runtimeOpts)
	}
	if shellTarget, ok := shellEnv.Targets[command[0]]; ok {
		providerKey := strings.TrimSpace(result.Aliases[shellTarget.Alias])
		if providerKey == "" {
			return fmt.Errorf("workspace alias %q is not installed", shellTarget.Alias)
		}
		return runWorkspaceToolCommand(cmd, root, result.Home, providerKey, shellTarget.Alias, shellTarget.Tool, command[1:], shellEnv.Env, shellEnv.PathEntries)
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
		return &workspaceTarget{Root: discovery.Root, ConfigPath: discovery.ConfigPath, Config: discovery.Config, Name: discovery.Config.Name(), Status: "ready"}, nil
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
	known, err := state.LoadWorkspaces(globalHome)
	if err != nil {
		return nil, err
	}
	names := workspaceNamesForRoot(known, activeRoot)
	if len(names) > 0 {
		return workspaceTargetFromRegisteredPath(activeRoot, names[0])
	}
	return workspaceTargetFromRegisteredPath(activeRoot, "")
}

func resolveWorkspaceTarget(reference, globalHome string) (*workspaceTarget, error) {
	trimmed := strings.TrimSpace(reference)
	if trimmed == "" {
		return nil, fmt.Errorf("workspace is required")
	}
	known, err := state.LoadWorkspaces(globalHome)
	if err != nil {
		return nil, err
	}
	if target, err := workspaceTargetFromPath(trimmed); err != nil {
		return nil, err
	} else if target != nil {
		return target, nil
	}
	if path := strings.TrimSpace(known[trimmed]); path != "" {
		return workspaceTargetFromRegisteredPath(path, trimmed)
	}
	if looksLikeWorkspacePath(trimmed) {
		return nil, fmt.Errorf("workspace path %q does not exist", trimmed)
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
		return &workspaceTarget{Root: discovery.Root, ConfigPath: discovery.ConfigPath, Config: discovery.Config, Name: discovery.Config.Name(), Status: "ready"}, nil
	}
	config, err := workspace.Load(absPath)
	if err != nil {
		return nil, err
	}
	return &workspaceTarget{Root: filepath.Dir(absPath), ConfigPath: absPath, Config: config, Name: config.Name(), Status: "ready"}, nil
}

func workspaceTargetFromRegisteredPath(path, name string) (*workspaceTarget, error) {
	target, err := workspaceTargetFromPath(path)
	if err != nil {
		return nil, err
	}
	if target != nil {
		if target.Name == "" {
			target.Name = strings.TrimSpace(name)
		}
		if target.Status == "" {
			target.Status = "ready"
		}
		return target, nil
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace path: %w", err)
	}
	return &workspaceTarget{
		Root:   absPath,
		Name:   strings.TrimSpace(name),
		Status: "missing",
		Detail: "workspace root does not exist",
	}, nil
}

func rememberWorkspaceTarget(globalHome string, target *workspaceTarget) error {
	if target == nil {
		return nil
	}
	name := target.DisplayName()
	if err := state.RememberWorkspace(globalHome, name, target.Root); err != nil {
		return err
	}
	if base := filepath.Base(target.Root); base != "" && base != "." && base != name {
		if err := state.RememberWorkspace(globalHome, base, target.Root); err != nil {
			return err
		}
	}
	return nil
}

func requireReadyWorkspaceTarget(target *workspaceTarget) error {
	if target == nil {
		return nil
	}
	if target.IsMissing() {
		return target.MissingError()
	}
	return nil
}

func workspaceNamesForRoot(known map[string]string, root string) []string {
	normalizedRoot := normalizeInventoryPath(root)
	if normalizedRoot == "" {
		return nil
	}
	names := make([]string, 0, 1)
	for name, knownRoot := range known {
		if normalizeInventoryPath(knownRoot) == normalizedRoot {
			names = append(names, strings.TrimSpace(name))
		}
	}
	sort.Strings(names)
	return names
}

func looksLikeWorkspacePath(reference string) bool {
	trimmed := strings.TrimSpace(reference)
	if trimmed == "" {
		return false
	}
	if trimmed == "." || trimmed == ".." {
		return true
	}
	return strings.Contains(trimmed, string(os.PathSeparator))
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

func loadWorkspaceConfigAtRootIfPresent(root string) (workspace.Config, string, bool, error) {
	for _, name := range workspace.ManifestNames {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return workspace.Config{}, "", false, fmt.Errorf("stat workspace manifest: %w", err)
		}
		config, err := workspace.Load(path)
		if err != nil {
			return workspace.Config{}, "", false, err
		}
		return config, path, true, nil
	}
	return workspace.Config{}, "", false, nil
}

func applyWorkspaceConfigChange(ctx context.Context, out io.Writer, globalHome string, target *workspaceTarget, desired workspace.Config, verbose bool) (workspace.SyncResult, string, error) {
	if target == nil {
		return workspace.SyncResult{}, "", fmt.Errorf("workspace target is required")
	}
	manifestPath := workspace.ManifestPath(target.Root)
	hadExistingConfig := workspaceConfigExists(target.ConfigPath)
	previousConfig := target.Config
	result, err := workspace.Sync(ctx, target.Root, desired, workspace.SyncOptions{
		Out:        out,
		GlobalHome: globalHome,
		Verbose:    verbose,
	})
	if err != nil {
		return workspace.SyncResult{}, manifestPath, err
	}
	if err := workspace.Save(manifestPath, desired); err != nil {
		rollbackErr := rollbackWorkspaceConfigChange(ctx, out, globalHome, target.Root, hadExistingConfig, previousConfig, verbose)
		if rollbackErr != nil {
			return workspace.SyncResult{}, manifestPath, fmt.Errorf("save workspace manifest: %w (rollback failed: %v)", err, rollbackErr)
		}
		return workspace.SyncResult{}, manifestPath, fmt.Errorf("save workspace manifest: %w", err)
	}
	target.Config = desired
	target.ConfigPath = manifestPath
	target.Name = desired.Name()
	target.Status = "ready"
	return result, manifestPath, nil
}

func rollbackWorkspaceConfigChange(ctx context.Context, out io.Writer, globalHome, root string, hadExistingConfig bool, previousConfig workspace.Config, verbose bool) error {
	if hadExistingConfig {
		_, err := workspace.Sync(ctx, root, previousConfig, workspace.SyncOptions{
			Out:        out,
			GlobalHome: globalHome,
			Verbose:    verbose,
		})
		return err
	}
	for _, path := range []string{workspace.Home(root), workspace.LockPath(root)} {
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("cleanup workspace path %s: %w", path, err)
		}
	}
	return nil
}

func workspaceConfigExists(path string) bool {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return false
	}
	info, err := os.Stat(trimmed)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
