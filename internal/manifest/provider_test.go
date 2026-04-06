package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tinx.yaml")
	content := strings.Join([]string{
		"apiVersion: tinx.io/v2alpha1",
		"kind: Package",
		"metadata:",
		"  namespace: sourceplane",
		"  name: demo",
		"  version: v0.1.0",
		"spec:",
		"  runtime:",
		"    type: binary",
		"    entrypoint: demo",
		"  env:",
		"    DEMO_REF: ${package_ref}",
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
	if provider.APIVersion != APIVersionV2Alpha1 {
		t.Fatalf("expected v2alpha1 package, got %q", provider.APIVersion)
	}
	if provider.Kind != KindPackage {
		t.Fatalf("expected normalized package kind, got %q", provider.Kind)
	}
	if provider.Spec.Runtime.Type != RuntimeBinary {
		t.Fatalf("expected binary runtime, got %#v", provider.Spec.Runtime)
	}
	if !provider.HasCapability("plan") {
		t.Fatal("expected plan capability")
	}
	if provider.Spec.Env["DEMO_REF"] != "${package_ref}" {
		t.Fatalf("expected env expansion template to round-trip, got %q", provider.Spec.Env["DEMO_REF"])
	}
	if len(provider.Spec.Path) != 1 || provider.Spec.Path[0] != "assets/bin" {
		t.Fatalf("expected provider path to round-trip, got %#v", provider.Spec.Path)
	}
}

func TestLoadProviderRequiresPlatforms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tinx.yaml")
	content := `apiVersion: tinx.io/v2alpha1
kind: Package
metadata:
  namespace: sourceplane
  name: demo
  version: v0.1.0
spec:
  runtime:
    type: binary
    entrypoint: demo
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected validation error")
	}
}
