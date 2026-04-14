package core

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

const (
	APIVersionV1    = "tinx.io/v1"
	KindProvider    = "Provider"
	KindTool        = "Tool"
	KindBundle      = "Bundle"
	KindAsset       = "Asset"
	KindEnvironment = "Environment"
	KindSecret      = "Secret"
	KindWorkspace   = "Workspace"

	RuntimeOCI    = "oci"
	RuntimeLocal  = "local"
	RuntimeScript = "script"

	SourceBundle = "bundle"
	SourceLocal  = "local"
	SourceScript = "script"
	SourceRemote = "remote"

	InstallLazy  = "lazy"
	InstallEager = "eager"
)

type Package struct {
	APIVersion   string                 `json:"apiVersion"`
	Provider     Provider               `json:"provider"`
	Tools        map[string]Tool        `json:"tools,omitempty"`
	Bundles      map[string]Bundle      `json:"bundles,omitempty"`
	Assets       map[string]Asset       `json:"assets,omitempty"`
	Environments map[string]Environment `json:"environments,omitempty"`
	Secrets      map[string]Secret      `json:"secrets,omitempty"`
	Workspaces   map[string]Workspace   `json:"workspaces,omitempty"`
}

type Metadata struct {
	Name        string `json:"name,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	Homepage    string `json:"homepage,omitempty"`
	License     string `json:"license,omitempty"`
}

type Provider struct {
	Metadata Metadata     `json:"metadata"`
	Spec     ProviderSpec `json:"spec"`
}

type ProviderSpec struct {
	Contents     []ContentRef        `json:"contents,omitempty"`
	Dependencies []ProviderReference `json:"dependencies,omitempty"`
}

type ContentRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type ProviderReference struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

type Tool struct {
	Metadata Metadata `json:"metadata"`
	Spec     ToolSpec `json:"spec"`
}

type ToolSpec struct {
	Default      bool                  `json:"default,omitempty"`
	Runtime      RuntimeSpec           `json:"runtime"`
	Source       SourceSpec            `json:"source"`
	Install      InstallSpec           `json:"install,omitempty"`
	Cache        CacheSpec             `json:"cache,omitempty"`
	Provides     []string              `json:"provides,omitempty"`
	DependsOn    []ToolDependency      `json:"dependsOn,omitempty"`
	Environments []string              `json:"environments,omitempty"`
	Capabilities map[string]Capability `json:"capabilities,omitempty"`
	Env          map[string]string     `json:"env,omitempty"`
	Path         []string              `json:"path,omitempty"`
}

type Capability struct {
	Description string `json:"description,omitempty"`
}

type RuntimeSpec struct {
	Type string `json:"type,omitempty"`
}

type SourceSpec struct {
	Type   string `json:"type,omitempty"`
	Ref    string `json:"ref,omitempty"`
	Script string `json:"script,omitempty"`
	Path   string `json:"path,omitempty"`
}

type InstallSpec struct {
	Strategy string `json:"strategy,omitempty"`
	Tool     string `json:"tool,omitempty"`
	Path     string `json:"path,omitempty"`
}

type CacheSpec struct {
	Key string `json:"key,omitempty"`
}

type ToolDependency struct {
	Tool string `json:"tool,omitempty"`
}

type Bundle struct {
	Metadata Metadata   `json:"metadata"`
	Spec     BundleSpec `json:"spec"`
}

type BundleSpec struct {
	Layers []BundleLayer `json:"layers,omitempty"`
}

type BundleLayer struct {
	Platform  PlatformSpec `json:"platform,omitempty"`
	MediaType string       `json:"mediaType,omitempty"`
	Source    string       `json:"source,omitempty"`
}

type PlatformSpec struct {
	OS   string `json:"os,omitempty"`
	Arch string `json:"arch,omitempty"`
}

type Asset struct {
	Metadata Metadata  `json:"metadata"`
	Spec     AssetSpec `json:"spec"`
}

type AssetSpec struct {
	Source SourceSpec `json:"source"`
	Mount  MountSpec  `json:"mount,omitempty"`
}

type MountSpec struct {
	Path string `json:"path,omitempty"`
}

type Environment struct {
	Metadata Metadata        `json:"metadata"`
	Spec     EnvironmentSpec `json:"spec"`
}

type EnvironmentSpec struct {
	Variables map[string]string `json:"variables,omitempty"`
	Export    []string          `json:"export,omitempty"`
	Path      []string          `json:"path,omitempty"`
}

type Secret struct {
	Metadata Metadata   `json:"metadata"`
	Spec     SecretSpec `json:"spec"`
}

type SecretSpec struct {
	Provider string            `json:"provider,omitempty"`
	Mapping  map[string]string `json:"mapping,omitempty"`
}

type Workspace struct {
	Metadata Metadata      `json:"metadata"`
	Spec     WorkspaceSpec `json:"spec"`
}

type WorkspaceSpec struct {
	Providers    []string `json:"providers,omitempty"`
	Tools        []string `json:"tools,omitempty"`
	Environments []string `json:"environments,omitempty"`
}

type PlatformSummary struct {
	OS   string
	Arch string
}

func (pkg *Package) Normalize() {
	if strings.TrimSpace(pkg.APIVersion) == "" {
		pkg.APIVersion = APIVersionV1
	}
	if pkg.Tools == nil {
		pkg.Tools = map[string]Tool{}
	}
	if pkg.Bundles == nil {
		pkg.Bundles = map[string]Bundle{}
	}
	if pkg.Assets == nil {
		pkg.Assets = map[string]Asset{}
	}
	if pkg.Environments == nil {
		pkg.Environments = map[string]Environment{}
	}
	if pkg.Secrets == nil {
		pkg.Secrets = map[string]Secret{}
	}
	if pkg.Workspaces == nil {
		pkg.Workspaces = map[string]Workspace{}
	}
	for name, tool := range pkg.Tools {
		tool.Metadata.Name = normalizeName(tool.Metadata.Name, name)
		tool.Spec.Runtime.Type = normalizeRuntimeType(tool.Spec.Runtime.Type, tool.Spec.Source.Type)
		tool.Spec.Source.Type = normalizeSourceType(tool.Spec.Source.Type, tool.Spec.Runtime.Type)
		tool.Spec.Install.Strategy = normalizeInstallStrategy(tool.Spec.Install.Strategy)
		tool.Spec.Install.Tool = strings.TrimSpace(tool.Spec.Install.Tool)
		tool.Spec.Install.Path = normalizeInstallPath(tool.Spec.Install.Path)
		tool.Spec.Provides = normalizeList(tool.Spec.Provides)
		if len(tool.Spec.Provides) == 0 {
			tool.Spec.Provides = []string{tool.Metadata.Name}
		}
		tool.Spec.Environments = normalizeList(tool.Spec.Environments)
		tool.Spec.Path = normalizeList(tool.Spec.Path)
		tool.Spec.Source.Ref = strings.TrimSpace(tool.Spec.Source.Ref)
		tool.Spec.Source.Script = strings.TrimSpace(tool.Spec.Source.Script)
		tool.Spec.Source.Path = strings.TrimSpace(tool.Spec.Source.Path)
		pkg.Tools[name] = tool
	}
	for name, bundle := range pkg.Bundles {
		bundle.Metadata.Name = normalizeName(bundle.Metadata.Name, name)
		for index := range bundle.Spec.Layers {
			bundle.Spec.Layers[index].Platform.OS = strings.TrimSpace(bundle.Spec.Layers[index].Platform.OS)
			bundle.Spec.Layers[index].Platform.Arch = strings.TrimSpace(bundle.Spec.Layers[index].Platform.Arch)
			bundle.Spec.Layers[index].Source = strings.TrimSpace(bundle.Spec.Layers[index].Source)
			bundle.Spec.Layers[index].MediaType = strings.TrimSpace(bundle.Spec.Layers[index].MediaType)
		}
		pkg.Bundles[name] = bundle
	}
	for name, asset := range pkg.Assets {
		asset.Metadata.Name = normalizeName(asset.Metadata.Name, name)
		asset.Spec.Source.Type = normalizeSourceType(asset.Spec.Source.Type, "")
		asset.Spec.Source.Ref = strings.TrimSpace(asset.Spec.Source.Ref)
		asset.Spec.Source.Path = strings.TrimSpace(asset.Spec.Source.Path)
		asset.Spec.Mount.Path = strings.TrimSpace(asset.Spec.Mount.Path)
		pkg.Assets[name] = asset
	}
	for name, env := range pkg.Environments {
		env.Metadata.Name = normalizeName(env.Metadata.Name, name)
		env.Spec.Export = normalizeList(env.Spec.Export)
		env.Spec.Path = normalizeList(env.Spec.Path)
		if env.Spec.Variables == nil {
			env.Spec.Variables = map[string]string{}
		}
		pkg.Environments[name] = env
	}
	for name, secret := range pkg.Secrets {
		secret.Metadata.Name = normalizeName(secret.Metadata.Name, name)
		secret.Spec.Provider = strings.TrimSpace(secret.Spec.Provider)
		if secret.Spec.Mapping == nil {
			secret.Spec.Mapping = map[string]string{}
		}
		pkg.Secrets[name] = secret
	}
	for name, workspace := range pkg.Workspaces {
		workspace.Metadata.Name = normalizeName(workspace.Metadata.Name, name)
		workspace.Spec.Providers = normalizeList(workspace.Spec.Providers)
		workspace.Spec.Tools = normalizeList(workspace.Spec.Tools)
		workspace.Spec.Environments = normalizeList(workspace.Spec.Environments)
		pkg.Workspaces[name] = workspace
	}
	pkg.Provider.Metadata.Name = strings.TrimSpace(pkg.Provider.Metadata.Name)
	pkg.Provider.Metadata.Namespace = strings.TrimSpace(pkg.Provider.Metadata.Namespace)
	pkg.Provider.Metadata.Version = strings.TrimSpace(pkg.Provider.Metadata.Version)
	pkg.Provider.Spec.Contents = normalizeContents(pkg.Provider.Spec.Contents)
	if len(pkg.Provider.Spec.Contents) == 0 {
		pkg.Provider.Spec.Contents = synthesizeContents(pkg)
	}
}

func (pkg Package) Validate() error {
	if pkg.APIVersion != APIVersionV1 {
		return fmt.Errorf("unsupported apiVersion %q", pkg.APIVersion)
	}
	if strings.TrimSpace(pkg.Provider.Metadata.Namespace) == "" {
		return fmt.Errorf("metadata.namespace is required")
	}
	if strings.TrimSpace(pkg.Provider.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if strings.TrimSpace(pkg.Provider.Metadata.Version) == "" {
		return fmt.Errorf("metadata.version is required")
	}
	for name, tool := range pkg.Tools {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("tool name is required")
		}
		if err := validateTool(tool, pkg); err != nil {
			return fmt.Errorf("tool %s: %w", name, err)
		}
	}
	for name, bundle := range pkg.Bundles {
		if err := validateBundle(bundle); err != nil {
			return fmt.Errorf("bundle %s: %w", name, err)
		}
	}
	for name, asset := range pkg.Assets {
		if strings.TrimSpace(asset.Spec.Source.Type) == "" {
			return fmt.Errorf("asset %s: source.type is required", name)
		}
	}
	for name, env := range pkg.Environments {
		for key := range env.Spec.Variables {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				return fmt.Errorf("environment %s: variable keys must not be empty", name)
			}
			if strings.HasPrefix(strings.ToUpper(trimmed), "TINX_") {
				return fmt.Errorf("environment %s: variable %q must not use reserved TINX_ prefix", name, trimmed)
			}
		}
	}
	for _, content := range pkg.Provider.Spec.Contents {
		if err := pkg.validateContent(content); err != nil {
			return err
		}
	}
	if len(pkg.Tools) == 0 {
		return fmt.Errorf("provider must define at least one tool")
	}
	if _, ok := pkg.DefaultTool(); !ok {
		return fmt.Errorf("provider must define a default tool")
	}
	return nil
}

func (pkg Package) ProviderRef() string {
	return strings.TrimSpace(pkg.Provider.Metadata.Namespace) + "/" + strings.TrimSpace(pkg.Provider.Metadata.Name)
}

func (pkg Package) DefaultTool() (Tool, bool) {
	tools := pkg.SortedTools()
	for _, tool := range tools {
		if tool.Spec.Default {
			return tool, true
		}
	}
	if tool, ok := pkg.Tools[pkg.Provider.Metadata.Name]; ok {
		return tool, true
	}
	if len(tools) == 0 {
		return Tool{}, false
	}
	return tools[0], true
}

func (pkg Package) Tool(name string) (Tool, bool) {
	tool, ok := pkg.Tools[strings.TrimSpace(name)]
	return tool, ok
}

func (pkg Package) ToolProviding(command string) (Tool, bool) {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return Tool{}, false
	}
	for _, tool := range pkg.SortedTools() {
		for _, provided := range tool.Spec.Provides {
			if provided == trimmed {
				return tool, true
			}
		}
	}
	return Tool{}, false
}

func (pkg Package) SortedTools() []Tool {
	items := make([]Tool, 0, len(pkg.Tools))
	for _, tool := range pkg.Tools {
		items = append(items, tool)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Metadata.Name < items[j].Metadata.Name
	})
	return items
}

func (pkg Package) SortedEnvironments() []Environment {
	items := make([]Environment, 0, len(pkg.Environments))
	for _, env := range pkg.Environments {
		items = append(items, env)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Metadata.Name < items[j].Metadata.Name
	})
	return items
}

func (pkg Package) PlatformSummaries() []PlatformSummary {
	seen := map[string]struct{}{}
	items := make([]PlatformSummary, 0)
	for _, bundle := range pkg.Bundles {
		for _, layer := range bundle.Spec.Layers {
			if strings.TrimSpace(layer.Platform.OS) == "" || strings.TrimSpace(layer.Platform.Arch) == "" {
				continue
			}
			key := layer.Platform.OS + "/" + layer.Platform.Arch
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			items = append(items, PlatformSummary{OS: layer.Platform.OS, Arch: layer.Platform.Arch})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].OS == items[j].OS {
			return items[i].Arch < items[j].Arch
		}
		return items[i].OS < items[j].OS
	})
	return items
}

func (tool Tool) CapabilityNames() []string {
	items := make([]string, 0, len(tool.Spec.Capabilities))
	for name := range tool.Spec.Capabilities {
		items = append(items, name)
	}
	sort.Strings(items)
	return items
}

func (tool Tool) PrimaryProvide() string {
	if len(tool.Spec.Provides) == 0 {
		return strings.TrimSpace(tool.Metadata.Name)
	}
	return strings.TrimSpace(tool.Spec.Provides[0])
}

func (tool Tool) InstallPath() string {
	if installPath := strings.TrimSpace(tool.Spec.Install.Path); installPath != "" {
		return normalizeInstallPath(installPath)
	}
	name := tool.PrimaryProvide()
	if name == "" {
		name = strings.TrimSpace(tool.Metadata.Name)
	}
	if name == "" {
		return ""
	}
	return path.Join("bin", name)
}

func (pkg Package) validateContent(content ContentRef) error {
	name := strings.TrimSpace(content.Name)
	if name == "" {
		return fmt.Errorf("provider content name is required")
	}
	switch strings.TrimSpace(content.Kind) {
	case KindTool:
		if _, ok := pkg.Tools[name]; !ok {
			return fmt.Errorf("provider content references unknown tool %q", name)
		}
	case KindBundle:
		if _, ok := pkg.Bundles[name]; !ok {
			return fmt.Errorf("provider content references unknown bundle %q", name)
		}
	case KindAsset:
		if _, ok := pkg.Assets[name]; !ok {
			return fmt.Errorf("provider content references unknown asset %q", name)
		}
	case KindEnvironment:
		if _, ok := pkg.Environments[name]; !ok {
			return fmt.Errorf("provider content references unknown environment %q", name)
		}
	case KindSecret:
		if _, ok := pkg.Secrets[name]; !ok {
			return fmt.Errorf("provider content references unknown secret %q", name)
		}
	case KindWorkspace:
		if _, ok := pkg.Workspaces[name]; !ok {
			return fmt.Errorf("provider content references unknown workspace %q", name)
		}
	default:
		return fmt.Errorf("provider content uses unsupported kind %q", content.Kind)
	}
	return nil
}

func validateTool(tool Tool, pkg Package) error {
	installerTool := strings.TrimSpace(tool.Spec.Install.Tool)
	if strings.TrimSpace(tool.Spec.Runtime.Type) == "" {
		return fmt.Errorf("runtime.type is required")
	}
	if strings.TrimSpace(tool.Spec.Source.Type) == "" {
		return fmt.Errorf("source.type is required")
	}
	switch tool.Spec.Runtime.Type {
	case RuntimeOCI, RuntimeLocal, RuntimeScript:
	default:
		return fmt.Errorf("unsupported runtime %q", tool.Spec.Runtime.Type)
	}
	switch tool.Spec.Source.Type {
	case SourceBundle:
		if strings.TrimSpace(tool.Spec.Source.Ref) == "" {
			return fmt.Errorf("source.ref is required for bundle sources")
		}
		if _, ok := pkg.Bundles[tool.Spec.Source.Ref]; !ok {
			return fmt.Errorf("source bundle %q not found", tool.Spec.Source.Ref)
		}
	case SourceScript:
		if strings.TrimSpace(tool.Spec.Source.Script) == "" {
			return fmt.Errorf("source.script is required for script sources")
		}
	case SourceLocal:
		if strings.TrimSpace(tool.Spec.Source.Path) == "" && strings.TrimSpace(tool.Spec.Source.Ref) == "" && installerTool == "" {
			return fmt.Errorf("source.path or source.ref is required for local sources unless install.tool is declared")
		}
	case SourceRemote:
		if strings.TrimSpace(tool.Spec.Source.Path) == "" && strings.TrimSpace(tool.Spec.Source.Ref) == "" {
			return fmt.Errorf("source.path or source.ref is required for %s sources", tool.Spec.Source.Type)
		}
	default:
		return fmt.Errorf("unsupported source type %q", tool.Spec.Source.Type)
	}
	for _, dependency := range tool.Spec.DependsOn {
		if strings.TrimSpace(dependency.Tool) == "" {
			return fmt.Errorf("dependsOn entries must declare tool")
		}
		if _, ok := pkg.Tools[dependency.Tool]; !ok {
			return fmt.Errorf("dependsOn references unknown tool %q", dependency.Tool)
		}
	}
	if installerTool != "" {
		if _, ok := pkg.Tools[installerTool]; !ok {
			return fmt.Errorf("install.tool references unknown tool %q", installerTool)
		}
		if installerTool == tool.Metadata.Name {
			return fmt.Errorf("install.tool %q must not reference the tool itself", installerTool)
		}
		if !toolDependsOn(tool, installerTool) {
			return fmt.Errorf("install.tool %q must also appear in dependsOn", installerTool)
		}
	}
	if installPath := strings.TrimSpace(tool.InstallPath()); installPath != "" {
		if path.IsAbs(installPath) || installPath == "." || installPath == ".." || strings.HasPrefix(installPath, "../") {
			return fmt.Errorf("install.path must be a relative path inside the tool install dir")
		}
	}
	for _, envName := range tool.Spec.Environments {
		if _, ok := pkg.Environments[envName]; !ok {
			return fmt.Errorf("references unknown environment %q", envName)
		}
	}
	return nil
}

func toolDependsOn(tool Tool, name string) bool {
	for _, dependency := range tool.Spec.DependsOn {
		if strings.TrimSpace(dependency.Tool) == name {
			return true
		}
	}
	return false
}

func validateBundle(bundle Bundle) error {
	if len(bundle.Spec.Layers) == 0 {
		return fmt.Errorf("spec.layers must declare at least one layer")
	}
	seen := map[string]struct{}{}
	for _, layer := range bundle.Spec.Layers {
		if strings.TrimSpace(layer.Source) == "" {
			return fmt.Errorf("bundle layers require source")
		}
		if strings.TrimSpace(layer.Platform.OS) == "" || strings.TrimSpace(layer.Platform.Arch) == "" {
			return fmt.Errorf("bundle layers require platform.os and platform.arch")
		}
		key := layer.Platform.OS + "/" + layer.Platform.Arch + ":" + strings.TrimSpace(layer.Source)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate bundle layer %s", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func normalizeRuntimeType(runtimeType, sourceType string) string {
	switch strings.TrimSpace(runtimeType) {
	case "", "binary":
		switch strings.TrimSpace(sourceType) {
		case SourceScript:
			return RuntimeScript
		case SourceLocal:
			return RuntimeLocal
		default:
			return RuntimeOCI
		}
	default:
		return strings.TrimSpace(runtimeType)
	}
}

func normalizeSourceType(sourceType, runtimeType string) string {
	trimmed := strings.TrimSpace(sourceType)
	if trimmed != "" {
		return trimmed
	}
	switch strings.TrimSpace(runtimeType) {
	case RuntimeScript:
		return SourceScript
	case RuntimeLocal:
		return SourceLocal
	default:
		return SourceBundle
	}
}

func normalizeInstallStrategy(strategy string) string {
	trimmed := strings.TrimSpace(strategy)
	if trimmed == "" {
		return InstallLazy
	}
	return trimmed
}

func normalizeContents(contents []ContentRef) []ContentRef {
	result := make([]ContentRef, 0, len(contents))
	seen := map[string]struct{}{}
	for _, content := range contents {
		kind := strings.TrimSpace(content.Kind)
		name := strings.TrimSpace(content.Name)
		if kind == "" || name == "" {
			continue
		}
		key := kind + "/" + name
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, ContentRef{Kind: kind, Name: name})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind == result[j].Kind {
			return result[i].Name < result[j].Name
		}
		return result[i].Kind < result[j].Kind
	})
	return result
}

func synthesizeContents(pkg *Package) []ContentRef {
	contents := make([]ContentRef, 0, len(pkg.Tools)+len(pkg.Bundles)+len(pkg.Assets)+len(pkg.Environments)+len(pkg.Secrets)+len(pkg.Workspaces))
	for name := range pkg.Tools {
		contents = append(contents, ContentRef{Kind: KindTool, Name: name})
	}
	for name := range pkg.Bundles {
		contents = append(contents, ContentRef{Kind: KindBundle, Name: name})
	}
	for name := range pkg.Assets {
		contents = append(contents, ContentRef{Kind: KindAsset, Name: name})
	}
	for name := range pkg.Environments {
		contents = append(contents, ContentRef{Kind: KindEnvironment, Name: name})
	}
	for name := range pkg.Secrets {
		contents = append(contents, ContentRef{Kind: KindSecret, Name: name})
	}
	for name := range pkg.Workspaces {
		contents = append(contents, ContentRef{Kind: KindWorkspace, Name: name})
	}
	return normalizeContents(contents)
}

func normalizeList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
}

func normalizeName(name, fallback string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(fallback)
}

func normalizeInstallPath(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	normalized := path.Clean(strings.ReplaceAll(trimmed, "\\", "/"))
	if normalized == "." {
		return ""
	}
	return normalized
}
