package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	gcrregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/spf13/cobra"

	"github.com/sourceplane/tinx/internal/state"
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

func TestRunGitHubCompositeActionDirectly(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, ".tinx-home")
	repoRoot := filepath.Join(workspace, "gha-repos")
	ref := createTestGitHubActionRepo(t, repoRoot)
	t.Setenv("TINX_GHA_REPO_ROOT", repoRoot)

	runBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"run", ref,
		"--input=tool-name=alpha",
	})
	output := runBuf.String()
	for _, expected := range []string{
		"had_tool=false",
		"install_state=fresh",
		"tool_name=alpha",
		"resolved_tool=tool=alpha",
		"tinx_path=",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in output, got: %s", expected, output)
		}
	}

	statePath := filepath.Join(home, "providers", "gha", "acme", "setup-echo", "v1", "gha-runtime-state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read GitHub Action runtime state: %v", err)
	}
	var runtimeState struct {
		Outputs map[string]string `json:"outputs"`
	}
	if err := json.Unmarshal(data, &runtimeState); err != nil {
		t.Fatalf("decode GitHub Action runtime state: %v", err)
	}
	if runtimeState.Outputs["install-state"] != "fresh" {
		t.Fatalf("expected install-state output to be fresh, got %q", runtimeState.Outputs["install-state"])
	}
}

func TestInstallGitHubCompositeActionAliasPersistsRuntimeState(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, ".tinx-home")
	repoRoot := filepath.Join(workspace, "gha-repos")
	ref := createTestGitHubActionRepo(t, repoRoot)
	t.Setenv("TINX_GHA_REPO_ROOT", repoRoot)

	installBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"install", "setup", ref,
	})
	if !bytes.Contains(installBuf.Bytes(), []byte("installed gha/setup@v1")) {
		t.Fatalf("unexpected install output: %s", installBuf.String())
	}

	helpBuf := runAliasCommand(t, home, []string{"setup", "help"})
	if !bytes.Contains(helpBuf.Bytes(), []byte("Inputs:")) || !bytes.Contains(helpBuf.Bytes(), []byte("tool-name")) {
		t.Fatalf("expected GitHub Action help to include inputs, got: %s", helpBuf.String())
	}

	firstRun := runAliasCommand(t, home, []string{"setup", "--input=tool-name=alpha"})
	if !bytes.Contains(firstRun.Bytes(), []byte("had_tool=false")) || !bytes.Contains(firstRun.Bytes(), []byte("install_state=fresh")) {
		t.Fatalf("unexpected first alias run output: %s", firstRun.String())
	}

	secondRun := runAliasCommand(t, home, []string{"setup", "--input=tool-name=alpha"})
	if !bytes.Contains(secondRun.Bytes(), []byte("had_tool=true")) || !bytes.Contains(secondRun.Bytes(), []byte("install_state=cached")) {
		t.Fatalf("expected persisted GitHub Action runtime state, got: %s", secondRun.String())
	}
}

func TestRunGitHubNodeActionDirectly(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, ".tinx-home")
	repoRoot := filepath.Join(workspace, "gha-repos")
	ref := createTestGitHubNodeActionRepo(t, repoRoot)
	t.Setenv("TINX_GHA_REPO_ROOT", repoRoot)

	runBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"run", ref,
		"--input=tool-name=beta",
	})
	output := runBuf.String()
	for _, expected := range []string{
		"had_tool=false",
		"install_state=fresh",
		"tool_name=beta",
		"runner_tool_cache=",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in output, got: %s", expected, output)
		}
	}

	statePath := filepath.Join(home, "providers", "gha", "acme", "setup-node-echo", "v1", "gha-runtime-state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read GitHub Action runtime state: %v", err)
	}
	var runtimeState struct {
		Outputs map[string]string `json:"outputs"`
	}
	if err := json.Unmarshal(data, &runtimeState); err != nil {
		t.Fatalf("decode GitHub Action runtime state: %v", err)
	}
	if runtimeState.Outputs["install-state"] != "fresh" {
		t.Fatalf("expected install-state output to be fresh, got %q", runtimeState.Outputs["install-state"])
	}
	if runtimeState.Outputs["tool-path"] == "" {
		t.Fatalf("expected node action to persist tool-path output")
	}
}

func TestInstallGitHubNodeActionAliasPromotesToBinaryProvider(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, ".tinx-home")
	repoRoot := filepath.Join(workspace, "gha-repos")
	ref := createTestGitHubNodeActionRepo(t, repoRoot)
	t.Setenv("TINX_GHA_REPO_ROOT", repoRoot)

	installBuf := runRootCommand(t, []string{
		"--tinx-home", home,
		"install", "node-setup", ref,
		"--input=tool-name=beta",
	})
	if !bytes.Contains(installBuf.Bytes(), []byte("installed gha/node-setup@v1")) {
		t.Fatalf("unexpected install output: %s", installBuf.String())
	}

	helpBuf := runAliasCommand(t, home, []string{"node-setup", "help"})
	if !bytes.Contains(helpBuf.Bytes(), []byte("Configured Inputs:")) || !bytes.Contains(helpBuf.Bytes(), []byte("tool-name")) {
		t.Fatalf("expected promoted provider help to include configured inputs, got: %s", helpBuf.String())
	}
	if bytes.Contains(helpBuf.Bytes(), []byte("Capabilities:")) {
		t.Fatalf("expected promoted provider help to skip capability listing, got: %s", helpBuf.String())
	}

	runBuf := runAliasCommand(t, home, []string{"node-setup", "status", "--short"})
	if !bytes.Contains(runBuf.Bytes(), []byte("node-tool=beta")) || !bytes.Contains(runBuf.Bytes(), []byte("args=status --short")) {
		t.Fatalf("expected promoted provider to execute local binary, got: %s", runBuf.String())
	}

	meta, err := state.LoadProviderMetadata(home, "gha", "node-setup")
	if err != nil {
		t.Fatalf("load promoted provider metadata: %v", err)
	}
	if meta.Runtime != "binary" {
		t.Fatalf("expected promoted provider runtime to be binary, got %q", meta.Runtime)
	}
	if meta.InvocationStyle != state.InvocationStylePassthrough {
		t.Fatalf("expected promoted provider invocation style to be passthrough, got %q", meta.InvocationStyle)
	}
	binaryPath := filepath.Join(home, "providers", "gha", "node-setup", "v1", "bin", goruntime.GOOS, goruntime.GOARCH, meta.Entrypoint)
	if _, err := os.Stat(binaryPath); err != nil {
		t.Fatalf("expected promoted provider binary at %s: %v", binaryPath, err)
	}
	if _, err := os.Stat(filepath.Join(home, "providers", "gha", "node-setup", "v1", "tinx.yaml")); err != nil {
		t.Fatalf("expected generated tinx.yaml for promoted provider: %v", err)
	}
	aliases, err := state.LoadAliases(home)
	if err != nil {
		t.Fatalf("load aliases: %v", err)
	}
	if aliases["node-setup"] != "gha/node-setup" {
		t.Fatalf("expected alias to target installed provider ref, got %q", aliases["node-setup"])
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

func createTestGitHubActionRepo(t *testing.T, root string) string {
	t.Helper()
	repoDir := filepath.Join(root, "acme", "setup-echo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	action := strings.Join([]string{
		"name: Setup Echo",
		"description: Composite action fixture",
		"inputs:",
		"  tool-name:",
		"    description: Tool name to expose",
		"    required: true",
		"outputs:",
		"  install-state:",
		"    description: Whether the tool was created or reused",
		"    value: ${{ steps.install.outputs.install_state }}",
		"runs:",
		"  using: composite",
		"  steps:",
		"    - id: probe",
		"      shell: bash",
		"      run: |",
		"        if command -v echo-tool >/dev/null 2>&1; then",
		"          echo \"had_tool=true\" >> \"$GITHUB_OUTPUT\"",
		"        else",
		"          echo \"had_tool=false\" >> \"$GITHUB_OUTPUT\"",
		"        fi",
		"    - id: install",
		"      shell: bash",
		"      run: |",
		"        tool_dir=\"$TINX_PROVIDER_HOME/tool-cache/${{ inputs.tool-name }}\"",
		"        install_state=\"cached\"",
		"        if [ ! -x \"$tool_dir/echo-tool\" ]; then",
		"          mkdir -p \"$tool_dir\"",
		"          echo '#!/bin/sh' > \"$tool_dir/echo-tool\"",
		"          echo 'printf '\\''tool=%s\\n'\\'' \"${TOOL_NAME:-unset}\"' >> \"$tool_dir/echo-tool\"",
		"          chmod +x \"$tool_dir/echo-tool\"",
		"          install_state=\"fresh\"",
		"        fi",
		"        echo \"$tool_dir\" >> \"$GITHUB_PATH\"",
		"        echo \"TOOL_NAME=${{ inputs.tool-name }}\" >> \"$GITHUB_ENV\"",
		"        echo \"install_state=$install_state\" >> \"$GITHUB_OUTPUT\"",
		"    - shell: bash",
		"      run: |",
		"        echo \"had_tool=${{ steps.probe.outputs.had_tool }}\"",
		"        echo \"install_state=${{ steps.install.outputs.install_state }}\"",
		"        echo \"tool_name=$TOOL_NAME\"",
		"        echo \"tinx_path=$(command -v tinx || true)\"",
		"        echo \"action_path=$GITHUB_ACTION_PATH\"",
		"        echo \"workspace=$GITHUB_WORKSPACE\"",
		"        echo \"resolved_tool=$(echo-tool)\"",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(repoDir, "action.yml"), []byte(action), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("init GitHub Action repo: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("open GitHub Action worktree: %v", err)
	}
	if _, err := worktree.Add("action.yml"); err != nil {
		t.Fatalf("stage action manifest: %v", err)
	}
	hash, err := worktree.Commit("initial action", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "tinx test",
			Email: "tinx@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("commit GitHub Action fixture: %v", err)
	}
	if _, err := repo.CreateTag("v1", hash, nil); err != nil {
		t.Fatalf("tag GitHub Action fixture: %v", err)
	}
	return "gha://acme/setup-echo@v1"
}

func createTestGitHubNodeActionRepo(t *testing.T, root string) string {
	t.Helper()
	repoDir := filepath.Join(root, "acme", "setup-node-echo")
	if err := os.MkdirAll(filepath.Join(repoDir, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	action := `name: Setup Node Echo
description: Node action fixture
inputs:
  tool-name:
    description: Tool name to expose
    required: true
outputs:
  install-state:
    description: Whether the tool was created or reused
  tool-path:
    description: Cached tool location
runs:
  using: node20
  main: dist/index.js
`
	script := `const fs = require('fs')
const path = require('path')

const toolName = process.env.INPUT_TOOL_NAME
const toolCache = process.env.RUNNER_TOOL_CACHE
const toolDir = path.join(toolCache, toolName)
const toolPath = path.join(toolDir, 'node-tool')
const pathEntries = (process.env.PATH || '').split(path.delim).filter(Boolean)
const hadTool = pathEntries.some((entry) => fs.existsSync(path.join(entry, 'node-tool')))
let installState = 'cached'

if (!fs.existsSync(toolPath)) {
  fs.mkdirSync(toolDir, {recursive: true})
	fs.writeFileSync(toolPath, "#!/bin/sh\nprintf 'node-tool=%s\\n' \"${TOOL_NAME:-unset}\"\nprintf 'args=%s\\n' \"$*\"\n")
  fs.chmodSync(toolPath, 0o755)
  installState = 'fresh'
}

fs.appendFileSync(process.env.GITHUB_PATH, toolDir + '\n')
fs.appendFileSync(process.env.GITHUB_ENV, 'TOOL_NAME=' + toolName + '\n')
fs.appendFileSync(process.env.GITHUB_OUTPUT, 'install-state=' + installState + '\n')
fs.appendFileSync(process.env.GITHUB_OUTPUT, 'tool-path=' + toolPath + '\n')

console.log('had_tool=' + hadTool)
console.log('install_state=' + installState)
console.log('tool_name=' + toolName)
console.log('path=' + process.env.PATH)
console.log('runner_tool_cache=' + toolCache)
`
	if err := os.WriteFile(filepath.Join(repoDir, "action.yml"), []byte(action), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "dist", "index.js"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("init GitHub Action repo: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("open GitHub Action worktree: %v", err)
	}
	if _, err := worktree.Add("action.yml"); err != nil {
		t.Fatalf("stage action manifest: %v", err)
	}
	if _, err := worktree.Add("dist/index.js"); err != nil {
		t.Fatalf("stage node action entrypoint: %v", err)
	}
	hash, err := worktree.Commit("initial node action", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "tinx test",
			Email: "tinx@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("commit GitHub Action fixture: %v", err)
	}
	if _, err := repo.CreateTag("v1", hash, nil); err != nil {
		t.Fatalf("tag GitHub Action fixture: %v", err)
	}
	return "gha://acme/setup-node-echo@v1"
}

func runRootCommand(t *testing.T, args []string) *bytes.Buffer {
	t.Helper()
	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	if err := cmd.ExecuteContext(context.Background()); err != nil {
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
