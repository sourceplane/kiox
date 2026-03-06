package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tinx.yaml")
	content := `apiVersion: tinx.io/v1
kind: Provider
metadata:
  namespace: sourceplane
  name: demo
  version: v0.1.0
spec:
  runtime: binary
  entrypoint: demo
  platforms:
    - os: darwin
      arch: arm64
      binary: bin/darwin/arm64/demo
  capabilities:
    plan:
      description: Generate a plan
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	provider, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if provider.Ref() != "sourceplane/demo" {
		t.Fatalf("Ref() = %q", provider.Ref())
	}
	if !provider.HasCapability("plan") {
		t.Fatal("expected plan capability")
	}
}

func TestLoadProviderRequiresPlatforms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tinx.yaml")
	content := `apiVersion: tinx.io/v1
kind: Provider
metadata:
  namespace: sourceplane
  name: demo
  version: v0.1.0
spec:
  runtime: binary
  entrypoint: demo
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected validation error")
	}
}
