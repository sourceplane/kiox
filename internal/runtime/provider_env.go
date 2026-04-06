package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sourceplane/tinx/internal/manifest"
	"github.com/sourceplane/tinx/internal/state"
)

type ProviderEnvironmentSpec struct {
	Home          string
	WorkspaceRoot string
	Alias         string
	BinaryPath    string
	Metadata      state.ProviderMetadata
}

func ResolveProviderEnvironment(spec ProviderEnvironmentSpec) (map[string]string, []string, error) {
	providerRoot := state.MetadataStoreRoot(spec.Metadata)
	if providerRoot == "" {
		return nil, nil, fmt.Errorf("provider store is missing for %s/%s@%s", spec.Metadata.Namespace, spec.Metadata.Name, spec.Metadata.Version)
	}
	providerManifestPath := filepath.Join(providerRoot, "tinx.yaml")
	provider, err := manifest.Load(providerManifestPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load provider manifest: %w", err)
	}

	templateVars := providerTemplateVars(spec, providerRoot, provider.AssetsRoot())
	env := map[string]string{}
	if len(provider.Spec.Env) > 0 {
		keys := make([]string, 0, len(provider.Spec.Env))
		for key := range provider.Spec.Env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			env[key] = expandProviderTemplate(provider.Spec.Env[key], templateVars)
		}
	}

	pathEntries := make([]string, 0, len(provider.Spec.Path))
	for _, entry := range provider.Spec.Path {
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
		providerAssets = filepath.Join(providerRoot, filepath.FromSlash(assetsRoot))
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
