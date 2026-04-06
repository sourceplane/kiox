package manifest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	APIVersionV1       = "tinx.io/v1"
	APIVersionV2Alpha1 = "tinx.io/v2alpha1"

	KindPackage  = "Package"
	KindProvider = "Provider"

	RuntimeBinary    = "binary"
	RuntimeWasm      = "wasm"
	RuntimeContainer = "container"
	RuntimeScript    = "script"
)

type Package struct {
	APIVersion string      `yaml:"apiVersion" json:"apiVersion"`
	Kind       string      `yaml:"kind" json:"kind"`
	Metadata   Metadata    `yaml:"metadata" json:"metadata"`
	Spec       PackageSpec `yaml:"spec" json:"spec"`
}

type Provider = Package

type Metadata struct {
	Name        string `yaml:"name" json:"name"`
	Namespace   string `yaml:"namespace" json:"namespace"`
	Version     string `yaml:"version" json:"version"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Homepage    string `yaml:"homepage,omitempty" json:"homepage,omitempty"`
	License     string `yaml:"license,omitempty" json:"license,omitempty"`
}

type PackageSpec struct {
	Runtime      RuntimeSpec           `yaml:"runtime" json:"runtime"`
	Entrypoint   string                `yaml:"entrypoint,omitempty" json:"entrypoint,omitempty"`
	Platforms    []Platform            `yaml:"platforms,omitempty" json:"platforms,omitempty"`
	Dependencies map[string]string     `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`
	Capabilities map[string]Capability `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
	Env          map[string]string     `yaml:"env,omitempty" json:"env,omitempty"`
	Path         []string              `yaml:"path,omitempty" json:"path,omitempty"`
	Layers       Layers                `yaml:"layers,omitempty" json:"layers,omitempty"`
}

type ProviderSpec = PackageSpec

type RuntimeSpec struct {
	Type        string `yaml:"type,omitempty" json:"type,omitempty"`
	Entrypoint  string `yaml:"entrypoint,omitempty" json:"entrypoint,omitempty"`
	Image       string `yaml:"image,omitempty" json:"image,omitempty"`
	Module      string `yaml:"module,omitempty" json:"module,omitempty"`
	Interpreter string `yaml:"interpreter,omitempty" json:"interpreter,omitempty"`
}

func (r *RuntimeSpec) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		r.Type = strings.TrimSpace(node.Value)
		return nil
	case yaml.MappingNode:
		type rawRuntime RuntimeSpec
		var raw rawRuntime
		if err := node.Decode(&raw); err != nil {
			return err
		}
		*r = RuntimeSpec(raw)
		return nil
	default:
		return fmt.Errorf("spec.runtime must be a string or mapping")
	}
}

type Platform struct {
	OS     string `yaml:"os" json:"os"`
	Arch   string `yaml:"arch" json:"arch"`
	Binary string `yaml:"binary,omitempty" json:"binary,omitempty"`
}

type Capability struct {
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

type Layers struct {
	Assets AssetsLayer `yaml:"assets,omitempty" json:"assets,omitempty"`
}

type AssetsLayer struct {
	Root     string   `yaml:"root,omitempty" json:"root,omitempty"`
	Includes []string `yaml:"includes,omitempty" json:"includes,omitempty"`
}

func Load(path string) (Package, error) {
	var pkg Package
	data, err := os.ReadFile(path)
	if err != nil {
		return pkg, fmt.Errorf("read manifest: %w", err)
	}
	if err := yaml.Unmarshal(data, &pkg); err != nil {
		return pkg, fmt.Errorf("decode manifest: %w", err)
	}
	if err := pkg.Normalize(); err != nil {
		return pkg, err
	}
	return pkg, nil
}

func (p *Package) Normalize() error {
	switch {
	case strings.TrimSpace(p.APIVersion) == "":
		p.APIVersion = APIVersionV2Alpha1
	case strings.EqualFold(p.APIVersion, APIVersionV1):
		p.APIVersion = APIVersionV2Alpha1
	case strings.EqualFold(p.APIVersion, APIVersionV2Alpha1):
		p.APIVersion = APIVersionV2Alpha1
	default:
		return fmt.Errorf("unsupported apiVersion %q", p.APIVersion)
	}

	switch {
	case strings.TrimSpace(p.Kind) == "":
		p.Kind = KindPackage
	case strings.EqualFold(p.Kind, KindProvider), strings.EqualFold(p.Kind, KindPackage):
		p.Kind = KindPackage
	default:
		return fmt.Errorf("unsupported kind %q", p.Kind)
	}

	if strings.TrimSpace(p.Spec.Runtime.Type) == "" {
		p.Spec.Runtime.Type = RuntimeBinary
	}
	if strings.TrimSpace(p.Spec.Entrypoint) == "" && strings.TrimSpace(p.Spec.Runtime.Entrypoint) != "" {
		p.Spec.Entrypoint = strings.TrimSpace(p.Spec.Runtime.Entrypoint)
	}
	if strings.TrimSpace(p.Spec.Runtime.Entrypoint) == "" && strings.TrimSpace(p.Spec.Entrypoint) != "" {
		p.Spec.Runtime.Entrypoint = strings.TrimSpace(p.Spec.Entrypoint)
	}
	p.Spec.Runtime.Type = strings.TrimSpace(strings.ToLower(p.Spec.Runtime.Type))
	p.Spec.Runtime.Entrypoint = strings.TrimSpace(p.Spec.Runtime.Entrypoint)
	p.Spec.Runtime.Image = strings.TrimSpace(p.Spec.Runtime.Image)
	p.Spec.Runtime.Module = strings.TrimSpace(p.Spec.Runtime.Module)
	p.Spec.Runtime.Interpreter = strings.TrimSpace(p.Spec.Runtime.Interpreter)
	p.Spec.Entrypoint = strings.TrimSpace(p.Spec.Entrypoint)

	for i := range p.Spec.Platforms {
		p.Spec.Platforms[i].Binary = filepath.ToSlash(strings.TrimSpace(p.Spec.Platforms[i].Binary))
		p.Spec.Platforms[i].OS = strings.TrimSpace(p.Spec.Platforms[i].OS)
		p.Spec.Platforms[i].Arch = strings.TrimSpace(p.Spec.Platforms[i].Arch)
	}
	if len(p.Spec.Path) > 0 {
		normalized := make([]string, 0, len(p.Spec.Path))
		for _, entry := range p.Spec.Path {
			trimmed := strings.TrimSpace(entry)
			if trimmed == "" {
				continue
			}
			normalized = append(normalized, filepath.ToSlash(trimmed))
		}
		p.Spec.Path = normalized
	}
	if len(p.Spec.Dependencies) > 0 {
		normalized := make(map[string]string, len(p.Spec.Dependencies))
		for name, requirement := range p.Spec.Dependencies {
			normalized[strings.TrimSpace(name)] = strings.TrimSpace(requirement)
		}
		p.Spec.Dependencies = normalized
	}
	return p.Validate()
}

func (p Package) Validate() error {
	if p.APIVersion != APIVersionV2Alpha1 {
		return fmt.Errorf("unsupported apiVersion %q", p.APIVersion)
	}
	if p.Kind != KindPackage {
		return fmt.Errorf("unsupported kind %q", p.Kind)
	}
	if strings.TrimSpace(p.Metadata.Namespace) == "" {
		return errors.New("metadata.namespace is required")
	}
	if strings.TrimSpace(p.Metadata.Name) == "" {
		return errors.New("metadata.name is required")
	}
	if strings.TrimSpace(p.Metadata.Version) == "" {
		return errors.New("metadata.version is required")
	}

	switch p.Spec.Runtime.Type {
	case RuntimeBinary:
		if p.Spec.Runtime.Entrypoint == "" {
			return errors.New("spec.runtime.entrypoint is required for binary packages")
		}
		if len(p.Spec.Platforms) == 0 {
			return errors.New("spec.platforms must declare at least one platform for binary packages")
		}
	case RuntimeWasm:
		if p.Spec.Runtime.Entrypoint == "" {
			return errors.New("spec.runtime.entrypoint is required for wasm packages")
		}
		if p.Spec.Runtime.Module == "" {
			return errors.New("spec.runtime.module is required for wasm packages")
		}
	case RuntimeContainer:
		if p.Spec.Runtime.Image == "" {
			return errors.New("spec.runtime.image is required for container packages")
		}
	case RuntimeScript:
		if p.Spec.Runtime.Entrypoint == "" {
			return errors.New("spec.runtime.entrypoint is required for script packages")
		}
		if p.Spec.Runtime.Interpreter == "" {
			return errors.New("spec.runtime.interpreter is required for script packages")
		}
	default:
		return fmt.Errorf("unsupported runtime %q", p.Spec.Runtime.Type)
	}

	seen := map[string]struct{}{}
	for _, platform := range p.Spec.Platforms {
		if strings.TrimSpace(platform.OS) == "" || strings.TrimSpace(platform.Arch) == "" {
			return errors.New("spec.platforms entries require os and arch")
		}
		if p.Spec.Runtime.Type == RuntimeBinary && strings.TrimSpace(platform.Binary) == "" {
			return fmt.Errorf("spec.platforms[%s/%s].binary is required", platform.OS, platform.Arch)
		}
		key := platform.OS + "/" + platform.Arch
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate platform %s", key)
		}
		seen[key] = struct{}{}
	}
	for key := range p.Spec.Env {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			return errors.New("spec.env keys must not be empty")
		}
		if strings.HasPrefix(strings.ToUpper(trimmedKey), "TINX_") {
			return fmt.Errorf("spec.env key %q must not use reserved TINX_ prefix", trimmedKey)
		}
	}
	for _, entry := range p.Spec.Path {
		if strings.TrimSpace(entry) == "" {
			return errors.New("spec.path entries must not be empty")
		}
	}
	for name, requirement := range p.Spec.Dependencies {
		if strings.TrimSpace(name) == "" {
			return errors.New("spec.dependencies keys must not be empty")
		}
		if strings.TrimSpace(requirement) == "" {
			return fmt.Errorf("spec.dependencies[%s] must not be empty", name)
		}
	}
	return nil
}

func (p Package) Ref() string {
	return strings.TrimSpace(p.Metadata.Namespace) + "/" + strings.TrimSpace(p.Metadata.Name)
}

func (p Package) CapabilityNames() []string {
	names := make([]string, 0, len(p.Spec.Capabilities))
	for name := range p.Spec.Capabilities {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (p Package) HasCapability(name string) bool {
	_, ok := p.Spec.Capabilities[name]
	return ok
}

func (p Package) Platform(goos, goarch string) (Platform, bool) {
	for _, platform := range p.Spec.Platforms {
		if platform.OS == goos && platform.Arch == goarch {
			return platform, true
		}
	}
	return Platform{}, false
}

func (p Package) AssetsRoot() string {
	root := strings.TrimSpace(p.Spec.Layers.Assets.Root)
	if root == "" {
		return ""
	}
	return filepath.ToSlash(root)
}
