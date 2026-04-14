package runtime

import (
	"fmt"
	"io"
	"os"

	"github.com/sourceplane/tinx/internal/core"
	"github.com/sourceplane/tinx/internal/state"
)

type Context struct {
	Home          string
	WorkspaceRoot string
	Alias         string
	Metadata      state.ProviderMetadata
	Package       core.Package
	GoOS          string
	GoArch        string
	WorkingDir    string
	Env           map[string]string
	PathEntries   []string
	Stdout        io.Writer
	Stderr        io.Writer
	Stdin         *os.File
}

type ResolvedTool struct {
	Tool       core.Tool
	BinaryPath string
	InstallDir string
	CacheKey   string
}

type Plugin interface {
	Name() string
	Resolve(tool core.Tool, ctx Context) (ResolvedTool, error)
	Install(resolved ResolvedTool, ctx Context) error
	Execute(resolved ResolvedTool, args []string, ctx Context) error
	IsInstalled(resolved ResolvedTool, ctx Context) (bool, error)
}

type Registry struct {
	plugins map[string]Plugin
}

func NewRegistry() *Registry {
	return &Registry{plugins: map[string]Plugin{}}
}

func (registry *Registry) Register(plugin Plugin) error {
	if plugin == nil {
		return fmt.Errorf("runtime plugin is nil")
	}
	name := plugin.Name()
	if name == "" {
		return fmt.Errorf("runtime plugin name is required")
	}
	if _, exists := registry.plugins[name]; exists {
		return fmt.Errorf("runtime plugin %q is already registered", name)
	}
	registry.plugins[name] = plugin
	return nil
}

func (registry *Registry) Get(name string) (Plugin, bool) {
	plugin, ok := registry.plugins[name]
	return plugin, ok
}

func (registry *Registry) MustGet(name string) (Plugin, error) {
	plugin, ok := registry.Get(name)
	if !ok {
		return nil, fmt.Errorf("runtime %q is not registered", name)
	}
	return plugin, nil
}
