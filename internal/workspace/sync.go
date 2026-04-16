package workspace

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
}

func Sync(ctx context.Context, root string, config Config, opts SyncOptions) (SyncResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := config.Normalize(); err != nil {
		return SyncResult{}, err
	}
	home := Home(root)
	if err := state.EnsureHome(home); err != nil {
		return SyncResult{}, err
	}
	existingAliases, err := state.LoadAliases(home)
	if err != nil {
		return SyncResult{}, err
	}
	result := SyncResult{
		Home:      home,
		Aliases:   make(map[string]string, len(config.ProviderMap())),
		Providers: make([]LockedProvider, 0, len(config.ProviderMap())),
	}
	storeHome := strings.TrimSpace(opts.GlobalHome)
	if storeHome == "" {
		storeHome = home
	}
	remoteCache, err := oci.LoadRemoteInstallCache(home, storeHome)
	if err != nil {
		return SyncResult{}, err
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
	for _, alias := range config.ProviderAliases() {
		provider := config.ProviderMap()[alias]
		source := resolveWorkspaceSource(root, provider.Source)
		installSource := resolvedWorkspaceInstallSource(source, lockedByAlias[alias], refreshAliases, alias)
		_, refreshRequested := refreshAliases[alias]
		installed, err := installWorkspaceProvider(ctx, home, storeHome, alias, installSource, provider, remoteCache, !refreshRequested, opts)
		if err != nil {
			return SyncResult{}, err
		}
		providerRef := installed.Namespace + "/" + installed.Name
		providerKey := state.MetadataKey(installed)
		result.Aliases[alias] = providerKey
		result.Providers = append(result.Providers, LockedProvider{
			Alias:    alias,
			Provider: providerRef,
			Source:   source,
			Version:  installed.Version,
			Resolved: lockedProviderResolvedSource(installed, installSource),
			Store:    installed.StoreID,
		})
	}
	if err := removeStaleWorkspaceProviders(home, existingAliases, result.Aliases); err != nil {
		return SyncResult{}, err
	}
	if err := state.SaveAliases(home, result.Aliases); err != nil {
		return SyncResult{}, err
	}
	if err := SaveLock(root, config.Name(), result.Providers); err != nil {
		return SyncResult{}, err
	}
	return result, nil
}

func installWorkspaceProvider(ctx context.Context, home, storeHome, alias, source string, provider Provider, remoteCache *oci.RemoteInstallCache, allowCache bool, opts SyncOptions) (state.ProviderMetadata, error) {
	if layoutPath, ok := localLayoutPath(source); ok {
		return oci.InstallMetadata(layoutPath, "", home, storeHome, alias, opts.Out)
	}
	if resolver.HasSourceScheme(source) {
		return state.ProviderMetadata{}, fmt.Errorf("unsupported provider source %q: expected an OCI registry reference or local OCI layout", source)
	}
	if allowCache {
		if cached, ok, err := remoteCache.Activate(home, alias, source, true, provider.PlainHTTP); err != nil {
			return state.ProviderMetadata{}, err
		} else if ok {
			return cached, nil
		}
	}
	return oci.InstallRemoteFull(ctx, home, storeHome, source, alias, provider.PlainHTTP, allowCache, opts.Out)
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
	return strings.Contains(trimmed, "@")
}

func lockedProviderResolvedSource(meta state.ProviderMetadata, installSource string) string {
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

func removeStaleWorkspaceProviders(home string, previousAliases, desiredAliases map[string]string) error {
	desiredKeys := make(map[string]struct{}, len(desiredAliases))
	for _, key := range desiredAliases {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		desiredKeys[trimmed] = struct{}{}
	}
	staleKeys := make(map[string]struct{})
	for alias, key := range previousAliases {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		if desiredKey, ok := desiredAliases[alias]; ok && strings.TrimSpace(desiredKey) == trimmedKey {
			continue
		}
		if _, ok := desiredKeys[trimmedKey]; ok {
			continue
		}
		staleKeys[trimmedKey] = struct{}{}
	}
	for key := range staleKeys {
		if err := state.RemoveProviderByKey(home, key); err != nil {
			return err
		}
	}
	return nil
}
