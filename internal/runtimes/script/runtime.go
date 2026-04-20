package script

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sourceplane/kiox/internal/core"
	truntime "github.com/sourceplane/kiox/internal/runtime"
	"github.com/sourceplane/kiox/internal/state"
)

type Plugin struct{}

func (Plugin) Name() string {
	return core.RuntimeScript
}

func (Plugin) Resolve(tool core.Tool, ctx truntime.Context) (truntime.ResolvedTool, error) {
	installDir := filepath.Join(state.MetadataStoreRoot(ctx.Metadata), "tools", tool.Metadata.Name)
	installPath := tool.InstallPath()
	if installPath == "" {
		return truntime.ResolvedTool{}, fmt.Errorf("tool %s does not declare an install path", tool.Metadata.Name)
	}
	binaryPath := filepath.Join(installDir, filepath.FromSlash(installPath))
	cacheKey := tool.Spec.Cache.Key
	if cacheKey == "" {
		cacheKey = ctx.Metadata.Namespace + "/" + ctx.Metadata.Name + "/" + tool.Metadata.Name + "@" + ctx.Metadata.Version
	}
	return truntime.ResolvedTool{Tool: tool, InstallDir: installDir, BinaryPath: binaryPath, CacheKey: cacheKey}, nil
}

func (Plugin) Install(resolved truntime.ResolvedTool, ctx truntime.Context) error {
	if err := os.MkdirAll(filepath.Dir(resolved.BinaryPath), 0o755); err != nil {
		return fmt.Errorf("create tool install dir: %w", err)
	}
	command := exec.Command("/bin/sh", "-c", resolved.Tool.Spec.Source.Script)
	command.Dir = firstNonEmpty(ctx.WorkingDir, state.MetadataStoreRoot(ctx.Metadata))
	env := truntime.CommandEnvironment(os.Environ(), mergeScriptEnvironment(ctx, resolved), append(ctx.PathEntries, filepath.Join(resolved.InstallDir, "bin")))
	command.Env = env
	command.Stdout = ctx.Stdout
	command.Stderr = ctx.Stderr
	command.Stdin = ctx.Stdin
	if err := command.Run(); err != nil {
		return fmt.Errorf("install script tool %s: %w", resolved.Tool.Metadata.Name, err)
	}
	return nil
}

func (Plugin) Execute(resolved truntime.ResolvedTool, args []string, ctx truntime.Context) error {
	return truntime.Execute(truntime.ExecOptions{
		BinaryPath:   resolved.BinaryPath,
		Args:         args,
		WorkingDir:   ctx.WorkingDir,
		ProviderHome: state.MetadataStoreRoot(ctx.Metadata),
		EnvOverrides: ctx.Env,
		PathEntries:  append(ctx.PathEntries, filepath.Join(resolved.InstallDir, "bin")),
		Stdout:       ctx.Stdout,
		Stderr:       ctx.Stderr,
		Stdin:        ctx.Stdin,
	})
}

func (Plugin) IsInstalled(resolved truntime.ResolvedTool, ctx truntime.Context) (bool, error) {
	info, err := os.Stat(resolved.BinaryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return !info.IsDir(), nil
}

func mergeScriptEnvironment(ctx truntime.Context, resolved truntime.ResolvedTool) map[string]string {
	env := make(map[string]string, len(ctx.Env)+5)
	for key, value := range ctx.Env {
		env[key] = value
	}
	env["KIOX_TOOL_INSTALL_DIR"] = resolved.InstallDir
	env["KIOX_TOOL_BIN"] = resolved.BinaryPath
	env["KIOX_TOOL_NAME"] = resolved.Tool.Metadata.Name
	env["KIOX_TOOL_COMMAND"] = resolved.Tool.PrimaryProvide()
	env["KIOX_PROVIDER_HOME"] = state.MetadataStoreRoot(ctx.Metadata)
	env["KIOX_INTERNAL_CLI"] = ""
	return env
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
