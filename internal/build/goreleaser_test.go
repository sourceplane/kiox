package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveGoReleaserConfigPrefersExplicitPath(t *testing.T) {
	moduleRoot := t.TempDir()
	explicit := filepath.Join(moduleRoot, "custom.yml")
	if err := os.WriteFile(explicit, []byte("project_name: explicit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveGoReleaserConfig(moduleRoot, explicit, "")
	if err != nil {
		t.Fatalf("resolve explicit config: %v", err)
	}
	if resolved != explicit {
		t.Fatalf("expected %s, got %s", explicit, resolved)
	}
}

func TestResolveGoReleaserConfigGeneratesFromManifest(t *testing.T) {
	moduleRoot := t.TempDir()
	manifestPath := filepath.Join(moduleRoot, "kiox.yaml")
	content := `apiVersion: kiox.io/v1
kind: Provider
metadata:
  namespace: sourceplane
  name: sparse-provider
  version: v0.2.0
spec:
  runtime: binary
  entrypoint: sparse-provider
  platforms:
    - os: darwin
      arch: amd64
      binary: bin/darwin/amd64/sparse-provider
    - os: linux
      arch: arm64
      binary: bin/linux/arm64/sparse-provider
`
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(moduleRoot, "cmd", "sparse-provider"), 0o755); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveGoReleaserConfig(moduleRoot, "", manifestPath)
	if err != nil {
		t.Fatalf("resolve generated config: %v", err)
	}
	if filepath.Base(resolved) != ".goreleaser.kiox.generated.yaml" {
		t.Fatalf("unexpected generated config name: %s", resolved)
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		t.Fatal(err)
	}
	generated := string(data)
	checks := []string{
		"project_name: sparse-provider",
		"main: ./cmd/sparse-provider",
		"binary: sparse-provider",
		"- darwin",
		"- linux",
		"- amd64",
		"- arm64",
		"goos: darwin",
		"goarch: arm64",
		"mkdir -p dist/bin/{{ .Os }}/{{ .Arch }}",
		"cp {{ .Path }} dist/bin/{{ .Os }}/{{ .Arch }}/sparse-provider",
		"disable: true",
	}
	for _, check := range checks {
		if !strings.Contains(generated, check) {
			t.Fatalf("generated config missing %q:\n%s", check, generated)
		}
	}
}
