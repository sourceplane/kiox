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
	Out io.Writer
}

type SyncResult struct {
	Home      string
	Aliases   map[string]string
	Providers []LockedProvider
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
		Aliases:   make(map[string]string, len(config.ProviderMap())),
		Providers: make([]LockedProvider, 0, len(config.ProviderMap())),
	}
	for _, alias := range config.ProviderAliases() {
		provider := config.ProviderMap()[alias]
		source := resolveWorkspaceSource(root, provider.Source)
		installed, err := installWorkspaceProvider(ctx, home, alias, source, provider, opts)
		if err != nil {
			return SyncResult{}, err
		}
		providerRef := installed.Namespace + "/" + installed.Name
		result.Aliases[alias] = providerRef
		result.Providers = append(result.Providers, LockedProvider{
			Alias:    alias,
			Provider: providerRef,
			Source:   source,
			Version:  installed.Version,
		})
	}
	if err := state.SaveAliases(home, result.Aliases); err != nil {
		return SyncResult{}, err
	}
	if err := SaveLock(root, config.Name(), result.Providers); err != nil {
		return SyncResult{}, err
	}
	return result, nil
}

func installWorkspaceProvider(ctx context.Context, home, alias, source string, provider Provider, opts SyncOptions) (state.ProviderMetadata, error) {
	if layoutPath, ok := localLayoutPath(source); ok {
		return oci.InstallMetadata(layoutPath, "", home, alias, opts.Out)
	}
	if resolver.HasSourceScheme(source) {
		return state.ProviderMetadata{}, fmt.Errorf("unsupported provider source %q: expected an OCI registry reference or local OCI layout", source)
	}
	return oci.InstallRemote(ctx, home, source, alias, provider.PlainHTTP, opts.Out)
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
