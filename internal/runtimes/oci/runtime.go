package oci

import (
	"fmt"
	"os"

	"github.com/sourceplane/kiox/internal/core"
	internaloci "github.com/sourceplane/kiox/internal/oci"
	truntime "github.com/sourceplane/kiox/internal/runtime"
	"github.com/sourceplane/kiox/internal/state"
)

type Plugin struct{}

func (Plugin) Name() string {
	return core.RuntimeOCI
}

func (Plugin) Resolve(tool core.Tool, ctx truntime.Context) (truntime.ResolvedTool, error) {
	binaryPath, err := internaloci.ExpectedToolPath(ctx.Metadata, ctx.Package, tool, ctx.GoOS, ctx.GoArch)
	if err != nil {
		return truntime.ResolvedTool{}, err
	}
	return truntime.ResolvedTool{Tool: tool, BinaryPath: binaryPath}, nil
}

func (Plugin) Install(resolved truntime.ResolvedTool, ctx truntime.Context) error {
	_, err := internaloci.MaterializeTool(ctx.Metadata, ctx.Package, resolved.Tool, ctx.GoOS, ctx.GoArch, ctx.Stderr)
	return err
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
	info, err := os.Stat(resolved.BinaryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.IsDir() {
		return false, fmt.Errorf("tool binary path %s is a directory", resolved.BinaryPath)
	}
	return true, nil
}
