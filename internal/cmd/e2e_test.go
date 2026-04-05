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
	"github.com/spf13/cobra"
)

func TestReleaseInstallAndRun(t *testing.T) {
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

	runInstallAndRunAssertions(t, home, providerDir, filepath.Join(providerDir, "oci"), "")
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

	runBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"run", "echo", "plan", "--intent", "intent.yaml",
	})
	if !bytes.Contains(runBuf.Bytes(), []byte("capability=plan")) {
		t.Fatalf("unexpected run output: %s", runBuf.String())
	}
}

func TestRunDirectFromRegistryWithoutInstall(t *testing.T) {
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

	runBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"run", ref, "plan",
		"--plain-http",
		"--intent", "intent.yaml",
	})
	if !bytes.Contains(runBuf.Bytes(), []byte("capability=plan")) {
		t.Fatalf("unexpected direct run output: %s", runBuf.String())
	}
	if !bytes.Contains(runBuf.Bytes(), []byte("args=--intent,intent.yaml")) {
		t.Fatalf("expected provider args to include --intent, got: %s", runBuf.String())
	}

	runWithConfigBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"run", ref, "plan",
		"--config-dir", "/tmp/compositions",
		"--plain-http",
	})
	if !bytes.Contains(runWithConfigBuf.Bytes(), []byte("args=--config-dir,/tmp/compositions")) {
		t.Fatalf("expected provider args to include --config-dir, got: %s", runWithConfigBuf.String())
	}
}

func TestRunThenInstallUsesCachedRemoteProvider(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, ".tinx-home")
	providerDir := copyTestProvider(t, workspace)
	server := httptest.NewServer(gcrregistry.New())
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

	firstRunBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"run", ref, "plan",
		"--plain-http",
		"--intent", "intent.yaml",
	})
	if !bytes.Contains(firstRunBuf.Bytes(), []byte("capability=plan")) {
		t.Fatalf("unexpected first direct run output: %s", firstRunBuf.String())
	}

	server.Close()

	secondRunBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"run", ref, "plan",
		"--plain-http",
		"--intent", "intent.yaml",
	})
	if !bytes.Contains(secondRunBuf.Bytes(), []byte("capability=plan")) {
		t.Fatalf("unexpected second direct run output: %s", secondRunBuf.String())
	}

	installBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"install", "ci", ref,
		"--plain-http",
	})
	if !bytes.Contains(installBuf.Bytes(), []byte("installed sourceplane/echo-provider@v0.1.0")) {
		t.Fatalf("unexpected install output after cache: %s", installBuf.String())
	}

	runAliasBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"run", "ci", "plan",
		"--intent", "intent.yaml",
	})
	if !bytes.Contains(runAliasBuf.Bytes(), []byte("capability=plan")) {
		t.Fatalf("unexpected alias run output after cache: %s", runAliasBuf.String())
	}
}

func TestRunProviderHelpFromAlias(t *testing.T) {
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

	helpBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"run", "echo", "help",
	})
	if !bytes.Contains(helpBuf.Bytes(), []byte("Capabilities:")) {
		t.Fatalf("missing capabilities section in help output: %s", helpBuf.String())
	}
	if !bytes.Contains(helpBuf.Bytes(), []byte("plan")) {
		t.Fatalf("missing capability in help output: %s", helpBuf.String())
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

func TestRunRejectsProviderSourceSchemes(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".tinx-home")
	buf := new(bytes.Buffer)
	err := executeCLI(context.Background(), []string{"--tinx-home", home, "run", "custom://acme/setup@v1", "plan"}, buf, buf)
	if err == nil {
		t.Fatal("expected run to reject provider source scheme")
	}
	if !strings.Contains(err.Error(), `unsupported provider source "custom://acme/setup@v1"`) {
		t.Fatalf("unexpected run error: %v", err)
	}
}

func runInstallAndRunAssertions(t *testing.T, home, providerDir, layoutPath, ref string) {
	t.Helper()
	installArgs := []string{"--tinx-home", home, "install", "echo", "sourceplane/echo-provider", "--source", layoutPath}
	if ref != "" {
		installArgs = []string{"--tinx-home", home, "install", "echo", ref, "--plain-http"}
	}
	installBuf := runRootCommand(t, installArgs)
	if !bytes.Contains(installBuf.Bytes(), []byte("installed sourceplane/echo-provider@v0.1.0")) {
		t.Fatalf("unexpected install output: %s", installBuf.String())
	}

	runBuf := runRootCommand(t, []string{"--tinx-home", home, "run", "echo", "plan", "--intent", "intent.yaml"})
	if !bytes.Contains(runBuf.Bytes(), []byte("capability=plan")) {
		t.Fatalf("unexpected run output: %s", runBuf.String())
	}

	cachedAsset := filepath.Join(home, "providers", "sourceplane", "echo-provider", "v0.1.0", "assets", "templates", "intent.json")
	if _, err := os.Stat(cachedAsset); err != nil {
		t.Fatalf("expected cached asset at %s: %v", cachedAsset, err)
	}
	_ = providerDir
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

func runAliasCommand(t *testing.T, home string, args []string) *bytes.Buffer {
	t.Helper()
	cmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := runAlias(cmd, &rootOptions{Home: home}, args); err != nil {
		t.Fatalf("alias command %v failed: %v\n%s", args, err, buf.String())
	}
	return buf
}

func TestTinxCLIHelperProcess(t *testing.T) {
	if os.Getenv("TINX_CLI_HELPER_PROCESS") != "1" {
		return
	}
	separator := -1
	for index, arg := range os.Args {
		if arg == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		fmt.Fprintln(os.Stderr, "missing helper separator")
		os.Exit(2)
	}
	if err := executeCLI(context.Background(), os.Args[separator+1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}
