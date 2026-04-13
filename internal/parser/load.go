package parser

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sourceplane/tinx/internal/core"
)

func Load(path string) (core.Package, error) {
	var pkg core.Package
	data, err := os.ReadFile(path)
	if err != nil {
		return pkg, fmt.Errorf("read manifest: %w", err)
	}
	pkg, err = LoadBytes(data)
	if err != nil {
		return pkg, err
	}
	return pkg, nil
}

func LoadBytes(data []byte) (core.Package, error) {
	var pkg core.Package
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	providerSeen := false
	for {
		var node yaml.Node
		if err := decoder.Decode(&node); err != nil {
			if err.Error() == "EOF" {
				break
			}
			return pkg, fmt.Errorf("decode manifest: %w", err)
		}
		if isEmptyDocument(&node) {
			continue
		}
		kind, err := nodeKind(&node)
		if err != nil {
			return pkg, err
		}
		switch kind {
		case core.KindProvider:
			if providerSeen {
				return pkg, fmt.Errorf("manifest must declare exactly one provider document")
			}
			providerSeen = true
			var doc rawProviderDocument
			if err := node.Decode(&doc); err != nil {
				return pkg, fmt.Errorf("decode provider document: %w", err)
			}
			if err := applyProviderDocument(&pkg, doc); err != nil {
				return pkg, err
			}
		case core.KindTool:
			var doc rawToolDocument
			if err := node.Decode(&doc); err != nil {
				return pkg, fmt.Errorf("decode tool document: %w", err)
			}
			tool := normalizeToolDocument(doc.Metadata, doc.Spec)
			if err := addTool(&pkg, tool); err != nil {
				return pkg, err
			}
		case core.KindBundle:
			var doc rawBundleDocument
			if err := node.Decode(&doc); err != nil {
				return pkg, fmt.Errorf("decode bundle document: %w", err)
			}
			bundle := normalizeBundleDocument(doc.Metadata, doc.Spec)
			if err := addBundle(&pkg, bundle); err != nil {
				return pkg, err
			}
		case core.KindAsset:
			var doc rawAssetDocument
			if err := node.Decode(&doc); err != nil {
				return pkg, fmt.Errorf("decode asset document: %w", err)
			}
			asset := normalizeAssetDocument(doc.Metadata, doc.Spec)
			if err := addAsset(&pkg, asset); err != nil {
				return pkg, err
			}
		case core.KindEnvironment:
			var doc rawEnvironmentDocument
			if err := node.Decode(&doc); err != nil {
				return pkg, fmt.Errorf("decode environment document: %w", err)
			}
			env := normalizeEnvironmentDocument(doc.Metadata, doc.Spec)
			if err := addEnvironment(&pkg, env); err != nil {
				return pkg, err
			}
		case core.KindSecret:
			var doc rawSecretDocument
			if err := node.Decode(&doc); err != nil {
				return pkg, fmt.Errorf("decode secret document: %w", err)
			}
			secret := normalizeSecretDocument(doc.Metadata, doc.Spec)
			if err := addSecret(&pkg, secret); err != nil {
				return pkg, err
			}
		case core.KindWorkspace:
			var doc rawWorkspaceDocument
			if err := node.Decode(&doc); err != nil {
				return pkg, fmt.Errorf("decode workspace document: %w", err)
			}
			workspace := normalizeWorkspaceDocument(doc.Metadata, doc.Spec)
			if err := addWorkspace(&pkg, workspace); err != nil {
				return pkg, err
			}
		default:
			return pkg, fmt.Errorf("unsupported kind %q", kind)
		}
	}
	if !providerSeen {
		return pkg, fmt.Errorf("manifest must declare a provider document")
	}
	inheritResourceMetadata(&pkg)
	pkg.Normalize()
	if err := pkg.Validate(); err != nil {
		return pkg, err
	}
	return pkg, nil
}

type rawProviderDocument struct {
	APIVersion string          `yaml:"apiVersion"`
	Kind       string          `yaml:"kind"`
	Metadata   rawMetadata     `yaml:"metadata"`
	Spec       rawProviderSpec `yaml:"spec"`
}

type rawProviderSpec struct {
	Contents     []rawContentRef          `yaml:"contents,omitempty"`
	Dependencies []core.ProviderReference `yaml:"dependencies,omitempty"`
	Tools        []rawInlineTool          `yaml:"tools,omitempty"`
	Assets       []rawInlineAsset         `yaml:"assets,omitempty"`
	Environments []rawInlineEnvironment   `yaml:"environments,omitempty"`
	Secrets      []rawInlineSecret        `yaml:"secrets,omitempty"`
	Workspaces   []rawInlineWorkspace     `yaml:"workspaces,omitempty"`
	Bundle       []rawInlineBundle        `yaml:"bundle,omitempty"`
	Bundles      []rawInlineBundle        `yaml:"bundles,omitempty"`

	Runtime      string                     `yaml:"runtime,omitempty"`
	Entrypoint   string                     `yaml:"entrypoint,omitempty"`
	Platforms    []rawLegacyPlatform        `yaml:"platforms,omitempty"`
	Env          map[string]string          `yaml:"env,omitempty"`
	Path         []string                   `yaml:"path,omitempty"`
	Layers       rawLegacyLayers            `yaml:"layers,omitempty"`
	Capabilities map[string]core.Capability `yaml:"capabilities,omitempty"`
}

type rawToolDocument struct {
	APIVersion string      `yaml:"apiVersion"`
	Kind       string      `yaml:"kind"`
	Metadata   rawMetadata `yaml:"metadata"`
	Spec       rawToolSpec `yaml:"spec"`
}

type rawBundleDocument struct {
	APIVersion string        `yaml:"apiVersion"`
	Kind       string        `yaml:"kind"`
	Metadata   rawMetadata   `yaml:"metadata"`
	Spec       rawBundleSpec `yaml:"spec"`
}

type rawAssetDocument struct {
	APIVersion string       `yaml:"apiVersion"`
	Kind       string       `yaml:"kind"`
	Metadata   rawMetadata  `yaml:"metadata"`
	Spec       rawAssetSpec `yaml:"spec"`
}

type rawEnvironmentDocument struct {
	APIVersion string             `yaml:"apiVersion"`
	Kind       string             `yaml:"kind"`
	Metadata   rawMetadata        `yaml:"metadata"`
	Spec       rawEnvironmentSpec `yaml:"spec"`
}

type rawSecretDocument struct {
	APIVersion string        `yaml:"apiVersion"`
	Kind       string        `yaml:"kind"`
	Metadata   rawMetadata   `yaml:"metadata"`
	Spec       rawSecretSpec `yaml:"spec"`
}

type rawWorkspaceDocument struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   rawMetadata      `yaml:"metadata"`
	Spec       rawWorkspaceSpec `yaml:"spec"`
}

type rawMetadata struct {
	Name        string `yaml:"name,omitempty"`
	Namespace   string `yaml:"namespace,omitempty"`
	Version     string `yaml:"version,omitempty"`
	Description string `yaml:"description,omitempty"`
	Homepage    string `yaml:"homepage,omitempty"`
	License     string `yaml:"license,omitempty"`
}

type rawToolSpec struct {
	Default      bool                       `yaml:"default,omitempty"`
	Description  string                     `yaml:"description,omitempty"`
	Runtime      rawRuntime                 `yaml:"runtime,omitempty"`
	Source       rawSource                  `yaml:"source,omitempty"`
	From         string                     `yaml:"from,omitempty"`
	Script       string                     `yaml:"script,omitempty"`
	Install      core.InstallSpec           `yaml:"install,omitempty"`
	Cache        core.CacheSpec             `yaml:"cache,omitempty"`
	Provides     []string                   `yaml:"provides,omitempty"`
	DependsOn    rawToolDependencies        `yaml:"dependsOn,omitempty"`
	Environments []string                   `yaml:"environments,omitempty"`
	Capabilities map[string]core.Capability `yaml:"capabilities,omitempty"`
	Env          map[string]string          `yaml:"env,omitempty"`
	Path         []string                   `yaml:"path,omitempty"`
}

type rawBundleSpec struct {
	Layers    []rawBundleLayer    `yaml:"layers,omitempty"`
	Platforms []rawBundlePlatform `yaml:"platforms,omitempty"`
	Type      string              `yaml:"type,omitempty"`
}

type rawAssetSpec struct {
	Source rawSource      `yaml:"source,omitempty"`
	From   string         `yaml:"from,omitempty"`
	Mount  core.MountSpec `yaml:"mount,omitempty"`
}

type rawEnvironmentSpec struct {
	Variables   map[string]string `yaml:"variables,omitempty"`
	Export      []string          `yaml:"export,omitempty"`
	Path        []string          `yaml:"path,omitempty"`
	Description string            `yaml:"description,omitempty"`
}

type rawSecretSpec struct {
	Provider    string            `yaml:"provider,omitempty"`
	Mapping     map[string]string `yaml:"mapping,omitempty"`
	Description string            `yaml:"description,omitempty"`
}

type rawWorkspaceSpec struct {
	Providers    []string `yaml:"providers,omitempty"`
	Tools        []string `yaml:"tools,omitempty"`
	Environments []string `yaml:"environments,omitempty"`
	Description  string   `yaml:"description,omitempty"`
}

type rawInlineTool struct {
	Name        string `yaml:"name"`
	rawToolSpec `yaml:",inline"`
}

type rawInlineBundle struct {
	Name          string `yaml:"name"`
	rawBundleSpec `yaml:",inline"`
}

type rawInlineAsset struct {
	Name         string `yaml:"name"`
	Description  string `yaml:"description,omitempty"`
	rawAssetSpec `yaml:",inline"`
}

type rawInlineEnvironment struct {
	Name               string `yaml:"name"`
	rawEnvironmentSpec `yaml:",inline"`
}

type rawInlineSecret struct {
	Name          string `yaml:"name"`
	rawSecretSpec `yaml:",inline"`
}

type rawInlineWorkspace struct {
	Name             string `yaml:"name"`
	rawWorkspaceSpec `yaml:",inline"`
}

type rawRuntime struct {
	Type string `yaml:"type,omitempty"`
}

func (runtime *rawRuntime) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		runtime.Type = strings.TrimSpace(node.Value)
		return nil
	case yaml.MappingNode:
		type alias rawRuntime
		var value alias
		if err := node.Decode(&value); err != nil {
			return err
		}
		*runtime = rawRuntime(value)
		return nil
	default:
		return fmt.Errorf("runtime must be a string or mapping")
	}
}

type rawSource struct {
	Type   string `yaml:"type,omitempty"`
	Ref    string `yaml:"ref,omitempty"`
	Script string `yaml:"script,omitempty"`
	Path   string `yaml:"path,omitempty"`
}

func (source *rawSource) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case 0:
		return nil
	case yaml.ScalarNode:
		source.Type = strings.TrimSpace(node.Value)
		return nil
	case yaml.MappingNode:
		type alias rawSource
		var value alias
		if err := node.Decode(&value); err != nil {
			return err
		}
		*source = rawSource(value)
		return nil
	default:
		return fmt.Errorf("source must be a string or mapping")
	}
}

type rawToolDependencies []core.ToolDependency

func (deps *rawToolDependencies) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == 0 {
		return nil
	}
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("dependsOn must be a sequence")
	}
	items := make([]core.ToolDependency, 0, len(node.Content))
	for _, child := range node.Content {
		switch child.Kind {
		case yaml.ScalarNode:
			items = append(items, core.ToolDependency{Tool: strings.TrimSpace(child.Value)})
		case yaml.MappingNode:
			var dep struct {
				Tool string `yaml:"tool"`
			}
			if err := child.Decode(&dep); err != nil {
				return err
			}
			items = append(items, core.ToolDependency{Tool: strings.TrimSpace(dep.Tool)})
		default:
			return fmt.Errorf("dependsOn entries must be strings or mappings")
		}
	}
	*deps = rawToolDependencies(items)
	return nil
}

type rawContentRef struct {
	Kind string
	Name string
}

func (content *rawContentRef) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("provider contents entries must be mappings")
	}
	if len(node.Content) == 2 {
		key := strings.TrimSpace(node.Content[0].Value)
		value := strings.TrimSpace(node.Content[1].Value)
		if key != "kind" && key != "name" {
			content.Kind = key
			content.Name = value
			return nil
		}
	}
	var value struct {
		Kind string `yaml:"kind"`
		Name string `yaml:"name"`
	}
	if err := node.Decode(&value); err != nil {
		return err
	}
	content.Kind = strings.TrimSpace(value.Kind)
	content.Name = strings.TrimSpace(value.Name)
	return nil
}

type rawLegacyPlatform struct {
	OS     string `yaml:"os"`
	Arch   string `yaml:"arch"`
	Binary string `yaml:"binary"`
}

type rawLegacyLayers struct {
	Assets struct {
		Root string `yaml:"root,omitempty"`
	} `yaml:"assets,omitempty"`
}

type rawBundlePlatform struct {
	OS        string `yaml:"os"`
	Arch      string `yaml:"arch"`
	Source    string `yaml:"source"`
	MediaType string `yaml:"mediaType,omitempty"`
}

type rawBundleLayer struct {
	Platform  core.PlatformSpec `yaml:"platform"`
	MediaType string            `yaml:"mediaType,omitempty"`
	Source    string            `yaml:"source,omitempty"`
}

func applyProviderDocument(pkg *core.Package, doc rawProviderDocument) error {
	pkg.APIVersion = strings.TrimSpace(doc.APIVersion)
	pkg.Provider = core.Provider{
		Metadata: metadataFromRaw(doc.Metadata),
		Spec: core.ProviderSpec{
			Contents:     contentsFromRaw(doc.Spec.Contents),
			Dependencies: doc.Spec.Dependencies,
		},
	}
	if isLegacyProvider(doc.Spec) {
		if err := normalizeLegacyProvider(pkg, doc); err != nil {
			return err
		}
	}
	for _, tool := range doc.Spec.Tools {
		item := normalizeInlineTool(tool)
		if err := addTool(pkg, item); err != nil {
			return err
		}
	}
	for _, bundle := range append([]rawInlineBundle(nil), doc.Spec.Bundle...) {
		item := normalizeInlineBundle(bundle)
		if err := addBundle(pkg, item); err != nil {
			return err
		}
	}
	for _, bundle := range doc.Spec.Bundles {
		item := normalizeInlineBundle(bundle)
		if err := addBundle(pkg, item); err != nil {
			return err
		}
	}
	for _, asset := range doc.Spec.Assets {
		item := normalizeInlineAsset(asset)
		if err := addAsset(pkg, item); err != nil {
			return err
		}
	}
	for _, env := range doc.Spec.Environments {
		item := normalizeInlineEnvironment(env)
		if err := addEnvironment(pkg, item); err != nil {
			return err
		}
	}
	for _, secret := range doc.Spec.Secrets {
		item := normalizeInlineSecret(secret)
		if err := addSecret(pkg, item); err != nil {
			return err
		}
	}
	for _, workspace := range doc.Spec.Workspaces {
		item := normalizeInlineWorkspace(workspace)
		if err := addWorkspace(pkg, item); err != nil {
			return err
		}
	}
	return nil
}

func normalizeLegacyProvider(pkg *core.Package, doc rawProviderDocument) error {
	bundleName := doc.Metadata.Name
	toolName := doc.Metadata.Name
	envName := doc.Metadata.Name
	bundle := core.Bundle{
		Metadata: core.Metadata{Name: bundleName},
		Spec:     core.BundleSpec{Layers: make([]core.BundleLayer, 0, len(doc.Spec.Platforms))},
	}
	for _, platform := range doc.Spec.Platforms {
		bundle.Spec.Layers = append(bundle.Spec.Layers, core.BundleLayer{
			Platform:  core.PlatformSpec{OS: strings.TrimSpace(platform.OS), Arch: strings.TrimSpace(platform.Arch)},
			MediaType: "application/vnd.tinx.tool.binary",
			Source:    filepath.ToSlash(strings.TrimSpace(platform.Binary)),
		})
	}
	if assetsRoot := strings.TrimSpace(doc.Spec.Layers.Assets.Root); assetsRoot != "" {
		bundle.Spec.Layers = append(bundle.Spec.Layers, core.BundleLayer{
			Platform:  core.PlatformSpec{OS: "any", Arch: "any"},
			MediaType: "application/vnd.tinx.asset.layer.v1+tar",
			Source:    filepath.ToSlash(assetsRoot),
		})
	}
	if err := addBundle(pkg, bundle); err != nil {
		return err
	}
	tool := core.Tool{
		Metadata: core.Metadata{Name: toolName, Description: doc.Metadata.Description},
		Spec: core.ToolSpec{
			Default:      true,
			Runtime:      core.RuntimeSpec{Type: legacyRuntimeType(doc.Spec.Runtime)},
			Source:       core.SourceSpec{Type: core.SourceBundle, Ref: bundleName},
			Install:      core.InstallSpec{Strategy: core.InstallLazy},
			Provides:     []string{strings.TrimSpace(doc.Spec.Entrypoint)},
			Capabilities: doc.Spec.Capabilities,
			Environments: []string{envName},
		},
	}
	if err := addTool(pkg, tool); err != nil {
		return err
	}
	env := core.Environment{
		Metadata: core.Metadata{Name: envName, Description: doc.Metadata.Description},
		Spec: core.EnvironmentSpec{
			Variables: copyStringMap(doc.Spec.Env),
			Export:    mapKeys(doc.Spec.Env),
			Path:      append([]string(nil), doc.Spec.Path...),
		},
	}
	if err := addEnvironment(pkg, env); err != nil {
		return err
	}
	if assetsRoot := strings.TrimSpace(doc.Spec.Layers.Assets.Root); assetsRoot != "" {
		if err := addAsset(pkg, core.Asset{
			Metadata: core.Metadata{Name: bundleName + "-assets"},
			Spec: core.AssetSpec{
				Source: core.SourceSpec{Type: core.SourceBundle, Ref: bundleName},
				Mount:  core.MountSpec{Path: filepath.ToSlash(assetsRoot)},
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func normalizeToolDocument(metadata rawMetadata, spec rawToolSpec) core.Tool {
	tool := core.Tool{Metadata: metadataFromRaw(metadata)}
	tool.Metadata.Description = firstNonEmpty(tool.Metadata.Description, spec.Description)
	tool.Spec = normalizeToolSpec(spec, tool.Metadata.Name)
	return tool
}

func normalizeInlineTool(raw rawInlineTool) core.Tool {
	tool := normalizeToolDocument(rawMetadata{Name: raw.Name, Description: raw.Description}, raw.rawToolSpec)
	tool.Metadata.Name = strings.TrimSpace(raw.Name)
	return tool
}

func normalizeToolSpec(spec rawToolSpec, fallbackName string) core.ToolSpec {
	source := spec.Source
	if strings.TrimSpace(spec.From) != "" {
		applyFromShortcut(&source, spec.From)
	}
	if strings.TrimSpace(spec.Script) != "" && strings.TrimSpace(source.Script) == "" {
		source.Type = core.SourceScript
		source.Script = spec.Script
	}
	provides := append([]string(nil), spec.Provides...)
	if len(provides) == 0 && strings.TrimSpace(fallbackName) != "" {
		provides = []string{strings.TrimSpace(fallbackName)}
	}
	return core.ToolSpec{
		Default:      spec.Default,
		Runtime:      core.RuntimeSpec{Type: strings.TrimSpace(spec.Runtime.Type)},
		Source:       core.SourceSpec{Type: strings.TrimSpace(source.Type), Ref: strings.TrimSpace(source.Ref), Script: strings.TrimSpace(source.Script), Path: filepath.ToSlash(strings.TrimSpace(source.Path))},
		Install:      spec.Install,
		Cache:        spec.Cache,
		Provides:     provides,
		DependsOn:    []core.ToolDependency(spec.DependsOn),
		Environments: append([]string(nil), spec.Environments...),
		Capabilities: spec.Capabilities,
		Env:          copyStringMap(spec.Env),
		Path:         normalizePaths(spec.Path),
	}
}

func normalizeBundleDocument(metadata rawMetadata, spec rawBundleSpec) core.Bundle {
	bundle := core.Bundle{Metadata: metadataFromRaw(metadata)}
	bundle.Spec = normalizeBundleSpec(spec)
	return bundle
}

func normalizeInlineBundle(raw rawInlineBundle) core.Bundle {
	bundle := normalizeBundleDocument(rawMetadata{Name: raw.Name}, raw.rawBundleSpec)
	bundle.Metadata.Name = strings.TrimSpace(raw.Name)
	return bundle
}

func normalizeBundleSpec(spec rawBundleSpec) core.BundleSpec {
	layers := make([]core.BundleLayer, 0, len(spec.Layers)+len(spec.Platforms))
	for _, layer := range spec.Layers {
		layers = append(layers, core.BundleLayer{Platform: layer.Platform, MediaType: strings.TrimSpace(layer.MediaType), Source: filepath.ToSlash(strings.TrimSpace(layer.Source))})
	}
	for _, platform := range spec.Platforms {
		mediaType := strings.TrimSpace(platform.MediaType)
		if mediaType == "" {
			mediaType = defaultBundleMediaType(spec.Type)
		}
		layers = append(layers, core.BundleLayer{
			Platform:  core.PlatformSpec{OS: strings.TrimSpace(platform.OS), Arch: strings.TrimSpace(platform.Arch)},
			MediaType: mediaType,
			Source:    filepath.ToSlash(strings.TrimSpace(platform.Source)),
		})
	}
	return core.BundleSpec{Layers: layers}
}

func normalizeAssetDocument(metadata rawMetadata, spec rawAssetSpec) core.Asset {
	asset := core.Asset{Metadata: metadataFromRaw(metadata)}
	asset.Spec = normalizeAssetSpec(spec)
	return asset
}

func normalizeInlineAsset(raw rawInlineAsset) core.Asset {
	asset := normalizeAssetDocument(rawMetadata{Name: raw.Name, Description: raw.Description}, raw.rawAssetSpec)
	asset.Metadata.Name = strings.TrimSpace(raw.Name)
	asset.Metadata.Description = firstNonEmpty(asset.Metadata.Description, raw.Description)
	return asset
}

func normalizeAssetSpec(spec rawAssetSpec) core.AssetSpec {
	source := spec.Source
	if strings.TrimSpace(spec.From) != "" {
		applyFromShortcut(&source, spec.From)
	}
	return core.AssetSpec{
		Source: core.SourceSpec{Type: strings.TrimSpace(source.Type), Ref: strings.TrimSpace(source.Ref), Script: strings.TrimSpace(source.Script), Path: filepath.ToSlash(strings.TrimSpace(source.Path))},
		Mount:  spec.Mount,
	}
}

func normalizeEnvironmentDocument(metadata rawMetadata, spec rawEnvironmentSpec) core.Environment {
	env := core.Environment{Metadata: metadataFromRaw(metadata)}
	env.Metadata.Description = firstNonEmpty(env.Metadata.Description, spec.Description)
	env.Spec = core.EnvironmentSpec{Variables: copyStringMap(spec.Variables), Export: append([]string(nil), spec.Export...), Path: normalizePaths(spec.Path)}
	return env
}

func normalizeInlineEnvironment(raw rawInlineEnvironment) core.Environment {
	env := normalizeEnvironmentDocument(rawMetadata{Name: raw.Name, Description: raw.Description}, raw.rawEnvironmentSpec)
	env.Metadata.Name = strings.TrimSpace(raw.Name)
	return env
}

func normalizeSecretDocument(metadata rawMetadata, spec rawSecretSpec) core.Secret {
	secret := core.Secret{Metadata: metadataFromRaw(metadata)}
	secret.Metadata.Description = firstNonEmpty(secret.Metadata.Description, spec.Description)
	secret.Spec = core.SecretSpec{Provider: strings.TrimSpace(spec.Provider), Mapping: copyStringMap(spec.Mapping)}
	return secret
}

func normalizeInlineSecret(raw rawInlineSecret) core.Secret {
	secret := normalizeSecretDocument(rawMetadata{Name: raw.Name}, raw.rawSecretSpec)
	secret.Metadata.Name = strings.TrimSpace(raw.Name)
	return secret
}

func normalizeWorkspaceDocument(metadata rawMetadata, spec rawWorkspaceSpec) core.Workspace {
	workspace := core.Workspace{Metadata: metadataFromRaw(metadata)}
	workspace.Metadata.Description = firstNonEmpty(workspace.Metadata.Description, spec.Description)
	workspace.Spec = core.WorkspaceSpec{Providers: append([]string(nil), spec.Providers...), Tools: append([]string(nil), spec.Tools...), Environments: append([]string(nil), spec.Environments...)}
	return workspace
}

func normalizeInlineWorkspace(raw rawInlineWorkspace) core.Workspace {
	workspace := normalizeWorkspaceDocument(rawMetadata{Name: raw.Name}, raw.rawWorkspaceSpec)
	workspace.Metadata.Name = strings.TrimSpace(raw.Name)
	return workspace
}

func metadataFromRaw(metadata rawMetadata) core.Metadata {
	return core.Metadata{
		Name:        strings.TrimSpace(metadata.Name),
		Namespace:   strings.TrimSpace(metadata.Namespace),
		Version:     strings.TrimSpace(metadata.Version),
		Description: strings.TrimSpace(metadata.Description),
		Homepage:    strings.TrimSpace(metadata.Homepage),
		License:     strings.TrimSpace(metadata.License),
	}
}

func contentsFromRaw(contents []rawContentRef) []core.ContentRef {
	result := make([]core.ContentRef, 0, len(contents))
	for _, content := range contents {
		result = append(result, core.ContentRef{Kind: content.Kind, Name: content.Name})
	}
	return result
}

func addTool(pkg *core.Package, tool core.Tool) error {
	if pkg.Tools == nil {
		pkg.Tools = map[string]core.Tool{}
	}
	name := strings.TrimSpace(tool.Metadata.Name)
	if name == "" {
		return fmt.Errorf("tool name is required")
	}
	if _, ok := pkg.Tools[name]; ok {
		return fmt.Errorf("duplicate tool %q", name)
	}
	pkg.Tools[name] = tool
	return nil
}

func addBundle(pkg *core.Package, bundle core.Bundle) error {
	if pkg.Bundles == nil {
		pkg.Bundles = map[string]core.Bundle{}
	}
	name := strings.TrimSpace(bundle.Metadata.Name)
	if name == "" {
		return fmt.Errorf("bundle name is required")
	}
	if _, ok := pkg.Bundles[name]; ok {
		return fmt.Errorf("duplicate bundle %q", name)
	}
	pkg.Bundles[name] = bundle
	return nil
}

func addAsset(pkg *core.Package, asset core.Asset) error {
	if pkg.Assets == nil {
		pkg.Assets = map[string]core.Asset{}
	}
	name := strings.TrimSpace(asset.Metadata.Name)
	if name == "" {
		return fmt.Errorf("asset name is required")
	}
	if _, ok := pkg.Assets[name]; ok {
		return fmt.Errorf("duplicate asset %q", name)
	}
	pkg.Assets[name] = asset
	return nil
}

func addEnvironment(pkg *core.Package, env core.Environment) error {
	if pkg.Environments == nil {
		pkg.Environments = map[string]core.Environment{}
	}
	name := strings.TrimSpace(env.Metadata.Name)
	if name == "" {
		return fmt.Errorf("environment name is required")
	}
	if _, ok := pkg.Environments[name]; ok {
		return fmt.Errorf("duplicate environment %q", name)
	}
	pkg.Environments[name] = env
	return nil
}

func addSecret(pkg *core.Package, secret core.Secret) error {
	if pkg.Secrets == nil {
		pkg.Secrets = map[string]core.Secret{}
	}
	name := strings.TrimSpace(secret.Metadata.Name)
	if name == "" {
		return fmt.Errorf("secret name is required")
	}
	if _, ok := pkg.Secrets[name]; ok {
		return fmt.Errorf("duplicate secret %q", name)
	}
	pkg.Secrets[name] = secret
	return nil
}

func addWorkspace(pkg *core.Package, workspace core.Workspace) error {
	if pkg.Workspaces == nil {
		pkg.Workspaces = map[string]core.Workspace{}
	}
	name := strings.TrimSpace(workspace.Metadata.Name)
	if name == "" {
		return fmt.Errorf("workspace name is required")
	}
	if _, ok := pkg.Workspaces[name]; ok {
		return fmt.Errorf("duplicate workspace %q", name)
	}
	pkg.Workspaces[name] = workspace
	return nil
}

func inheritResourceMetadata(pkg *core.Package) {
	namespace := pkg.Provider.Metadata.Namespace
	version := pkg.Provider.Metadata.Version
	for name, tool := range pkg.Tools {
		tool.Metadata.Namespace = firstNonEmpty(tool.Metadata.Namespace, namespace)
		tool.Metadata.Version = firstNonEmpty(tool.Metadata.Version, version)
		pkg.Tools[name] = tool
	}
	for name, bundle := range pkg.Bundles {
		bundle.Metadata.Namespace = firstNonEmpty(bundle.Metadata.Namespace, namespace)
		bundle.Metadata.Version = firstNonEmpty(bundle.Metadata.Version, version)
		pkg.Bundles[name] = bundle
	}
	for name, asset := range pkg.Assets {
		asset.Metadata.Namespace = firstNonEmpty(asset.Metadata.Namespace, namespace)
		asset.Metadata.Version = firstNonEmpty(asset.Metadata.Version, version)
		pkg.Assets[name] = asset
	}
	for name, env := range pkg.Environments {
		env.Metadata.Namespace = firstNonEmpty(env.Metadata.Namespace, namespace)
		env.Metadata.Version = firstNonEmpty(env.Metadata.Version, version)
		pkg.Environments[name] = env
	}
	for name, secret := range pkg.Secrets {
		secret.Metadata.Namespace = firstNonEmpty(secret.Metadata.Namespace, namespace)
		secret.Metadata.Version = firstNonEmpty(secret.Metadata.Version, version)
		pkg.Secrets[name] = secret
	}
	for name, workspace := range pkg.Workspaces {
		workspace.Metadata.Namespace = firstNonEmpty(workspace.Metadata.Namespace, namespace)
		workspace.Metadata.Version = firstNonEmpty(workspace.Metadata.Version, version)
		pkg.Workspaces[name] = workspace
	}
}

func nodeKind(node *yaml.Node) (string, error) {
	target := node
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		target = node.Content[0]
	}
	if target.Kind != yaml.MappingNode {
		return "", fmt.Errorf("manifest documents must be mappings")
	}
	for index := 0; index+1 < len(target.Content); index += 2 {
		if strings.TrimSpace(target.Content[index].Value) == "kind" {
			return strings.TrimSpace(target.Content[index+1].Value), nil
		}
	}
	return "", fmt.Errorf("manifest document is missing kind")
}

func isEmptyDocument(node *yaml.Node) bool {
	if node.Kind == 0 {
		return true
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) == 0 {
		return true
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		return isEmptyDocument(node.Content[0])
	}
	if node.Kind == yaml.ScalarNode {
		return strings.TrimSpace(node.Value) == ""
	}
	return false
}

func isLegacyProvider(spec rawProviderSpec) bool {
	return strings.TrimSpace(spec.Runtime) != "" || strings.TrimSpace(spec.Entrypoint) != "" || len(spec.Platforms) > 0 || len(spec.Env) > 0 || len(spec.Path) > 0 || len(spec.Capabilities) > 0 || strings.TrimSpace(spec.Layers.Assets.Root) != ""
}

func legacyRuntimeType(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "binary" {
		return core.RuntimeOCI
	}
	return trimmed
}

func applyFromShortcut(source *rawSource, from string) {
	trimmed := strings.TrimSpace(from)
	if trimmed == "" {
		return
	}
	switch {
	case trimmed == core.SourceScript:
		source.Type = core.SourceScript
	case trimmed == core.SourceLocal:
		source.Type = core.SourceLocal
	case strings.HasPrefix(trimmed, "bundle."):
		source.Type = core.SourceBundle
		source.Ref = strings.TrimSpace(strings.TrimPrefix(trimmed, "bundle."))
	case strings.HasPrefix(trimmed, "layers."):
		source.Type = core.SourceBundle
		source.Ref = strings.TrimSpace(strings.TrimPrefix(trimmed, "layers."))
	default:
		source.Type = trimmed
	}
}

func defaultBundleMediaType(bundleType string) string {
	if strings.EqualFold(strings.TrimSpace(bundleType), "asset") {
		return "application/vnd.tinx.asset.layer.v1+tar"
	}
	return "application/vnd.tinx.tool.binary"
}

func copyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	copyMap := make(map[string]string, len(values))
	for key, value := range values {
		copyMap[key] = value
	}
	return copyMap
}

func mapKeys(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func normalizePaths(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		result = append(result, filepath.ToSlash(trimmed))
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
