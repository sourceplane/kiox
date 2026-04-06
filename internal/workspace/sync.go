package workspace

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sourceplane/tinx/internal/oci"
	"github.com/sourceplane/tinx/internal/resolver"
	"github.com/sourceplane/tinx/internal/state"
)

type SyncOptions struct {
	Out            io.Writer
	GlobalHome     string
	RefreshAliases []string
}

type SyncResult struct {
	Home      string
	Aliases   map[string]string
	Providers []LockedProvider
	Tools     []LockedTool
	Packages  []LockedPackage
}

func Sync(ctx context.Context, root string, config Config, opts SyncOptions) (SyncResult, error) {
	if err := config.Normalize(); err != nil {
		return SyncResult{}, err
	}
	home := Home(root)
	if err := state.EnsureHome(home); err != nil {
		return SyncResult{}, err
	}
	result := SyncResult{
		Home:      home,
		Aliases:   make(map[string]string, len(config.ToolMap())),
		Providers: make([]LockedProvider, 0, len(config.ToolMap())),
		Tools:     make([]LockedTool, 0, len(config.ToolMap())),
		Packages:  make([]LockedPackage, 0, len(config.ToolMap())),
	}
	storeHome := strings.TrimSpace(opts.GlobalHome)
	if storeHome == "" {
		storeHome = home
	}
	locked, err := LoadLock(root)
	if err != nil {
		return SyncResult{}, err
	}
	lockedByAlias := make(map[string]LockedProvider, len(locked.Providers))
	for _, provider := range locked.Providers {
		lockedByAlias[provider.Alias] = provider
	}
	refreshAliases := make(map[string]struct{}, len(opts.RefreshAliases))
	for _, alias := range opts.RefreshAliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		refreshAliases[alias] = struct{}{}
	}
	for _, alias := range config.ToolNames() {
		tool := config.ToolMap()[alias]
		source := declaredToolSource(root, tool)
		installSource := resolvedWorkspaceInstallSource(source, lockedByAlias[alias], refreshAliases, alias)
		installed, err := installWorkspaceProvider(ctx, home, storeHome, alias, installSource, tool, opts)
		if err != nil {
			return SyncResult{}, err
		}
		providerRef := installed.Namespace + "/" + installed.Name
		providerKey := state.MetadataKey(installed)
		resolvedSource := lockedProviderResolvedSource(installed, installSource)
		result.Aliases[alias] = providerKey
		result.Providers = append(result.Providers, LockedProvider{
			Alias:    alias,
			Provider: providerRef,
			Source:   source,
			Version:  installed.Version,
			Resolved: resolvedSource,
			Store:    installed.StoreID,
		})
		result.Tools = append(result.Tools, LockedTool{
			Name:            alias,
			Package:         providerRef,
			Constraint:      strings.TrimSpace(tool.Version),
			ResolvedPackage: state.ProviderKey(installed.Namespace, installed.Name, installed.Version),
		})
		result.Packages = append(result.Packages, LockedPackage{
			Package:      providerRef,
			Version:      installed.Version,
			Source:       source,
			Resolved:     resolvedSource,
			Digest:       strings.TrimSpace(installed.Source.Digest),
			ContentStore: installed.StoreID,
			Runtime: LockedRuntime{
				Type:       strings.TrimSpace(installed.Runtime),
				Entrypoint: strings.TrimSpace(installed.Entrypoint),
			},
			Dependencies: copyStringMap(installed.Dependencies),
		})
	}
	if err := state.SaveAliases(home, result.Aliases); err != nil {
		return SyncResult{}, err
	}
	manifestHash, err := ManifestHash(config)
	if err != nil {
		return SyncResult{}, err
	}
	result.Packages = dedupeLockedPackages(result.Packages)
	sort.Slice(result.Tools, func(i, j int) bool {
		return result.Tools[i].Name < result.Tools[j].Name
	})
	if err := SaveLock(root, config.Name(), manifestHash, result.Tools, result.Packages); err != nil {
		return SyncResult{}, err
	}
	return result, nil
}

func installWorkspaceProvider(ctx context.Context, home, storeHome, alias, source string, provider Provider, opts SyncOptions) (state.ProviderMetadata, error) {
	if layoutPath, ok := localLayoutPath(source); ok {
		return oci.InstallMetadata(layoutPath, "", home, storeHome, alias, opts.Out)
	}
	if resolver.HasSourceScheme(source) {
		return state.ProviderMetadata{}, fmt.Errorf("unsupported provider source %q: expected an OCI registry reference or local OCI layout", source)
	}
	return oci.InstallRemoteFull(ctx, home, storeHome, source, alias, provider.PlainHTTP, opts.Out)
}

func declaredToolSource(root string, tool Tool) string {
	if strings.TrimSpace(tool.Source) != "" {
		return resolveWorkspaceSource(root, tool.Source)
	}
	if strings.TrimSpace(tool.Package) == "" {
		return ""
	}
	source := resolver.ResolveProviderSource(tool.Package)
	if version := strings.TrimSpace(tool.Version); version != "" && isExactVersion(version) && !workspaceSourcePinned(source) {
		return source + ":" + version
	}
	return source
}

func resolvedWorkspaceInstallSource(source string, locked LockedProvider, refreshAliases map[string]struct{}, alias string) string {
	if _, ok := refreshAliases[alias]; ok {
		return source
	}
	if _, ok := localLayoutPath(source); ok {
		return source
	}
	if workspaceSourcePinned(source) {
		return source
	}
	if strings.TrimSpace(locked.Source) == source && strings.TrimSpace(locked.Resolved) != "" {
		return strings.TrimSpace(locked.Resolved)
	}
	return source
}

func workspaceSourcePinned(source string) bool {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "@") {
		return true
	}
	if idx := strings.LastIndex(trimmed, ":"); idx > 0 {
		after := trimmed[idx+1:]
		if !strings.Contains(after, "/") {
			return true
		}
	}
	return false
}

func lockedProviderResolvedSource(meta state.ProviderMetadata, installSource string) string {
	if resolved := strings.TrimSpace(meta.Source.Resolved); resolved != "" {
		return resolved
	}
	if resolved := strings.TrimSpace(meta.Source.Ref); resolved != "" {
		return resolved
	}
	return strings.TrimSpace(installSource)
}

func resolveWorkspaceSource(root, source string) string {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return ""
	}
	if path, ok := localLayoutPath(trimmed); ok {
		return path
	}
	if filepath.IsAbs(trimmed) {
		return trimmed
	}
	rootRelative := filepath.Join(root, trimmed)
	if path, ok := localLayoutPath(rootRelative); ok {
		return path
	}
	return resolver.ResolveProviderSource(trimmed)
}

func localLayoutPath(source string) (string, bool) {
	path := strings.TrimSpace(source)
	if path == "" {
		return "", false
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(path, "index.json")); err != nil {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(path, "oci-layout")); err != nil {
		return "", false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	return absPath, true
}

func isExactVersion(version string) bool {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" {
		return false
	}
	return !strings.ContainsAny(trimmed, "<>^~*,")
}

func copyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func dedupeLockedPackages(packages []LockedPackage) []LockedPackage {
	if len(packages) == 0 {
		return nil
	}
	byKey := make(map[string]LockedPackage, len(packages))
	for _, pkg := range packages {
		key := strings.TrimSpace(pkg.Package) + "@" + strings.TrimSpace(pkg.Version)
		byKey[key] = pkg
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]LockedPackage, 0, len(keys))
	for _, key := range keys {
		result = append(result, byKey[key])
	}
	return result
}
