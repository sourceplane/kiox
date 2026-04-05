package workspace

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
	APIVersionV1      = "tinx.io/v1"
	KindWorkspace     = "Workspace"
	KindWorkspaceLock = "WorkspaceLock"
	ManifestName      = "tinx.yaml"
	LockName          = "tinx.lock"
	StateDirName      = ".workspace"
)

var ManifestNames = []string{
	"tinx.yaml",
	"tinx.yml",
	"providers.tx.yaml",
	"providers.tx.yml",
	"providers.tinx.yaml",
	"providers.tinx.yml",
}

var ErrNotWorkspace = errors.New("tinx manifest is not a workspace")

type Config struct {
	APIVersion string              `yaml:"apiVersion,omitempty"`
	Kind       string              `yaml:"kind,omitempty"`
	Workspace  string              `yaml:"workspace,omitempty"`
	Metadata   Metadata            `yaml:"metadata,omitempty"`
	Providers  map[string]Provider `yaml:"providers,omitempty"`
	Spec       Spec                `yaml:"spec,omitempty"`
}

type Metadata struct {
	Name string `yaml:"name,omitempty"`
}

type Spec struct {
	Providers map[string]Provider `yaml:"providers,omitempty"`
}

type Provider struct {
	Source    string `yaml:"source,omitempty"`
	PlainHTTP bool   `yaml:"plainHTTP,omitempty"`
}

type Discovery struct {
	Root       string
	ConfigPath string
	Config     Config
}

type LockFile struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Workspace  string           `yaml:"workspace,omitempty"`
	Providers  []LockedProvider `yaml:"providers,omitempty"`
}

type LockedProvider struct {
	Alias    string `yaml:"alias"`
	Provider string `yaml:"provider"`
	Source   string `yaml:"source"`
	Version  string `yaml:"version"`
}

func (p *Provider) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		p.Source = strings.TrimSpace(node.Value)
		return nil
	case yaml.MappingNode:
		type rawProvider Provider
		var raw rawProvider
		if err := node.Decode(&raw); err != nil {
			return err
		}
		*p = Provider(raw)
		return nil
	default:
		return fmt.Errorf("workspace providers must be a source string or mapping")
	}
}

func Load(path string) (Config, error) {
	var config Config
	data, err := os.ReadFile(path)
	if err != nil {
		return config, fmt.Errorf("read workspace manifest: %w", err)
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("decode workspace manifest: %w", err)
	}
	if err := config.Normalize(); err != nil {
		return config, err
	}
	return config, nil
}

func Save(path string, config Config) error {
	if err := config.Normalize(); err != nil {
		return err
	}
	document := struct {
		APIVersion string              `yaml:"apiVersion,omitempty"`
		Kind       string              `yaml:"kind,omitempty"`
		Workspace  string              `yaml:"workspace,omitempty"`
		Metadata   Metadata            `yaml:"metadata,omitempty"`
		Providers  map[string]Provider `yaml:"providers,omitempty"`
	}{
		APIVersion: config.APIVersion,
		Kind:       "workspace",
		Workspace:  strings.TrimSpace(config.Workspace),
		Metadata:   config.Metadata,
		Providers:  config.ProviderMap(),
	}
	if document.Workspace == "" {
		document.Workspace = config.Name()
	}
	data, err := yaml.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode workspace manifest: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create workspace manifest dir: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

func (c *Config) Normalize() error {
	c.Kind = strings.TrimSpace(c.Kind)
	if strings.EqualFold(c.Kind, KindWorkspace) {
		c.Kind = KindWorkspace
	}
	if c.Kind == "" && (c.Workspace != "" || len(c.Providers) > 0 || len(c.Spec.Providers) > 0) {
		c.Kind = KindWorkspace
	}
	if c.Kind != KindWorkspace {
		return ErrNotWorkspace
	}
	if c.APIVersion == "" {
		c.APIVersion = APIVersionV1
	}
	if c.Metadata.Name == "" {
		c.Metadata.Name = strings.TrimSpace(c.Workspace)
	}
	if len(c.Spec.Providers) == 0 && len(c.Providers) > 0 {
		c.Spec.Providers = c.Providers
	}
	if len(c.Providers) == 0 && len(c.Spec.Providers) > 0 {
		c.Providers = c.Spec.Providers
	}
	if c.APIVersion != APIVersionV1 {
		return fmt.Errorf("unsupported workspace apiVersion %q", c.APIVersion)
	}
	for alias, provider := range c.ProviderMap() {
		if strings.TrimSpace(alias) == "" {
			return fmt.Errorf("workspace provider alias cannot be empty")
		}
		if strings.Contains(alias, "/") {
			return fmt.Errorf("workspace provider alias %q must not contain '/'", alias)
		}
		if strings.TrimSpace(provider.Source) == "" {
			return fmt.Errorf("workspace provider %q is missing source", alias)
		}
	}
	return nil
}

func (c Config) Name() string {
	return strings.TrimSpace(c.Metadata.Name)
}

func (c Config) ProviderMap() map[string]Provider {
	if len(c.Spec.Providers) > 0 {
		return c.Spec.Providers
	}
	return c.Providers
}

func (c Config) ProviderAliases() []string {
	providers := c.ProviderMap()
	aliases := make([]string, 0, len(providers))
	for alias := range providers {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

func (c Config) HasProviderAlias(alias string) bool {
	_, ok := c.ProviderMap()[alias]
	return ok
}

func Discover(startDir string) (*Discovery, error) {
	root, err := filepath.Abs(startDir)
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}
	for {
		for _, name := range ManifestNames {
			path := filepath.Join(root, name)
			_, statErr := os.Stat(path)
			switch {
			case statErr == nil:
				config, loadErr := Load(path)
				if loadErr == nil {
					return &Discovery{Root: root, ConfigPath: path, Config: config}, nil
				}
				if !errors.Is(loadErr, ErrNotWorkspace) {
					return nil, loadErr
				}
			case !os.IsNotExist(statErr):
				return nil, fmt.Errorf("stat workspace manifest: %w", statErr)
			}
		}
		parent := filepath.Dir(root)
		if parent == root {
			return nil, nil
		}
		root = parent
	}
}

func Home(root string) string {
	return filepath.Join(root, StateDirName)
}

func EnvPath(root string) string {
	return filepath.Join(Home(root), "env")
}

func PathPath(root string) string {
	return filepath.Join(Home(root), "path")
}

func BinPath(root string) string {
	return filepath.Join(Home(root), "bin")
}

func LockPath(root string) string {
	return filepath.Join(root, LockName)
}

func ManifestPath(root string) string {
	return filepath.Join(root, ManifestName)
}

func SaveLock(root, name string, providers []LockedProvider) error {
	lock := LockFile{
		APIVersion: APIVersionV1,
		Kind:       KindWorkspaceLock,
		Workspace:  strings.TrimSpace(name),
		Providers:  append([]LockedProvider(nil), providers...),
	}
	data, err := yaml.Marshal(lock)
	if err != nil {
		return fmt.Errorf("encode workspace lock: %w", err)
	}
	return os.WriteFile(LockPath(root), data, 0o644)
}

func (d *Discovery) DisplayName() string {
	if d == nil {
		return ""
	}
	if name := d.Config.Name(); name != "" {
		return name
	}
	return filepath.Base(d.Root)
}
