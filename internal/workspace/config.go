package workspace

import (
	"crypto/sha256"
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

	KindWorkspace     = "Workspace"
	KindWorkspaceLock = "WorkspaceLock"

	ManifestName       = "tinx.yaml"
	LockName           = "tinx.lock"
	StateDirName       = ".tinx"
	LegacyStateDirName = ".workspace"
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
	APIVersion string          `yaml:"apiVersion,omitempty"`
	Kind       string          `yaml:"kind,omitempty"`
	Workspace  string          `yaml:"workspace,omitempty"`
	Metadata   Metadata        `yaml:"metadata,omitempty"`
	Tools      map[string]Tool `yaml:"tools,omitempty"`
	Providers  map[string]Tool `yaml:"providers,omitempty"`
	Spec       Spec            `yaml:"spec,omitempty"`
}

type Metadata struct {
	Name string `yaml:"name,omitempty"`
}

type Spec struct {
	Tools     map[string]Tool `yaml:"tools,omitempty"`
	Providers map[string]Tool `yaml:"providers,omitempty"`
}

type Tool struct {
	Package   string `yaml:"package,omitempty"`
	Version   string `yaml:"version,omitempty"`
	Source    string `yaml:"source,omitempty"`
	Runtime   string `yaml:"runtime,omitempty"`
	PlainHTTP bool   `yaml:"plainHTTP,omitempty"`
}

type Provider = Tool

type Discovery struct {
	Root       string
	ConfigPath string
	Config     Config
}

type LockFile struct {
	APIVersion   string           `yaml:"apiVersion"`
	Kind         string           `yaml:"kind"`
	Workspace    string           `yaml:"workspace,omitempty"`
	ManifestHash string           `yaml:"manifestHash,omitempty"`
	Tools        []LockedTool     `yaml:"tools,omitempty"`
	Packages     []LockedPackage  `yaml:"packages,omitempty"`
	Providers    []LockedProvider `yaml:"providers,omitempty"`
}

type LockedTool struct {
	Name            string `yaml:"name"`
	Package         string `yaml:"package"`
	Constraint      string `yaml:"constraint,omitempty"`
	ResolvedPackage string `yaml:"resolvedPackage"`
}

type LockedPackage struct {
	Package      string            `yaml:"package"`
	Version      string            `yaml:"version"`
	Source       string            `yaml:"source,omitempty"`
	Resolved     string            `yaml:"resolved,omitempty"`
	Digest       string            `yaml:"digest,omitempty"`
	ContentStore string            `yaml:"contentStore,omitempty"`
	Runtime      LockedRuntime     `yaml:"runtime,omitempty"`
	Dependencies map[string]string `yaml:"dependencies,omitempty"`
}

type LockedRuntime struct {
	Type        string `yaml:"type,omitempty"`
	Entrypoint  string `yaml:"entrypoint,omitempty"`
	Image       string `yaml:"image,omitempty"`
	Module      string `yaml:"module,omitempty"`
	Interpreter string `yaml:"interpreter,omitempty"`
}

type LockedProvider struct {
	Alias    string `yaml:"alias"`
	Provider string `yaml:"provider"`
	Source   string `yaml:"source,omitempty"`
	Version  string `yaml:"version,omitempty"`
	Resolved string `yaml:"resolved,omitempty"`
	Store    string `yaml:"store,omitempty"`
}

func (t *Tool) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		parsed, err := parseToolString(node.Value)
		if err != nil {
			return err
		}
		*t = parsed
		return nil
	case yaml.MappingNode:
		type rawTool Tool
		var raw rawTool
		if err := node.Decode(&raw); err != nil {
			return err
		}
		*t = Tool(raw)
		return nil
	default:
		return fmt.Errorf("workspace tools must be a string or mapping")
	}
}

func parseToolString(value string) (Tool, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return Tool{}, fmt.Errorf("workspace tool source cannot be empty")
	}
	if packageRef, constraint, ok := parsePackageRequirement(trimmed); ok {
		return Tool{
			Package: packageRef,
			Version: constraint,
		}, nil
	}
	return Tool{Source: trimmed}, nil
}

func parsePackageRequirement(value string) (string, string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, ".") {
		return "", "", false
	}
	if strings.Contains(trimmed, "://") {
		return "", "", false
	}
	base := trimmed
	constraint := ""
	if at := strings.Index(trimmed, "@"); at >= 0 {
		base = trimmed[:at]
		constraint = strings.TrimSpace(trimmed[at+1:])
	}
	if strings.Count(base, "/") != 1 {
		return "", "", false
	}
	parts := strings.SplitN(base, "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	if strings.ContainsAny(parts[0], ".:") || parts[0] == "localhost" {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]) + "/" + strings.TrimSpace(parts[1]), constraint, true
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
	document, err := normalizedManifestDocument(config)
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode workspace manifest: %w", err)
	}
	return atomicWriteFile(path, data, 0o644)
}

func ManifestHash(config Config) (string, error) {
	document, err := normalizedManifestDocument(config)
	if err != nil {
		return "", err
	}
	data, err := yaml.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("encode workspace manifest: %w", err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", sum[:]), nil
}

func normalizedManifestDocument(config Config) (struct {
	APIVersion string          `yaml:"apiVersion,omitempty"`
	Kind       string          `yaml:"kind,omitempty"`
	Metadata   Metadata        `yaml:"metadata,omitempty"`
	Tools      map[string]Tool `yaml:"tools,omitempty"`
}, error) {
	if err := config.Normalize(); err != nil {
		return struct {
			APIVersion string          `yaml:"apiVersion,omitempty"`
			Kind       string          `yaml:"kind,omitempty"`
			Metadata   Metadata        `yaml:"metadata,omitempty"`
			Tools      map[string]Tool `yaml:"tools,omitempty"`
		}{}, err
	}
	tools := make(map[string]Tool, len(config.ToolMap()))
	for name, tool := range config.ToolMap() {
		tools[name] = tool
	}
	document := struct {
		APIVersion string          `yaml:"apiVersion,omitempty"`
		Kind       string          `yaml:"kind,omitempty"`
		Metadata   Metadata        `yaml:"metadata,omitempty"`
		Tools      map[string]Tool `yaml:"tools,omitempty"`
	}{
		APIVersion: config.APIVersion,
		Kind:       KindWorkspace,
		Metadata:   config.Metadata,
		Tools:      tools,
	}
	if strings.TrimSpace(document.Metadata.Name) == "" {
		document.Metadata.Name = config.Name()
	}
	return document, nil
}

func (c *Config) Normalize() error {
	c.Kind = strings.TrimSpace(c.Kind)
	switch {
	case strings.EqualFold(c.Kind, KindWorkspace):
		c.Kind = KindWorkspace
	case c.Kind == "" && (c.Workspace != "" || len(c.Tools) > 0 || len(c.Providers) > 0 || len(c.Spec.Tools) > 0 || len(c.Spec.Providers) > 0):
		c.Kind = KindWorkspace
	}
	if c.Kind != KindWorkspace {
		return ErrNotWorkspace
	}

	switch {
	case strings.TrimSpace(c.APIVersion) == "":
		c.APIVersion = APIVersionV2Alpha1
	case strings.EqualFold(c.APIVersion, APIVersionV1):
		c.APIVersion = APIVersionV2Alpha1
	case strings.EqualFold(c.APIVersion, APIVersionV2Alpha1):
		c.APIVersion = APIVersionV2Alpha1
	default:
		return fmt.Errorf("unsupported workspace apiVersion %q", c.APIVersion)
	}

	if strings.TrimSpace(c.Metadata.Name) == "" {
		c.Metadata.Name = strings.TrimSpace(c.Workspace)
	}

	switch {
	case len(c.Tools) == 0 && len(c.Spec.Tools) > 0:
		c.Tools = copyTools(c.Spec.Tools)
	case len(c.Spec.Tools) == 0 && len(c.Tools) > 0:
		c.Spec.Tools = copyTools(c.Tools)
	}
	switch {
	case len(c.Tools) == 0 && len(c.Providers) > 0:
		c.Tools = copyTools(c.Providers)
	case len(c.Providers) == 0 && len(c.Tools) > 0:
		c.Providers = copyTools(c.Tools)
	}
	switch {
	case len(c.Tools) == 0 && len(c.Spec.Providers) > 0:
		c.Tools = copyTools(c.Spec.Providers)
	case len(c.Spec.Providers) == 0 && len(c.Tools) > 0:
		c.Spec.Providers = copyTools(c.Tools)
	}
	if len(c.Spec.Tools) == 0 && len(c.Tools) > 0 {
		c.Spec.Tools = copyTools(c.Tools)
	}

	normalizedTools := make(map[string]Tool, len(c.ToolMap()))
	for name, tool := range c.ToolMap() {
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("workspace tool name cannot be empty")
		}
		if strings.Contains(name, "/") {
			return fmt.Errorf("workspace tool name %q must not contain '/'", name)
		}
		normalized, err := normalizeTool(tool)
		if err != nil {
			return fmt.Errorf("workspace tool %q: %w", name, err)
		}
		normalizedTools[name] = normalized
	}
	c.Tools = normalizedTools
	c.Providers = copyTools(normalizedTools)
	c.Spec.Tools = copyTools(normalizedTools)
	c.Spec.Providers = copyTools(normalizedTools)
	return nil
}

func normalizeTool(tool Tool) (Tool, error) {
	tool.Package = strings.TrimSpace(tool.Package)
	tool.Version = strings.TrimSpace(tool.Version)
	tool.Source = strings.TrimSpace(tool.Source)
	tool.Runtime = strings.TrimSpace(tool.Runtime)

	if tool.Package != "" {
		if packageRef, constraint, ok := parsePackageRequirement(tool.Package); ok {
			tool.Package = packageRef
			if tool.Version == "" {
				tool.Version = constraint
			}
		}
	}
	if tool.Package == "" && tool.Source != "" {
		if packageRef, constraint, ok := parsePackageRequirement(tool.Source); ok {
			tool.Package = packageRef
			tool.Source = ""
			if tool.Version == "" {
				tool.Version = constraint
			}
		}
	}
	if tool.Package == "" && tool.Source == "" {
		return Tool{}, fmt.Errorf("tool must declare package or source")
	}
	return tool, nil
}

func copyTools(tools map[string]Tool) map[string]Tool {
	if len(tools) == 0 {
		return nil
	}
	cloned := make(map[string]Tool, len(tools))
	for name, tool := range tools {
		cloned[name] = tool
	}
	return cloned
}

func (c Config) Name() string {
	if name := strings.TrimSpace(c.Metadata.Name); name != "" {
		return name
	}
	return strings.TrimSpace(c.Workspace)
}

func (c Config) ToolMap() map[string]Tool {
	if len(c.Tools) > 0 {
		return c.Tools
	}
	if len(c.Spec.Tools) > 0 {
		return c.Spec.Tools
	}
	if len(c.Providers) > 0 {
		return c.Providers
	}
	return c.Spec.Providers
}

func (c Config) ProviderMap() map[string]Provider {
	return c.ToolMap()
}

func (c Config) ToolNames() []string {
	tools := c.ToolMap()
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c Config) ProviderAliases() []string {
	return c.ToolNames()
}

func (c Config) HasToolName(name string) bool {
	_, ok := c.ToolMap()[name]
	return ok
}

func (c Config) HasProviderAlias(alias string) bool {
	return c.HasToolName(alias)
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

func LegacyHome(root string) string {
	return filepath.Join(root, LegacyStateDirName)
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

func SaveLock(root, name, manifestHash string, tools []LockedTool, packages []LockedPackage) error {
	lock := struct {
		APIVersion   string          `yaml:"apiVersion"`
		Kind         string          `yaml:"kind"`
		Workspace    string          `yaml:"workspace,omitempty"`
		ManifestHash string          `yaml:"manifestHash,omitempty"`
		Tools        []LockedTool    `yaml:"tools,omitempty"`
		Packages     []LockedPackage `yaml:"packages,omitempty"`
	}{
		APIVersion:   APIVersionV2Alpha1,
		Kind:         KindWorkspaceLock,
		Workspace:    strings.TrimSpace(name),
		ManifestHash: strings.TrimSpace(manifestHash),
		Tools:        append([]LockedTool(nil), tools...),
		Packages:     append([]LockedPackage(nil), packages...),
	}
	data, err := yaml.Marshal(lock)
	if err != nil {
		return fmt.Errorf("encode workspace lock: %w", err)
	}
	return atomicWriteFile(LockPath(root), data, 0o644)
}

func LoadLock(root string) (LockFile, error) {
	var lock LockFile
	data, err := os.ReadFile(LockPath(root))
	if err != nil {
		if os.IsNotExist(err) {
			return lock, nil
		}
		return lock, fmt.Errorf("read workspace lock: %w", err)
	}
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return lock, fmt.Errorf("decode workspace lock: %w", err)
	}
	if lock.APIVersion == "" {
		lock.APIVersion = APIVersionV2Alpha1
	}
	if lock.Kind == "" {
		lock.Kind = KindWorkspaceLock
	}
	if len(lock.Tools) == 0 && len(lock.Providers) > 0 {
		lock.Tools, lock.Packages = legacyProvidersToLock(lock.Providers)
	}
	if len(lock.Providers) == 0 && (len(lock.Tools) > 0 || len(lock.Packages) > 0) {
		lock.Providers = compatibilityProviders(lock.Tools, lock.Packages)
	}
	return lock, nil
}

func compatibilityProviders(tools []LockedTool, packages []LockedPackage) []LockedProvider {
	if len(tools) == 0 {
		return nil
	}
	packagesByResolved := make(map[string]LockedPackage, len(packages))
	packagesByRefVersion := make(map[string]LockedPackage, len(packages))
	for _, pkg := range packages {
		if resolved := strings.TrimSpace(pkg.Package) + "@" + strings.TrimSpace(pkg.Version); strings.TrimSpace(pkg.Package) != "" && strings.TrimSpace(pkg.Version) != "" {
			packagesByResolved[resolved] = pkg
			packagesByRefVersion[resolved] = pkg
		}
	}
	compat := make([]LockedProvider, 0, len(tools))
	for _, tool := range tools {
		pkg := packagesByResolved[strings.TrimSpace(tool.ResolvedPackage)]
		version := pkg.Version
		if version == "" {
			_, version = splitResolvedPackage(tool.ResolvedPackage)
		}
		compat = append(compat, LockedProvider{
			Alias:    tool.Name,
			Provider: tool.Package,
			Source:   pkg.Source,
			Version:  version,
			Resolved: pkg.Resolved,
			Store:    pkg.ContentStore,
		})
	}
	return compat
}

func legacyProvidersToLock(providers []LockedProvider) ([]LockedTool, []LockedPackage) {
	if len(providers) == 0 {
		return nil, nil
	}
	tools := make([]LockedTool, 0, len(providers))
	packagesByResolved := make(map[string]LockedPackage, len(providers))
	for _, provider := range providers {
		resolvedPackage := strings.TrimSpace(provider.Provider)
		if strings.TrimSpace(provider.Version) != "" {
			resolvedPackage = resolvedPackage + "@" + strings.TrimSpace(provider.Version)
		}
		tools = append(tools, LockedTool{
			Name:            provider.Alias,
			Package:         provider.Provider,
			ResolvedPackage: resolvedPackage,
		})
		if _, ok := packagesByResolved[resolvedPackage]; !ok {
			packagesByResolved[resolvedPackage] = LockedPackage{
				Package:      provider.Provider,
				Version:      provider.Version,
				Source:       provider.Source,
				Resolved:     provider.Resolved,
				ContentStore: provider.Store,
			}
		}
	}
	packages := make([]LockedPackage, 0, len(packagesByResolved))
	for _, pkg := range packagesByResolved {
		packages = append(packages, pkg)
	}
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].Package < packages[j].Package
	})
	return tools, packages
}

func splitResolvedPackage(value string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(value), "@", 2)
	if len(parts) != 2 {
		return strings.TrimSpace(value), ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	tempFile, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tempFile.Chmod(mode); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
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
