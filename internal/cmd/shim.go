package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/core"
	"github.com/sourceplane/tinx/internal/oci"
	truntime "github.com/sourceplane/tinx/internal/runtime"
	"github.com/sourceplane/tinx/internal/runtimes"
	"github.com/sourceplane/tinx/internal/state"
	"github.com/sourceplane/tinx/internal/workspace"
)

func newShimCommand(root *rootOptions) *cobra.Command {
	var workspaceRoot string
	var alias string
	var toolName string

	cmd := &cobra.Command{
		Use:                "__shim",
		Short:              "Internal lazy tool shim entrypoint",
		Hidden:             true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		DisableFlagParsing: false,
		Args:               cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShimCommand(cmd, root, workspaceRoot, alias, toolName, args)
		},
	}
	cmd.Flags().StringVar(&workspaceRoot, "workspace-root", "", "workspace root for the shim")
	cmd.Flags().StringVar(&alias, "alias", "", "workspace provider alias")
	cmd.Flags().StringVar(&toolName, "tool", "", "tool name to execute")
	return cmd
}

func runShimCommand(cmd *cobra.Command, root *rootOptions, workspaceRoot, alias, toolName string, args []string) error {
	absRoot, err := filepath.Abs(strings.TrimSpace(workspaceRoot))
	if err != nil {
		return fmt.Errorf("resolve workspace root: %w", err)
	}
	if strings.TrimSpace(alias) == "" {
		return fmt.Errorf("shim alias is required")
	}
	discovery, err := workspace.Discover(absRoot)
	if err != nil {
		return err
	}
	if discovery == nil {
		return fmt.Errorf("workspace root %q does not contain a workspace manifest", absRoot)
	}
	globalHome := strings.TrimSpace(os.Getenv("TINX_GLOBAL_HOME"))
	if globalHome == "" {
		globalHome = strings.TrimSpace(root.Home)
	}
	result, err := workspace.Sync(cmd.Context(), discovery.Root, discovery.Config, workspace.SyncOptions{
		Out:        cmd.ErrOrStderr(),
		GlobalHome: globalHome,
	})
	if err != nil {
		return err
	}
	providerKey := strings.TrimSpace(result.Aliases[alias])
	if providerKey == "" {
		return fmt.Errorf("workspace alias %q is not installed", alias)
	}
	meta, err := state.LoadProviderMetadataByKey(result.Home, providerKey)
	if err != nil {
		return err
	}
	pkg, err := oci.LoadPackageModel(meta)
	if err != nil {
		return err
	}
	if strings.TrimSpace(toolName) == "" {
		tool, ok := pkg.DefaultTool()
		if !ok {
			return fmt.Errorf("provider %s/%s@%s has no default tool", meta.Namespace, meta.Name, meta.Version)
		}
		toolName = tool.Metadata.Name
	}
	plan, err := core.ResolveToolPlan(pkg, toolName)
	if err != nil {
		return err
	}
	registry, err := runtimes.NewBuiltinRegistry()
	if err != nil {
		return err
	}
	accumulatedEnv := map[string]string{}
	accumulatedPaths := []string{}
	resolvedByTool := map[string]truntime.ResolvedTool{}
	workingDir := workspaceWorkingDir(discovery.Root)

	for _, tool := range plan {
		plugin, err := registry.MustGet(tool.Spec.Runtime.Type)
		if err != nil {
			return err
		}
		toolCtx := truntime.Context{
			Home:          result.Home,
			WorkspaceRoot: discovery.Root,
			Alias:         alias,
			Metadata:      meta,
			Package:       pkg,
			GoOS:          goruntime.GOOS,
			GoArch:        goruntime.GOARCH,
			WorkingDir:    workingDir,
			Env:           accumulatedEnv,
			PathEntries:   accumulatedPaths,
			Stdout:        cmd.OutOrStdout(),
			Stderr:        cmd.ErrOrStderr(),
			Stdin:         os.Stdin,
		}
		resolved, err := plugin.Resolve(tool, toolCtx)
		if err != nil {
			return err
		}
		toolEnv, toolPaths, err := truntime.ResolveProviderEnvironment(truntime.ProviderEnvironmentSpec{
			Home:          result.Home,
			WorkspaceRoot: discovery.Root,
			Alias:         alias,
			ToolName:      tool.Metadata.Name,
			BinaryPath:    resolved.BinaryPath,
			Metadata:      meta,
		})
		if err != nil {
			return err
		}
		if err := mergeShimEnvironment(accumulatedEnv, toolEnv, tool.Metadata.Name); err != nil {
			return err
		}
		accumulatedPaths = appendShimPaths(accumulatedPaths, toolPaths...)
		if binaryDir := filepath.Dir(strings.TrimSpace(resolved.BinaryPath)); binaryDir != "" && binaryDir != "." {
			accumulatedPaths = appendShimPaths(accumulatedPaths, binaryDir)
		}
		toolCtx.Env = accumulatedEnv
		toolCtx.PathEntries = accumulatedPaths
		installed, err := plugin.IsInstalled(resolved, toolCtx)
		if err != nil {
			return err
		}
		if !installed {
			if err := installManagedTool(tool, resolved, toolCtx, registry, resolvedByTool); err != nil {
				return err
			}
		}
		resolvedByTool[tool.Metadata.Name] = resolved
	}
	targetTool := plan[len(plan)-1]
	targetResolved := resolvedByTool[targetTool.Metadata.Name]
	targetPlugin, err := registry.MustGet(targetTool.Spec.Runtime.Type)
	if err != nil {
		return err
	}
	return targetPlugin.Execute(targetResolved, args, truntime.Context{
		Home:          result.Home,
		WorkspaceRoot: discovery.Root,
		Alias:         alias,
		Metadata:      meta,
		Package:       pkg,
		GoOS:          goruntime.GOOS,
		GoArch:        goruntime.GOARCH,
		WorkingDir:    workingDir,
		Env:           accumulatedEnv,
		PathEntries:   accumulatedPaths,
		Stdout:        cmd.OutOrStdout(),
		Stderr:        cmd.ErrOrStderr(),
		Stdin:         os.Stdin,
	})
}

func installManagedTool(tool core.Tool, resolved truntime.ResolvedTool, toolCtx truntime.Context, registry *truntime.Registry, resolvedByTool map[string]truntime.ResolvedTool) error {
	plugin, err := registry.MustGet(tool.Spec.Runtime.Type)
	if err != nil {
		return err
	}
	installerName := strings.TrimSpace(tool.Spec.Install.Tool)
	if installerName == "" {
		if err := plugin.Install(resolved, toolCtx); err != nil {
			return err
		}
	} else {
		installerResolved, ok := resolvedByTool[installerName]
		if !ok {
			return fmt.Errorf("tool %s installer %q was not resolved", tool.Metadata.Name, installerName)
		}
		installerTool, ok := toolCtx.Package.Tool(installerName)
		if !ok {
			return fmt.Errorf("tool %s installer %q was not found", tool.Metadata.Name, installerName)
		}
		installerPlugin, err := registry.MustGet(installerTool.Spec.Runtime.Type)
		if err != nil {
			return err
		}
		installCtx := toolCtx
		installCtx.Env = installTargetEnvironment(toolCtx.Env, resolved)
		if err := installerPlugin.Execute(installerResolved, []string{resolved.BinaryPath}, installCtx); err != nil {
			return fmt.Errorf("install tool %s with %s: %w", tool.Metadata.Name, installerName, err)
		}
	}
	installed, err := plugin.IsInstalled(resolved, toolCtx)
	if err != nil {
		return err
	}
	if !installed {
		return fmt.Errorf("tool %s did not produce expected binary %s", tool.Metadata.Name, resolved.BinaryPath)
	}
	return nil
}

func installTargetEnvironment(existing map[string]string, resolved truntime.ResolvedTool) map[string]string {
	env := make(map[string]string, len(existing)+4)
	for key, value := range existing {
		env[key] = value
	}
	env["TINX_TARGET_TOOL_NAME"] = resolved.Tool.Metadata.Name
	env["TINX_TARGET_TOOL_BIN"] = resolved.BinaryPath
	env["TINX_TARGET_TOOL_COMMAND"] = resolved.Tool.PrimaryProvide()
	env["TINX_TARGET_TOOL_INSTALL_DIR"] = resolved.InstallDir
	return env
}

func mergeShimEnvironment(existing, additions map[string]string, toolName string) error {
	for key, value := range additions {
		if current, ok := existing[key]; ok && current != value {
			return fmt.Errorf("workspace env conflict for %s while resolving tool %s", key, toolName)
		}
		existing[key] = value
	}
	return nil
}

func appendShimPaths(existing []string, values ...string) []string {
	seen := make(map[string]struct{}, len(existing)+len(values))
	for _, value := range existing {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		seen[trimmed] = struct{}{}
	}
	result := append([]string(nil), existing...)
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}
