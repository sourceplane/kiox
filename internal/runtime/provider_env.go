package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sourceplane/tinx/internal/core"
	"github.com/sourceplane/tinx/internal/oci"
	"github.com/sourceplane/tinx/internal/state"
)

type ProviderEnvironmentSpec struct {
	Home          string
	WorkspaceRoot string
	Alias         string
	ToolName      string
	BinaryPath    string
	Metadata      state.ProviderMetadata
}

func ResolveProviderEnvironment(spec ProviderEnvironmentSpec) (map[string]string, []string, error) {
	providerRoot := state.MetadataStoreRoot(spec.Metadata)
	if providerRoot == "" {
		return nil, nil, fmt.Errorf("provider store is missing for %s/%s@%s", spec.Metadata.Namespace, spec.Metadata.Name, spec.Metadata.Version)
	}
	pkg, err := oci.LoadPackageModel(spec.Metadata)
	if err != nil {
		return nil, nil, fmt.Errorf("load provider package: %w", err)
	}
	tool, ok := resolveEnvironmentTool(pkg, spec.ToolName)
	if !ok {
		return nil, nil, fmt.Errorf("resolve provider environment tool %q", spec.ToolName)
	}

	templateVars := providerTemplateVars(spec, providerRoot, providerAssetsRoot(pkg, providerRoot))
	env := map[string]string{}
	for _, envName := range resolveEnvironmentNames(pkg, tool) {
		providerEnv := pkg.Environments[envName]
		keys := exportKeys(providerEnv.Spec)
		for _, key := range keys {
			if value, ok := providerEnv.Spec.Variables[key]; ok {
				env[key] = expandProviderTemplate(value, templateVars)
			}
		}
	}
	toolKeys := make([]string, 0, len(tool.Spec.Env))
	for key := range tool.Spec.Env {
		toolKeys = append(toolKeys, key)
	}
	sort.Strings(toolKeys)
	for _, key := range toolKeys {
		env[key] = expandProviderTemplate(tool.Spec.Env[key], templateVars)
	}

	pathEntries := make([]string, 0)
	for _, envName := range resolveEnvironmentNames(pkg, tool) {
		for _, entry := range pkg.Environments[envName].Spec.Path {
			expanded := strings.TrimSpace(expandProviderTemplate(entry, templateVars))
			if expanded == "" {
				continue
			}
			if !filepath.IsAbs(expanded) {
				expanded = filepath.Join(providerRoot, filepath.FromSlash(expanded))
			}
			pathEntries = appendUniquePaths(pathEntries, filepath.Clean(expanded))
		}
	}
	for _, entry := range tool.Spec.Path {
		expanded := strings.TrimSpace(expandProviderTemplate(entry, templateVars))
		if expanded == "" {
			continue
		}
		if !filepath.IsAbs(expanded) {
			expanded = filepath.Join(providerRoot, filepath.FromSlash(expanded))
		}
		pathEntries = appendUniquePaths(pathEntries, filepath.Clean(expanded))
	}
	return env, pathEntries, nil
}

func resolveEnvironmentTool(pkg core.Package, toolName string) (core.Tool, bool) {
	if strings.TrimSpace(toolName) != "" {
		if tool, ok := pkg.Tool(toolName); ok {
			return tool, true
		}
	}
	return pkg.DefaultTool()
}

func resolveEnvironmentNames(pkg core.Package, tool core.Tool) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(pkg.Environments)+len(tool.Spec.Environments))
	for _, content := range pkg.Provider.Spec.Contents {
		if content.Kind != core.KindEnvironment {
			continue
		}
		if _, ok := pkg.Environments[content.Name]; !ok {
			continue
		}
		if _, ok := seen[content.Name]; ok {
			continue
		}
		seen[content.Name] = struct{}{}
		result = append(result, content.Name)
	}
	if len(result) == 0 {
		for _, env := range pkg.SortedEnvironments() {
			if _, ok := seen[env.Metadata.Name]; ok {
				continue
			}
			seen[env.Metadata.Name] = struct{}{}
			result = append(result, env.Metadata.Name)
		}
	}
	for _, envName := range tool.Spec.Environments {
		if _, ok := pkg.Environments[envName]; !ok {
			continue
		}
		if _, ok := seen[envName]; ok {
			continue
		}
		seen[envName] = struct{}{}
		result = append(result, envName)
	}
	return result
}

func exportKeys(spec core.EnvironmentSpec) []string {
	if len(spec.Export) > 0 {
		return append([]string(nil), spec.Export...)
	}
	keys := make([]string, 0, len(spec.Variables))
	for key := range spec.Variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func providerAssetsRoot(pkg core.Package, providerRoot string) string {
	if len(pkg.Assets) == 0 {
		return providerRoot
	}
	paths := make([]string, 0, len(pkg.Assets))
	for _, asset := range pkg.Assets {
		path := strings.TrimSpace(asset.Spec.Mount.Path)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(providerRoot, filepath.FromSlash(path))
		}
		paths = append(paths, filepath.Clean(path))
	}
	if len(paths) == 0 {
		return providerRoot
	}
	sort.Strings(paths)
	return paths[0]
}

func providerTemplateVars(spec ProviderEnvironmentSpec, providerRoot, assetsRoot string) map[string]string {
	workingDir, err := os.Getwd()
	if err != nil {
		workingDir = "."
	}
	workspaceRoot := strings.TrimSpace(spec.WorkspaceRoot)
	workspaceHome := ""
	if workspaceRoot != "" {
		workspaceHome = spec.Home
	}
	providerAssets := providerRoot
	if strings.TrimSpace(assetsRoot) != "" {
		if filepath.IsAbs(assetsRoot) {
			providerAssets = filepath.Clean(assetsRoot)
		} else {
			providerAssets = filepath.Join(providerRoot, filepath.FromSlash(assetsRoot))
		}
	}
	return map[string]string{
		"cwd":                workingDir,
		"workspace_root":     workspaceRoot,
		"workspace_home":     workspaceHome,
		"provider_alias":     strings.TrimSpace(spec.Alias),
		"provider_ref":       strings.TrimSpace(spec.Metadata.Namespace) + "/" + strings.TrimSpace(spec.Metadata.Name),
		"provider_namespace": strings.TrimSpace(spec.Metadata.Namespace),
		"provider_name":      strings.TrimSpace(spec.Metadata.Name),
		"provider_version":   strings.TrimSpace(spec.Metadata.Version),
		"provider_home":      providerRoot,
		"provider_root":      providerRoot,
		"provider_binary":    strings.TrimSpace(spec.BinaryPath),
		"provider_assets":    providerAssets,
	}
}

func expandProviderTemplate(value string, vars map[string]string) string {
	return os.Expand(value, func(name string) string {
		if resolved, ok := vars[name]; ok {
			return resolved
		}
		return "${" + name + "}"
	})
}

func appendUniquePaths(existing []string, values ...string) []string {
	seen := make(map[string]struct{}, len(existing)+len(values))
	for _, value := range existing {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		seen[trimmed] = struct{}{}
	}
	result := append([]string(nil), existing...)
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
	return result
}
