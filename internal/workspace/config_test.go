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
	workspaceContent := []byte("apiVersion: kiox.io/v1\nkind: Workspace\nmetadata:\n  name: dev\nspec:\n  providers:\n    echo: ghcr.io/sourceplane/echo-provider:v0.1.0\n")
	if err := os.WriteFile(workspacePath, workspaceContent, 0o644); err != nil {
		t.Fatal(err)
	}
	providerDir := filepath.Join(root, "providers", "example")
	if err := os.MkdirAll(providerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	providerManifest := []byte("apiVersion: kiox.io/v1\nkind: Provider\nmetadata:\n  namespace: sourceplane\n  name: example\n  version: v0.1.0\nspec:\n  runtime: binary\n  entrypoint: example\n  platforms:\n    - os: darwin\n      arch: arm64\n      binary: bin/darwin/arm64/example\n")
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
		Metadata:  Metadata{Name: "dev"},
		Providers: map[string]Provider{
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

func TestSaveWritesCanonicalWorkspaceManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ManifestName)
	config := Config{
		APIVersion: APIVersionV1,
		Kind:       KindWorkspace,
		Workspace:  "legacy-name",
		Metadata:   Metadata{Name: "dev"},
		Providers:  map[string]Provider{},
	}

	if err := Save(path, config); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved manifest: %v", err)
	}
	content := string(data)
	for _, expected := range []string{"kind: Workspace", "providers: {}", "name: dev"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("expected %q in saved manifest, got:\n%s", expected, content)
		}
	}
	if strings.Contains(content, "workspace:") {
		t.Fatalf("expected canonical manifest to omit legacy workspace field, got:\n%s", content)
	}
}

func TestSaveLockWritesCanonicalMetadata(t *testing.T) {
	root := t.TempDir()
	if err := SaveLock(root, "dev", nil); err != nil {
		t.Fatalf("SaveLock() error = %v", err)
	}

	data, err := os.ReadFile(LockPath(root))
	if err != nil {
		t.Fatalf("read saved lock file: %v", err)
	}
	content := string(data)
	for _, expected := range []string{"kind: WorkspaceLock", "metadata:", "name: dev"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("expected %q in saved lock file, got:\n%s", expected, content)
		}
	}
	if strings.Contains(content, "workspace:") {
		t.Fatalf("expected canonical lock file to omit legacy workspace field, got:\n%s", content)
	}
}
