package runtimes

import (
	"github.com/sourceplane/kiox/internal/runtime"
	runtimelocal "github.com/sourceplane/kiox/internal/runtimes/local"
	runtimeoci "github.com/sourceplane/kiox/internal/runtimes/oci"
	runtimescript "github.com/sourceplane/kiox/internal/runtimes/script"
)

func NewBuiltinRegistry() (*runtime.Registry, error) {
	registry := runtime.NewRegistry()
	for _, plugin := range []runtime.Plugin{
		runtimelocal.Plugin{},
		runtimeoci.Plugin{},
		runtimescript.Plugin{},
	} {
		if err := registry.Register(plugin); err != nil {
			return nil, err
		}
	}
	return registry, nil
}
