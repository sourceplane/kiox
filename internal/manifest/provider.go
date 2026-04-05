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
	APIVersionV1  = "tinx.io/v1"
	KindProvider  = "Provider"
	RuntimeBinary = "binary"
)

type Provider struct {
	APIVersion string       `yaml:"apiVersion" json:"apiVersion"`
	Kind       string       `yaml:"kind" json:"kind"`
	Metadata   Metadata     `yaml:"metadata" json:"metadata"`
	Spec       ProviderSpec `yaml:"spec" json:"spec"`
}

type Metadata struct {
	Name        string `yaml:"name" json:"name"`
	Namespace   string `yaml:"namespace" json:"namespace"`
	Version     string `yaml:"version" json:"version"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Homepage    string `yaml:"homepage,omitempty" json:"homepage,omitempty"`
	License     string `yaml:"license,omitempty" json:"license,omitempty"`
}

type ProviderSpec struct {
	Runtime      string                `yaml:"runtime" json:"runtime"`
	Entrypoint   string                `yaml:"entrypoint" json:"entrypoint"`
	Platforms    []Platform            `yaml:"platforms" json:"platforms"`
	Capabilities map[string]Capability `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
	Env          map[string]string     `yaml:"env,omitempty" json:"env,omitempty"`
	Path         []string              `yaml:"path,omitempty" json:"path,omitempty"`
	Layers       Layers                `yaml:"layers,omitempty" json:"layers,omitempty"`
}

type Platform struct {
	OS     string `yaml:"os" json:"os"`
	Arch   string `yaml:"arch" json:"arch"`
	Binary string `yaml:"binary" json:"binary"`
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

func Load(path string) (Provider, error) {
	var provider Provider
	data, err := os.ReadFile(path)
	if err != nil {
		return provider, fmt.Errorf("read manifest: %w", err)
	}
	if err := yaml.Unmarshal(data, &provider); err != nil {
		return provider, fmt.Errorf("decode manifest: %w", err)
	}
	if err := provider.Normalize(); err != nil {
		return provider, err
	}
	return provider, nil
}

func (p *Provider) Normalize() error {
	if p.APIVersion == "" {
		p.APIVersion = APIVersionV1
	}
	if p.Spec.Runtime == "" {
		p.Spec.Runtime = RuntimeBinary
	}
	for i := range p.Spec.Platforms {
		p.Spec.Platforms[i].Binary = filepath.ToSlash(p.Spec.Platforms[i].Binary)
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
	return p.Validate()
}

func (p Provider) Validate() error {
	if p.APIVersion != APIVersionV1 {
		return fmt.Errorf("unsupported apiVersion %q", p.APIVersion)
	}
	if p.Kind != KindProvider {
		return fmt.Errorf("unsupported kind %q", p.Kind)
	}
	if p.Metadata.Namespace == "" {
		return errors.New("metadata.namespace is required")
	}
	if p.Metadata.Name == "" {
		return errors.New("metadata.name is required")
	}
	if p.Metadata.Version == "" {
		return errors.New("metadata.version is required")
	}
	if p.Spec.Runtime != RuntimeBinary {
		return fmt.Errorf("unsupported runtime %q", p.Spec.Runtime)
	}
	if p.Spec.Entrypoint == "" {
		return errors.New("spec.entrypoint is required")
	}
	if len(p.Spec.Platforms) == 0 {
		return errors.New("spec.platforms must declare at least one platform")
	}
	seen := map[string]struct{}{}
	for _, platform := range p.Spec.Platforms {
		if platform.OS == "" || platform.Arch == "" {
			return errors.New("spec.platforms entries require os and arch")
		}
		if platform.Binary == "" {
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
	return nil
}

func (p Provider) Ref() string {
	return p.Metadata.Namespace + "/" + p.Metadata.Name
}

func (p Provider) CapabilityNames() []string {
	names := make([]string, 0, len(p.Spec.Capabilities))
	for name := range p.Spec.Capabilities {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (p Provider) HasCapability(name string) bool {
	_, ok := p.Spec.Capabilities[name]
	return ok
}

func (p Provider) Platform(goos, goarch string) (Platform, bool) {
	for _, platform := range p.Spec.Platforms {
		if platform.OS == goos && platform.Arch == goarch {
			return platform, true
		}
	}
	return Platform{}, false
}

func (p Provider) AssetsRoot() string {
	root := strings.TrimSpace(p.Spec.Layers.Assets.Root)
	if root == "" {
		return ""
	}
	return filepath.ToSlash(root)
}
