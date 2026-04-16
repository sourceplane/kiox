package local

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sourceplane/tinx/internal/core"
	truntime "github.com/sourceplane/tinx/internal/runtime"
	"github.com/sourceplane/tinx/internal/state"
)

type Plugin struct{}

func (Plugin) Name() string {
	return core.RuntimeLocal
}

func (Plugin) Resolve(tool core.Tool, ctx truntime.Context) (truntime.ResolvedTool, error) {
	installDir := filepath.Join(state.MetadataStoreRoot(ctx.Metadata), "tools", tool.Metadata.Name)
	binaryPath := strings.TrimSpace(tool.Spec.Source.Path)
	fromInstallDir := false
	if binaryPath == "" {
		binaryPath = strings.TrimSpace(tool.Spec.Source.Ref)
	}
	if binaryPath == "" && strings.TrimSpace(tool.Spec.Install.Tool) != "" {
		binaryPath = tool.InstallPath()
		fromInstallDir = true
	}
	if binaryPath == "" {
		return truntime.ResolvedTool{}, fmt.Errorf("tool %s does not declare a local binary path", tool.Metadata.Name)
	}
	if !filepath.IsAbs(binaryPath) && fromInstallDir {
		binaryPath = filepath.Join(installDir, filepath.FromSlash(binaryPath))
	} else if !filepath.IsAbs(binaryPath) && (strings.Contains(binaryPath, string(os.PathSeparator)) || strings.Contains(binaryPath, "/")) {
		binaryPath = filepath.Join(state.MetadataStoreRoot(ctx.Metadata), filepath.FromSlash(binaryPath))
	}
	return truntime.ResolvedTool{Tool: tool, BinaryPath: binaryPath, InstallDir: installDir}, nil
}

func (Plugin) Install(resolved truntime.ResolvedTool, ctx truntime.Context) error {
	return nil
}

func (Plugin) Execute(resolved truntime.ResolvedTool, args []string, ctx truntime.Context) error {
	return truntime.Execute(truntime.ExecOptions{
		BinaryPath:   resolved.BinaryPath,
		Args:         args,
		WorkingDir:   ctx.WorkingDir,
		ProviderHome: state.MetadataStoreRoot(ctx.Metadata),
		EnvOverrides: ctx.Env,
		PathEntries:  ctx.PathEntries,
		Stdout:       ctx.Stdout,
		Stderr:       ctx.Stderr,
		Stdin:        ctx.Stdin,
	})
}

func (Plugin) IsInstalled(resolved truntime.ResolvedTool, ctx truntime.Context) (bool, error) {
	if strings.Contains(resolved.BinaryPath, string(os.PathSeparator)) || filepath.IsAbs(resolved.BinaryPath) {
		info, err := os.Stat(resolved.BinaryPath)
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, err
		}
		return !info.IsDir(), nil
	}
	_, err := truntime.LookPath(resolved.BinaryPath, ctx.Env, ctx.PathEntries)
	if err == nil {
		return true, nil
	}
	if strings.Contains(err.Error(), "executable file not found") {
		return false, nil
	}
	return false, err
}
