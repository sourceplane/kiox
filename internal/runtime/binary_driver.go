package runtime

import (
	"os"
	"strings"

	"github.com/sourceplane/tinx/internal/manifest"
	"github.com/sourceplane/tinx/internal/oci"
	"github.com/sourceplane/tinx/internal/state"
)

type binaryDriver struct{}

func (binaryDriver) Resolve(meta state.ProviderMetadata) (RuntimePlan, error) {
	return RuntimePlan{
		Metadata: meta,
		Runtime: manifest.RuntimeSpec{
			Type: manifest.RuntimeBinary,
		},
	}, nil
}

func (binaryDriver) Materialize(plan RuntimePlan, req PrepareRequest) (string, error) {
	binaryPath := oci.CurrentBinaryPath(plan.Metadata)
	if info, err := os.Stat(binaryPath); err == nil && !info.IsDir() {
		return binaryPath, nil
	}
	return oci.MaterializeRuntime(plan.Metadata, req.GOOS, req.GOARCH, req.Out)
}

func (binaryDriver) Prepare(plan RuntimePlan, req PrepareRequest, executable string) (PrepareResult, error) {
	env, pathEntries, err := ResolveProviderEnvironment(ProviderEnvironmentSpec{
		Home:          req.Home,
		WorkspaceRoot: req.WorkspaceRoot,
		Alias:         req.ToolName,
		BinaryPath:    executable,
		Metadata:      plan.Metadata,
	})
	if err != nil {
		return PrepareResult{}, err
	}
	return PrepareResult{
		Executable:  executable,
		Env:         env,
		PathEntries: dedupePathEntries(pathEntries),
		Runtime:     plan.Runtime,
	}, nil
}

func (binaryDriver) Cleanup(RuntimePlan) error {
	return nil
}

func dedupePathEntries(entries []string) []string {
	if len(entries) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(entries))
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		out = append(out, entry)
	}
	return out
}
