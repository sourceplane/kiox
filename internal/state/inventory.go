package state

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func ListInstalledProviders(home string) ([]ProviderMetadata, error) {
	providersRoot := filepath.Join(home, "providers")
	namespaces, err := os.ReadDir(providersRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read providers root: %w", err)
	}

	providers := make([]ProviderMetadata, 0)
	for _, namespaceEntry := range namespaces {
		if !namespaceEntry.IsDir() {
			continue
		}
		namespace := namespaceEntry.Name()
		providerEntries, err := os.ReadDir(filepath.Join(providersRoot, namespace))
		if err != nil {
			return nil, fmt.Errorf("read provider namespace %q: %w", namespace, err)
		}
		for _, providerEntry := range providerEntries {
			if !providerEntry.IsDir() {
				continue
			}
			versions, err := os.ReadDir(filepath.Join(providersRoot, namespace, providerEntry.Name()))
			if err != nil {
				return nil, fmt.Errorf("read provider versions for %q/%q: %w", namespace, providerEntry.Name(), err)
			}
			for _, versionEntry := range versions {
				if !versionEntry.IsDir() {
					continue
				}
				meta, err := LoadProviderMetadata(home, namespace, providerEntry.Name(), versionEntry.Name())
				if err != nil {
					return nil, err
				}
				providers = append(providers, meta)
			}
		}
	}

	sort.Slice(providers, func(i, j int) bool {
		left := ProviderKey(providers[i].Namespace, providers[i].Name, providers[i].Version)
		right := ProviderKey(providers[j].Namespace, providers[j].Name, providers[j].Version)
		return left < right
	})
	return providers, nil
}
