package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	preferredProviderManifestName = "provider.yaml"
	legacyProviderManifestName    = "kiox.yaml"
)

func resolveProviderManifestPath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve manifest path: %w", err)
	}
	if _, err := os.Stat(absPath); err == nil {
		return absPath, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat manifest path: %w", err)
	}
	if filepath.Base(absPath) != preferredProviderManifestName {
		return absPath, nil
	}
	legacyPath := filepath.Join(filepath.Dir(absPath), legacyProviderManifestName)
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat manifest path: %w", err)
	}
	return absPath, nil
}