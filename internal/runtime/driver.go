package runtime

import (
	"fmt"
	"io"
	goruntime "runtime"
	"strings"

	"github.com/sourceplane/tinx/internal/manifest"
	"github.com/sourceplane/tinx/internal/oci"
	"github.com/sourceplane/tinx/internal/state"
)

type RuntimePlan struct {
	Package  manifest.Package
	Runtime  manifest.RuntimeSpec
	Metadata state.ProviderMetadata
}

type PrepareRequest struct {
	Home          string
	WorkspaceRoot string
	ToolName      string
	Metadata      state.ProviderMetadata
	GOOS          string
	GOARCH        string
	Out           io.Writer
}

type PrepareResult struct {
	Executable  string
	Env         map[string]string
	PathEntries []string
	Runtime     manifest.RuntimeSpec
}

type Driver interface {
	Resolve(meta state.ProviderMetadata) (RuntimePlan, error)
	Materialize(plan RuntimePlan, req PrepareRequest) (string, error)
	Prepare(plan RuntimePlan, req PrepareRequest, executable string) (PrepareResult, error)
	Cleanup(plan RuntimePlan) error
}

type Manager struct {
	drivers map[string]Driver
}

func NewManager() Manager {
	return Manager{
		drivers: map[string]Driver{
			manifest.RuntimeBinary: binaryDriver{},
		},
	}
}

func (m Manager) PrepareTool(req PrepareRequest) (PrepareResult, error) {
	if strings.TrimSpace(req.GOOS) == "" {
		req.GOOS = goruntime.GOOS
	}
	if strings.TrimSpace(req.GOARCH) == "" {
		req.GOARCH = goruntime.GOARCH
	}
	plan, err := m.resolve(req.Metadata)
	if err != nil {
		return PrepareResult{}, err
	}
	driver := m.drivers[strings.TrimSpace(plan.Runtime.Type)]
	executable, err := driver.Materialize(plan, req)
	if err != nil {
		return PrepareResult{}, err
	}
	return driver.Prepare(plan, req, executable)
}

func (m Manager) resolve(meta state.ProviderMetadata) (RuntimePlan, error) {
	layoutPath := strings.TrimSpace(meta.Source.LayoutPath)
	if layoutPath == "" {
		return RuntimePlan{}, fmt.Errorf("package layout is missing for %s/%s@%s", meta.Namespace, meta.Name, meta.Version)
	}
	pkg, err := oci.LoadPackageManifest(layoutPath, meta.Source.Tag)
	if err != nil {
		return RuntimePlan{}, err
	}
	runtimeSpec := pkg.Spec.Runtime
	if strings.TrimSpace(runtimeSpec.Type) == "" {
		runtimeSpec.Type = manifest.RuntimeBinary
	}
	driver, ok := m.drivers[strings.TrimSpace(runtimeSpec.Type)]
	if !ok {
		return RuntimePlan{}, fmt.Errorf("unsupported runtime %q for %s/%s@%s", runtimeSpec.Type, meta.Namespace, meta.Name, meta.Version)
	}
	plan, err := driver.Resolve(meta)
	if err != nil {
		return RuntimePlan{}, err
	}
	plan.Package = pkg
	plan.Runtime = runtimeSpec
	return plan, nil
}
