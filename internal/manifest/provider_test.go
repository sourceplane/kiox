package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "provider.yaml")
	content := strings.Join([]string{
		"apiVersion: kiox.io/v1",
		"kind: Provider",
		"metadata:",
		"  namespace: sourceplane",
		"  name: demo",
		"  version: v0.1.0",
		"spec:",
		"  runtime: binary",
		"  entrypoint: demo",
		"  env:",
		"    DEMO_REF: ${provider_ref}",
		"  path:",
		"    - assets/bin",
		"  platforms:",
		"    - os: darwin",
		"      arch: arm64",
		"      binary: bin/darwin/arm64/demo",
		"  capabilities:",
		"    plan:",
		"      description: Generate a plan",
		"",
	}, "\n")
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
	if provider.Spec.Env["DEMO_REF"] != "${provider_ref}" {
		t.Fatalf("expected env expansion template to round-trip, got %q", provider.Spec.Env["DEMO_REF"])
	}
	if len(provider.Spec.Path) != 1 || provider.Spec.Path[0] != "assets/bin" {
		t.Fatalf("expected provider path to round-trip, got %#v", provider.Spec.Path)
	}
}

func TestLoadProviderRequiresPlatforms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "provider.yaml")
	content := `apiVersion: kiox.io/v1
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
