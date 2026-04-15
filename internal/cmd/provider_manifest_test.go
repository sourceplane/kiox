package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProviderManifestPathPrefersProviderYAML(t *testing.T) {
	dir := t.TempDir()
	providerPath := filepath.Join(dir, preferredProviderManifestName)
	if err := os.WriteFile(providerPath, []byte("kind: Provider\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveProviderManifestPath(providerPath)
	if err != nil {
		t.Fatalf("resolve provider manifest path: %v", err)
	}
	if resolved != providerPath {
		t.Fatalf("expected %s, got %s", providerPath, resolved)
	}
}

func TestResolveProviderManifestPathFallsBackToLegacyTinxYAML(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, legacyProviderManifestName)
	if err := os.WriteFile(legacyPath, []byte("kind: Provider\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveProviderManifestPath(filepath.Join(dir, preferredProviderManifestName))
	if err != nil {
		t.Fatalf("resolve provider manifest path: %v", err)
	}
	if resolved != legacyPath {
		t.Fatalf("expected fallback path %s, got %s", legacyPath, resolved)
	}
}