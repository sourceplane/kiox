package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gcrregistry "github.com/google/go-containerregistry/pkg/registry"
)

func TestReleaseAndInstallLocalLayout(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, ".tinx-home")
	providerDir := copyTestProvider(t, workspace)

	releaseBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"release",
		"--manifest", filepath.Join(providerDir, "tinx.yaml"),
		"--dist", filepath.Join(providerDir, "dist"),
		"--output", filepath.Join(providerDir, "oci"),
	})
	if !bytes.Contains(releaseBuf.Bytes(), []byte("released sourceplane/echo-provider")) {
		t.Fatalf("unexpected release output: %s", releaseBuf.String())
	}

	installBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"install", "echo", "sourceplane/echo-provider",
		"--source", filepath.Join(providerDir, "oci"),
	})
	if !bytes.Contains(installBuf.Bytes(), []byte("installed sourceplane/echo-provider@v0.1.0")) {
		t.Fatalf("unexpected install output: %s", installBuf.String())
	}
	for _, expected := range []string{
		filepath.Join(home, "providers", "sourceplane", "echo-provider", "metadata.json"),
		filepath.Join(home, "providers", "sourceplane", "echo-provider", "v0.1.0", "oci", "index.json"),
		filepath.Join(home, "providers", "sourceplane", "echo-provider", "v0.1.0", "tinx.yaml"),
	} {
		if _, err := os.Stat(expected); err != nil {
			t.Fatalf("expected installed artifact %s: %v", expected, err)
		}
	}
}

func TestReleaseDelegatesToGoReleaser(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, ".tinx-home")
	providerDir := copyTestProvider(t, workspace)
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(workspace, "goreleaser.log")
	script := fmt.Sprintf(`#!/bin/sh
set -eu
printf '%%s\n' "$*" > %q
mkdir -p dist/bin/darwin/amd64 dist/bin/darwin/arm64 dist/bin/linux/amd64 dist/bin/linux/arm64
go build -o dist/bin/darwin/amd64/echo-provider ./cmd/echo-provider
cp dist/bin/darwin/amd64/echo-provider dist/bin/darwin/arm64/echo-provider
cp dist/bin/darwin/amd64/echo-provider dist/bin/linux/amd64/echo-provider
cp dist/bin/darwin/amd64/echo-provider dist/bin/linux/arm64/echo-provider
`, logPath)
	if err := os.WriteFile(filepath.Join(binDir, "goreleaser"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath)

	releaseBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"release",
		"--manifest", filepath.Join(providerDir, "tinx.yaml"),
		"--dist", filepath.Join(providerDir, "artifacts"),
		"--output", filepath.Join(providerDir, "oci"),
		"--delegate-goreleaser",
	})
	if !bytes.Contains(releaseBuf.Bytes(), []byte("released sourceplane/echo-provider")) {
		t.Fatalf("unexpected release output: %s", releaseBuf.String())
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logged), "build") {
		t.Fatalf("expected goreleaser invocation, got %q", string(logged))
	}
}

func TestReleaseDelegatesToGeneratedGoReleaserConfig(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, ".tinx-home")
	providerDir := copyTestProvider(t, workspace)
	if err := os.Remove(filepath.Join(providerDir, ".goreleaser.yaml")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(workspace, "goreleaser-generated.log")
	script := fmt.Sprintf(`#!/bin/sh
set -eu
printf '%%s\n' "$*" > %q
config=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--config" ]; then
    config="$2"
    shift 2
    continue
  fi
  shift
done
if [ -z "$config" ]; then
  echo "missing --config" >&2
  exit 1
fi
if [ ! -f "$config" ]; then
  echo "config file does not exist: $config" >&2
  exit 1
fi
grep -q 'project_name: echo-provider' "$config"
mkdir -p dist/bin/darwin/amd64 dist/bin/darwin/arm64 dist/bin/linux/amd64 dist/bin/linux/arm64
go build -o dist/bin/darwin/amd64/echo-provider ./cmd/echo-provider
cp dist/bin/darwin/amd64/echo-provider dist/bin/darwin/arm64/echo-provider
cp dist/bin/darwin/amd64/echo-provider dist/bin/linux/amd64/echo-provider
cp dist/bin/darwin/amd64/echo-provider dist/bin/linux/arm64/echo-provider
`, logPath)
	if err := os.WriteFile(filepath.Join(binDir, "goreleaser"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath)

	releaseBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"release",
		"--manifest", filepath.Join(providerDir, "tinx.yaml"),
		"--dist", filepath.Join(providerDir, "artifacts"),
		"--output", filepath.Join(providerDir, "oci"),
		"--delegate-goreleaser",
	})
	if !bytes.Contains(releaseBuf.Bytes(), []byte("released sourceplane/echo-provider")) {
		t.Fatalf("unexpected release output: %s", releaseBuf.String())
	}
	if _, err := os.Stat(filepath.Join(providerDir, ".goreleaser.tinx.generated.yaml")); err != nil {
		t.Fatalf("expected generated goreleaser config: %v", err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logged), "--config") {
		t.Fatalf("expected goreleaser invocation to include --config, got %q", string(logged))
	}
}

func TestReleasePushAndInstallFromRegistry(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, ".tinx-home")
	providerDir := copyTestProvider(t, workspace)
	server := httptest.NewServer(gcrregistry.New())
	defer server.Close()
	registryHost := strings.TrimPrefix(server.URL, "http://")
	ref := registryHost + "/sourceplane/echo-provider:v0.1.0"

	releaseBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"release",
		"--manifest", filepath.Join(providerDir, "tinx.yaml"),
		"--dist", filepath.Join(providerDir, "dist"),
		"--output", filepath.Join(providerDir, "oci"),
		"--push", ref,
		"--plain-http",
	})
	if !bytes.Contains(releaseBuf.Bytes(), []byte("pushed "+ref)) {
		t.Fatalf("unexpected push output: %s", releaseBuf.String())
	}

	installBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"install", "echo", ref,
		"--plain-http",
	})
	if !bytes.Contains(installBuf.Bytes(), []byte("installed sourceplane/echo-provider@v0.1.0")) {
		t.Fatalf("unexpected install output: %s", installBuf.String())
	}

	listBuf := runRootCommand(t, []string{"--tinx-home", home, "list", "providers", "default"})
	if !bytes.Contains(listBuf.Bytes(), []byte("tinx add "+ref+" as echo")) {
		t.Fatalf("expected default inventory to guide workspace add flow, got: %s", listBuf.String())
	}
}

func TestInstallUsesCachedRemoteProvider(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, ".tinx-home")
	providerDir := copyTestProvider(t, workspace)
	server := httptest.NewServer(gcrregistry.New())
	defer server.Close()
	registryHost := strings.TrimPrefix(server.URL, "http://")
	ref := registryHost + "/sourceplane/echo-provider:v0.1.0"

	releaseBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"release",
		"--manifest", filepath.Join(providerDir, "tinx.yaml"),
		"--dist", filepath.Join(providerDir, "dist"),
		"--output", filepath.Join(providerDir, "oci"),
		"--push", ref,
		"--plain-http",
	})
	if !bytes.Contains(releaseBuf.Bytes(), []byte("pushed "+ref)) {
		t.Fatalf("unexpected push output: %s", releaseBuf.String())
	}

	firstInstallBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"install", ref,
		"--plain-http",
	})
	if !bytes.Contains(firstInstallBuf.Bytes(), []byte("installed sourceplane/echo-provider@v0.1.0")) {
		t.Fatalf("unexpected first install output: %s", firstInstallBuf.String())
	}

	server.Close()

	secondInstallBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"install", "ci", ref,
		"--plain-http",
	})
	if !bytes.Contains(secondInstallBuf.Bytes(), []byte("installed sourceplane/echo-provider@v0.1.0")) {
		t.Fatalf("unexpected cached install output: %s", secondInstallBuf.String())
	}
}

func TestInstallRejectsProviderSourceSchemes(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".tinx-home")
	buf := new(bytes.Buffer)
	err := executeCLI(context.Background(), []string{"--tinx-home", home, "install", "tool", "custom://acme/setup@v1"}, buf, buf)
	if err == nil {
		t.Fatal("expected install to reject provider source scheme")
	}
	if !strings.Contains(err.Error(), `unsupported provider source "custom://acme/setup@v1"`) {
		t.Fatalf("unexpected install error: %v", err)
	}
}

func TestInstallRejectsStandaloneExecutionAfterDash(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".tinx-home")
	buf := new(bytes.Buffer)
	err := executeCLI(context.Background(), []string{"--tinx-home", home, "install", "sourceplane/echo-provider", "--", "echo-provider", "plan"}, buf, buf)
	if err == nil {
		t.Fatal("expected install to reject standalone execution")
	}
	if !strings.Contains(err.Error(), "install no longer executes commands") {
		t.Fatalf("unexpected install error: %v", err)
	}
}

func TestRunCommandRemoved(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".tinx-home")
	buf := new(bytes.Buffer)
	err := executeCLI(context.Background(), []string{"--tinx-home", home, "run", "sourceplane/echo-provider", "plan"}, buf, buf)
	if err == nil {
		t.Fatal("expected run to be rejected")
	}
	if !strings.Contains(err.Error(), "'tinx run' has been removed") {
		t.Fatalf("unexpected run error: %v", err)
	}
}

func TestDirectExecutionRemoved(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".tinx-home")
	buf := new(bytes.Buffer)
	err := executeCLI(context.Background(), []string{"--tinx-home", home, "echo-provider", "plan"}, buf, buf)
	if err == nil {
		t.Fatal("expected direct execution to be rejected")
	}
	if !strings.Contains(err.Error(), "direct provider execution has been removed") {
		t.Fatalf("unexpected direct execution error: %v", err)
	}
}

func copyTestProvider(t *testing.T, workspace string) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "echo-provider")
	dst := filepath.Join(workspace, "provider")
	if err := copyTree(src, dst); err != nil {
		t.Fatalf("copy test provider: %v", err)
	}
	return dst
}

func copyTree(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func runRootCommand(t *testing.T, args []string) *bytes.Buffer {
	t.Helper()
	buf := new(bytes.Buffer)
	if err := executeCLI(context.Background(), args, buf, buf); err != nil {
		t.Fatalf("command %v failed: %v\n%s", args, err, buf.String())
	}
	return buf
}
