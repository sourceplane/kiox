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
	"github.com/sourceplane/tinx/pkg/version"
)

type workspaceTarget struct {
	Root       string
	ConfigPath string
	Config     workspace.Config
}

func executeCLI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	home, strippedArgs, err := extractHomeArg(args)
	if err != nil {
		return err
	}
	root := NewRootCommand()
	root.SetOut(stdout)
	root.SetErr(stderr)
	if home != "" {
		if setErr := root.PersistentFlags().Set("tinx-home", home); setErr != nil {
			return setErr
		}
	}
	root.SetArgs(strippedArgs)
	if err := root.ExecuteContext(ctx); err != nil {
		if !strings.Contains(err.Error(), "unknown command") {
			return err
		}
		fallback := &cobra.Command{}
		fallback.SetOut(stdout)
		fallback.SetErr(stderr)
		if len(strippedArgs) > 0 && strippedArgs[0] == "--" {
			return runWorkspaceCommand(fallback, &rootOptions{Home: home}, strippedArgs[1:])
		}
		return runAlias(fallback, &rootOptions{Home: home}, strippedArgs)
	}
	return nil
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
	if len(command) == 0 {
		return fmt.Errorf("workspace command is required")
	}
	globalHome, err := ensureGlobalHome(root.Home)
	if err != nil {
		return err
	}
	target, err := resolveCurrentWorkspaceTarget(globalHome)
	if err != nil {
		return err
	}
	if target == nil {
		return fmt.Errorf("no active workspace; run tinx use <workspace> first or execute inside a workspace")
	}
	result, err := workspace.Sync(cmd.Context(), target.Root, target.Config, workspace.SyncOptions{
		Out:         cmd.ErrOrStderr(),
		Stdout:      cmd.OutOrStdout(),
		Stderr:      cmd.ErrOrStderr(),
		Stdin:       os.Stdin,
		WorkingDir:  mustGetwd(),
		TinxVersion: version.String(),
	})
	if err != nil {
		return err
	}
	return cmdruntime.Dispatch(cmdruntime.DispatchOptions{
		Home:       result.Home,
		WorkingDir: mustGetwd(),
		Commands:   providerCommandsFromAliases(result.Aliases),
		Command:    command,
		Stdout:     cmd.OutOrStdout(),
		Stderr:     cmd.ErrOrStderr(),
		Stdin:      os.Stdin,
	})
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

func providerCommandsFromAliases(aliases map[string]string) []cmdruntime.ProviderCommand {
	names := make([]string, 0, len(aliases))
	for name := range aliases {
		names = append(names, name)
	}
	sort.Strings(names)
	commands := make([]cmdruntime.ProviderCommand, 0, len(names))
	for _, name := range names {
		commands = append(commands, cmdruntime.ProviderCommand{Name: name, Ref: aliases[name]})
	}
	return commands
}

func commandNameForProvider(meta state.ProviderMetadata) string {
	entrypoint := strings.TrimSpace(filepath.Base(meta.Entrypoint))
	if entrypoint != "" && entrypoint != "." && !looksLikeManifestOrScript(entrypoint) {
		return entrypoint
	}
	if meta.Name != "" {
		return meta.Name
	}
	return entrypoint
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
