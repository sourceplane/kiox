package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSupportsTopLevelWorkspaceShape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ManifestName)
	content := []byte("workspace: dev\nproviders:\n  echo: ghcr.io/sourceplane/echo-provider:v0.1.0\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	config, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.Kind != KindWorkspace {
		t.Fatalf("expected workspace kind, got %q", config.Kind)
	}
	if config.APIVersion != APIVersionV2Alpha1 {
		t.Fatalf("expected workspace apiVersion %q, got %q", APIVersionV2Alpha1, config.APIVersion)
	}
	if !config.HasProviderAlias("echo") {
		t.Fatalf("expected echo provider alias to be present")
	}
	if got := config.ProviderMap()["echo"].Source; got != "ghcr.io/sourceplane/echo-provider:v0.1.0" {
		t.Fatalf("unexpected provider source %q", got)
	}
}

func TestDiscoverSkipsProviderManifestAndFindsParentWorkspace(t *testing.T) {
	root := t.TempDir()
	workspacePath := filepath.Join(root, ManifestName)
	workspaceContent := []byte("apiVersion: tinx.io/v2alpha1\nkind: Workspace\nmetadata:\n  name: dev\ntools:\n  echo:\n    source: ghcr.io/sourceplane/echo-provider:v0.1.0\n")
	if err := os.WriteFile(workspacePath, workspaceContent, 0o644); err != nil {
		t.Fatal(err)
	}
	providerDir := filepath.Join(root, "providers", "example")
	if err := os.MkdirAll(providerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	providerManifest := []byte("apiVersion: tinx.io/v2alpha1\nkind: Package\nmetadata:\n  namespace: sourceplane\n  name: example\n  version: v0.1.0\nspec:\n  runtime:\n    type: binary\n    entrypoint: example\n  platforms:\n    - os: darwin\n      arch: arm64\n      binary: bin/darwin/arm64/example\n")
	if err := os.WriteFile(filepath.Join(providerDir, ManifestName), providerManifest, 0o644); err != nil {
		t.Fatal(err)
	}

	discovery, err := Discover(providerDir)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if discovery == nil {
		t.Fatal("expected workspace discovery result")
	}
	if discovery.Root != root {
		t.Fatalf("expected workspace root %q, got %q", root, discovery.Root)
	}
	if discovery.DisplayName() != "dev" {
		t.Fatalf("expected workspace name dev, got %q", discovery.DisplayName())
	}
}

func TestSyncRejectsProviderSourceSchemes(t *testing.T) {
	root := t.TempDir()
	config := Config{
		Kind:      KindWorkspace,
		Workspace: "dev",
		Tools: map[string]Tool{
			"echo": {Source: "custom://acme/echo@v1"},
		},
	}

	_, err := Sync(context.Background(), root, config, SyncOptions{})
	if err == nil {
		t.Fatal("expected Sync to reject provider source schemes")
	}
	if !strings.Contains(err.Error(), `unsupported provider source "custom://acme/echo@v1"`) {
		t.Fatalf("unexpected Sync error: %v", err)
	}
}
