package oci

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sourceplane/tinx/internal/state"
)

const storeProviderMetadataName = "provider-metadata.json"

func storeProviderMetadataPath(storeRoot string) string {
	return filepath.Join(storeRoot, storeProviderMetadataName)
}

func saveStoreProviderMetadata(storeRoot string, meta state.ProviderMetadata) error {
	if err := os.MkdirAll(storeRoot, 0o755); err != nil {
		return fmt.Errorf("create provider store root: %w", err)
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode store provider metadata: %w", err)
	}
	if err := os.WriteFile(storeProviderMetadataPath(storeRoot), data, 0o644); err != nil {
		return fmt.Errorf("write store provider metadata: %w", err)
	}
	return nil
}

func loadStoreProviderMetadata(storeRoot string) (state.ProviderMetadata, error) {
	var meta state.ProviderMetadata
	data, err := os.ReadFile(storeProviderMetadataPath(storeRoot))
	if err != nil {
		return meta, err
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return meta, fmt.Errorf("decode store provider metadata: %w", err)
	}
	return meta, nil
}

func listStoredProviders(home string) ([]state.ProviderMetadata, error) {
	storeRoot := filepath.Join(strings.TrimSpace(home), "store")
	entries, err := os.ReadDir(storeRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read store root: %w", err)
	}
	providers := make([]state.ProviderMetadata, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := loadStoreProviderMetadata(filepath.Join(storeRoot, entry.Name()))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		providers = append(providers, meta)
	}
	sort.Slice(providers, func(i, j int) bool {
		left := storeCandidateKey(providers[i])
		right := storeCandidateKey(providers[j])
		return left < right
	})
	return providers, nil
}

func storeCandidateKey(meta state.ProviderMetadata) string {
	if storeID := strings.TrimSpace(meta.StoreID); storeID != "" {
		return storeID
	}
	if layoutPath := strings.TrimSpace(meta.Source.LayoutPath); layoutPath != "" {
		return layoutPath + "@" + strings.TrimSpace(meta.Source.Tag)
	}
	return state.MetadataKey(meta)
}

func activateCachedProvider(activationHome, alias string, meta state.ProviderMetadata, needsActivation, plainHTTP bool) (state.ProviderMetadata, error) {
	resolved := meta
	if plainHTTP {
		resolved.Source.PlainHTTP = true
	}
	if needsActivation {
		resolved.InstalledAt = time.Now().UTC()
		if err := state.SaveProviderMetadata(activationHome, resolved); err != nil {
			return state.ProviderMetadata{}, err
		}
		if err := state.SaveInstallSource(activationHome, resolved.Namespace, resolved.Name, resolved.Version, resolved.Source); err != nil {
			return state.ProviderMetadata{}, err
		}
	}
	if err := updateAlias(activationHome, alias, resolved); err != nil {
		return state.ProviderMetadata{}, err
	}
	return resolved, nil
}
